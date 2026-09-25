// Package mcpserver exposes native, read-only Lumbrera operations over MCP.
package mcpserver

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/indexruntime"
	"github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/searchservice"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Config struct {
	Brain   string
	Version string
}

type Service struct {
	config  Config
	gate    chan struct{}
	handler http.Handler
}

type searchInput struct {
	Query      string   `json:"query"`
	Limit      *int     `json:"limit"`
	Kind       string   `json:"kind"`
	PathPrefix string   `json:"path_prefix"`
	Tags       []string `json:"tags"`
	Sources    []string `json:"sources"`
	Tiers      []string `json:"tiers"`
}

type readInput struct {
	Path     string `json:"path"`
	Anchor   string `json:"anchor"`
	Offset   int    `json:"offset"`
	MaxBytes *int   `json:"max_bytes"`
}

const requestMax = 65536
const responseMax = 2 << 20
const readMax = 65536
const readDefault = 32768
const toolTimeout = 30 * time.Second

//go:embed contracts/mcp-v1.json
var contract []byte

func New(config Config) (*Service, error) {
	if config.Brain == "" {
		return nil, fmt.Errorf("brain directory is required")
	}
	var err error
	config.Brain, err = filepath.Abs(config.Brain)
	if err != nil {
		return nil, err
	}
	if err = brain.RequireCurrent(config.Brain); err != nil {
		return nil, err
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	s := &Service{config: config, gate: make(chan struct{}, 1)}
	ms := server.NewMCPServer("lumbrera", config.Version, server.WithToolCapabilities(false), server.WithRecovery(), server.WithInputSchemaValidation(), server.WithOutputSchemaValidation())
	// Raw schemas must be preserved rather than round-tripped through the SDK's
	// restricted property model.
	var raw struct {
		Tools []struct {
			Name, Description         string
			InputSchema, OutputSchema json.RawMessage
			Annotations               mcp.ToolAnnotation
		} `json:"tools"`
	}
	if err = json.Unmarshal(contract, &raw); err != nil {
		return nil, err
	}
	handlers := map[string]server.ToolHandlerFunc{"brain_search": s.search, "brain_read": s.read, "brain_status": s.status}
	for _, d := range raw.Tools {
		tool := mcp.NewToolWithRawSchema(d.Name, d.Description, d.InputSchema)
		tool.RawOutputSchema = d.OutputSchema
		tool.Annotations = d.Annotations
		ms.AddTool(tool, s.bounded(handlers[d.Name]))
	}
	transport := server.NewStreamableHTTPServer(ms, server.WithStateful(false), server.WithDisableStreaming(true))
	mux := http.NewServeMux()
	mux.Handle("/mcp", transport)
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		state, err := s.indexStatus(ctx)
		if err != nil || state != searchindex.StatusFresh {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This endpoint is for native MCP clients; browser access is not enabled.
		if _, ok := r.Header["Origin"]; ok {
			http.Error(w, "browser origins are not allowed", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, requestMax)
		mux.ServeHTTP(w, r)
	})
	return s, nil
}

func (s *Service) Handler() http.Handler { return s.handler }

// enter serializes server operations and makes waiting cancellable.
func (s *Service) enter(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) leave() { <-s.gate }

// Prepare uses the same non-repairing automatic index refresh as CLI search.
func (s *Service) Prepare(ctx context.Context) error {
	if err := s.enter(ctx); err != nil {
		return err
	}
	defer s.leave()
	if _, err := brainfs.ValidateDirectory(s.config.Brain, ".brain", false); err != nil {
		return err
	}
	return indexruntime.EnsureFresh(ctx, s.config.Brain)
}

// lock cooperates with CLI writes/index rebuilds. Direct filesystem edits do
// not honor this lock and are outside the per-operation consistency guarantee.
func (s *Service) lock() (*brainlock.Lock, error) {
	if _, err := brainfs.ValidateDirectory(s.config.Brain, ".brain", false); err != nil {
		return nil, err
	}
	return brainlock.Acquire(s.config.Brain, "mcp-read")
}
func (s *Service) indexStatus(ctx context.Context) (searchindex.StatusState, error) {
	if err := s.enter(ctx); err != nil {
		return "", err
	}
	defer s.leave()
	lock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer lock.Release()
	status, err := searchindex.CheckStatus(ctx, s.config.Brain)
	return status.State, err
}

func (s *Service) bounded(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, toolTimeout)
		defer cancel()
		return next(ctx, r)
	}
}

func failure(code, message string, retryable bool) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": retryable}})
	return mcp.NewToolResultError(string(b))
}
func result(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return failure("INTERNAL", "Cannot encode response", false), nil
	}
	if len(data) > responseMax {
		return failure("RESPONSE_TOO_LARGE", "Narrow the query or reduce the page size", false), nil
	}
	toolResult := mcp.NewToolResultStructured(v, string(data))
	encoded, err := json.Marshal(toolResult)
	if err != nil {
		return failure("INTERNAL", "Cannot encode response", false), nil
	}
	if len(encoded) > responseMax {
		return failure("RESPONSE_TOO_LARGE", "Narrow the query or reduce the page size", false), nil
	}
	return toolResult, nil
}
func arguments(r mcp.CallToolRequest, v any) error {
	data, err := json.Marshal(r.Params.Arguments)
	if err != nil {
		return err
	}
	if string(data) == "null" {
		data = []byte("{}")
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("null argument")
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func (in searchInput) validate() *mcp.CallToolResult {
	if strings.TrimSpace(in.Query) == "" || utf8.RuneCountInString(in.Query) > 4096 {
		return failure("INVALID_ARGUMENT", "Invalid search arguments", false)
	}
	if in.Limit != nil && (*in.Limit < 1 || *in.Limit > 20) {
		return failure("INVALID_ARGUMENT", "Limit must be between 1 and 20", false)
	}
	if in.Kind != "" && in.Kind != "all" && in.Kind != "wiki" && in.Kind != "note" && in.Kind != "source" {
		return failure("INVALID_ARGUMENT", "Invalid kind", false)
	}
	if in.PathPrefix != "" && !allowedPrefix(in.PathPrefix) {
		return failure("PATH_NOT_ALLOWED", "Invalid content prefix", false)
	}
	for _, tag := range in.Tags {
		if strings.TrimSpace(tag) == "" {
			return failure("INVALID_ARGUMENT", "Empty tag", false)
		}
	}
	for _, source := range in.Sources {
		if !allowedPath(source) || !(strings.HasPrefix(source, "sources/") || strings.HasPrefix(source, "notes/")) {
			return failure("PATH_NOT_ALLOWED", "Invalid evidence filter", false)
		}
	}
	for _, tier := range in.Tiers {
		if tier != "canonical" && tier != "design" && tier != "reference" {
			return failure("INVALID_ARGUMENT", "Invalid tier", false)
		}
	}
	return nil
}

func (s *Service) search(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in searchInput
	if arguments(r, &in) != nil {
		return failure("INVALID_ARGUMENT", "Invalid search arguments", false), nil
	}
	if invalid := in.validate(); invalid != nil {
		return invalid, nil
	}
	limit := 5
	if in.Limit != nil {
		limit = *in.Limit
	}
	if in.Kind == "" {
		in.Kind = "all"
	}
	if err := s.enter(ctx); err != nil {
		return failure("TIMEOUT", "Search was cancelled or timed out", true), nil
	}
	defer s.leave()
	if _, err := brainfs.ValidateDirectory(s.config.Brain, ".brain", false); err != nil {
		return failure("NOT_READY", "Unsafe index directory", false), nil
	}
	if err := indexruntime.EnsureFresh(ctx, s.config.Brain); err != nil {
		if ctx.Err() != nil {
			return failure("TIMEOUT", "Search was cancelled or timed out", true), nil
		}
		return failure("NOT_READY", "Cannot prepare index; check brain integrity or retry after another operation", true), nil
	}
	lock, err := s.lock()
	if err != nil {
		return failure("BUSY", "Another Lumbrera operation is in progress", true), nil
	}
	defer lock.Release()
	// A CLI write may have happened after EnsureFresh released its lock.
	status, err := searchindex.CheckStatus(ctx, s.config.Brain)
	if err != nil || status.State != searchindex.StatusFresh {
		return failure("NOT_READY", "Index changed during preparation; retry search", true), nil
	}
	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(s.config.Brain))
	if err != nil {
		return failure("NOT_READY", "Index cannot be opened", true), nil
	}
	defer db.Close()
	response, err := searchindex.Search(ctx, db, in.Query, searchindex.SearchOptions{Limit: limit, Kind: in.Kind, PathPrefix: in.PathPrefix, Tags: in.Tags, Sources: in.Sources, Tiers: in.Tiers})
	if err != nil {
		if ctx.Err() != nil {
			return failure("TIMEOUT", "Search was cancelled or timed out", true), nil
		}
		return failure("INVALID_ARGUMENT", "Search could not be evaluated", false), nil
	}
	return result(searchservice.Project(response))
}

func (s *Service) read(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in readInput
	if arguments(r, &in) != nil || in.Offset < 0 || strings.Contains(in.Anchor, "#") {
		return failure("INVALID_ARGUMENT", "Invalid read arguments", false), nil
	}
	if !allowedPath(in.Path) {
		return failure("PATH_NOT_ALLOWED", "Path is outside readable brain content", false), nil
	}
	size := readDefault
	if in.MaxBytes != nil {
		size = *in.MaxBytes
	}
	if size < 1 || size > readMax {
		return failure("INVALID_ARGUMENT", "max_bytes must be between 1 and 65536", false), nil
	}
	if err := s.enter(ctx); err != nil {
		return failure("TIMEOUT", "Read was cancelled", true), nil
	}
	defer s.leave()
	lock, err := s.lock()
	if err != nil {
		return failure("BUSY", "Another Lumbrera operation is in progress", true), nil
	}
	defer lock.Release()
	root, err := os.OpenRoot(s.config.Brain)
	if err != nil {
		return failure("NOT_READY", "Brain directory unavailable", true), nil
	}
	defer root.Close()
	// Reject symlinks even when their destination is inside the brain, so an
	// allowed content path cannot alias internal files or instructions.
	parts := strings.Split(in.Path, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return failure("NOT_FOUND", "Content not found", false), nil
			}
			return failure("PATH_NOT_ALLOWED", "Content cannot be inspected", false), nil
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return failure("UNSUPPORTED_MEDIA", "Not a regular document", false), nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return failure("PATH_NOT_ALLOWED", "Symlinks are not readable content", false), nil
		}
	}
	file, err := root.Open(in.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return failure("NOT_FOUND", "Content not found", false), nil
		}
		return failure("PATH_NOT_ALLOWED", "Content cannot be opened", false), nil
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > maxBlobBytes {
		return failure("UNSUPPORTED_MEDIA", "Not a supported document", false), nil
	}
	data, err := readBounded(file, maxBlobBytes)
	if err != nil {
		return failure("INTERNAL", "Content cannot be read", false), nil
	}
	if !utf8.Valid(data) {
		return failure("UNSUPPORTED_MEDIA", "Content is not UTF-8 text", false), nil
	}
	body := string(data)
	if strings.HasPrefix(in.Path, "wiki/") || strings.HasPrefix(in.Path, "notes/") {
		_, body, _, err = frontmatter.Split(data)
		if err != nil {
			return failure("INTERNAL", "Invalid managed content", false), nil
		}
	}
	var anchor, heading any
	if in.Anchor != "" {
		sections, err := markdown.SplitSections(body)
		if err != nil {
			return failure("INTERNAL", "Cannot parse sections", false), nil
		}
		found := false
		for _, section := range sections {
			if section.Anchor == in.Anchor {
				body = section.Body
				anchor = in.Anchor
				heading = section.Heading
				found = true
				break
			}
		}
		if !found {
			return failure("ANCHOR_NOT_FOUND", "Section anchor not found", false), nil
		}
	}
	if in.Offset > len(body) || (in.Offset < len(body) && !utf8.RuneStart(body[in.Offset])) {
		return failure("INVALID_ARGUMENT", "Offset must be a UTF-8 boundary within the document", false), nil
	}
	end := in.Offset + size
	if end > len(body) {
		end = len(body)
	}
	for end < len(body) && end > in.Offset && !utf8.RuneStart(body[end]) {
		end--
	}
	if end == in.Offset && end < len(body) {
		return failure("INVALID_ARGUMENT", "Page size is smaller than the next UTF-8 character", false), nil
	}
	var next any
	if end < len(body) {
		next = end
	}
	return result(map[string]any{"path": in.Path, "anchor": anchor, "heading": heading, "content": body[in.Offset:end], "offset": in.Offset, "bytes_returned": end - in.Offset, "has_more": end < len(body), "next_offset": next})
}

func (s *Service) status(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if arguments(r, &struct{}{}) != nil {
		return failure("INVALID_ARGUMENT", "Status accepts no arguments", false), nil
	}
	state, err := s.indexStatus(ctx)
	if err != nil {
		state = "failed"
	}
	return result(map[string]any{"ready": err == nil && state == searchindex.StatusFresh, "brain_format": brain.Version, "index_state": state, "writes_enabled": false, "limits": map[string]int{"search_limit_max": 20, "read_max_bytes": readMax, "read_default_bytes": readDefault, "request_max_bytes": requestMax, "response_max_bytes": responseMax, "search_timeout_seconds": 30}})
}

const maxBlobBytes = 64 << 20

func safeRelative(name string) bool {
	if name == "" || name == "." || len(name) > 1024 || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".." || p == "" {
			return false
		}
	}
	return true
}
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("content exceeds limit")
	}
	return data, nil
}
func allowedPath(p string) bool {
	if !safeRelative(p) {
		return false
	}
	switch p {
	case "INDEX.md", "SOURCES.md", "NOTES.md", "ASSETS.md", "CHANGELOG.md", "BRAIN.sum", "tags.md":
		return true
	}
	return (strings.HasPrefix(p, "sources/") || strings.HasPrefix(p, "notes/") || strings.HasPrefix(p, "wiki/")) && strings.HasSuffix(p, ".md")
}
func allowedPrefix(p string) bool {
	p = strings.TrimSuffix(p, "/")
	if !safeRelative(p) {
		return false
	}
	for _, root := range []string{"sources", "notes", "wiki"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

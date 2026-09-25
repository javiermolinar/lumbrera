package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/searchcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Evidence\n\nsearchunique evidence.\n", "sources/raw.md", "--reason", "test", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nsearchunique café.\n\n## Detail\n\nFirst detail.\n\n## Detail\n\nSecond detail.\n", "wiki/topic.md", "--title", "Topic", "--summary", "searchunique summary", "--tag", "topic", "--source", "sources/raw.md", "--reason", "test", "--actor", "test")
	return repo
}

func service(t *testing.T, repo string) *Service {
	t.Helper()
	s, err := New(Config{Brain: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}
func httpClient(t *testing.T, s *Service) *client.Client {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	c, err := client.NewStreamableHttpClient(ts.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_, err = c.Initialize(t.Context(), mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "test", Version: "1"}}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func call(t *testing.T, c *client.Client, name string, args map[string]any) map[string]any {
	t.Helper()
	r, err := c.CallTool(t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("%s: %#v", name, r.Content)
	}
	var v map[string]any
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	text, ok := r.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("missing JSON text fallback")
	}
	var fallback any
	if err = json.Unmarshal([]byte(text.Text), &fallback); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, fallback) {
		t.Fatal("structured/text mismatch")
	}
	return v
}
func direct(t *testing.T, s *Service, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	handlers := map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){"brain_read": s.read, "brain_search": s.search, "brain_status": s.status}
	r, err := handlers[name](t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func errorCode(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if !r.IsError {
		t.Fatalf("expected error, got %#v", r)
	}
	var v struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.Content[0].(mcp.TextContent).Text), &v); err != nil {
		t.Fatal(err)
	}
	return v.Error.Code
}

func TestHTTPContractAndCLIParity(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	c := httpClient(t, s)
	tools, err := c.ListTools(t.Context(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("tools: %#v", tools)
	}
	for _, tool := range tools.Tools {
		if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
			t.Fatalf("not read-only: %s", tool.Name)
		}
	}
	status := call(t, c, "brain_status", map[string]any{})
	if status["ready"] != true || status["writes_enabled"] != false {
		t.Fatalf("status: %#v", status)
	}
	for _, filter := range []map[string]any{{}, {"kind": "wiki", "tags": []string{"topic"}, "sources": []string{"sources/raw.md"}, "tiers": []string{"canonical"}, "path_prefix": "wiki/"}} {
		args := map[string]any{"query": "searchunique"}
		cli := []string{"searchunique", "--brain", repo}
		for k, v := range filter {
			args[k] = v
			switch k {
			case "kind":
				cli = append(cli, "--kind", v.(string))
			case "path_prefix":
				cli = append(cli, "--path", v.(string))
			default:
				flag := map[string]string{"tags": "--tag", "sources": "--source", "tiers": "--tier"}[k]
				for _, item := range v.([]string) {
					cli = append(cli, flag, item)
				}
			}
		}
		v := call(t, c, "brain_search", args)
		var out bytes.Buffer
		if err := searchcmd.RunWithOutput(cli, &out); err != nil {
			t.Fatal(err)
		}
		var expected any
		if err := json.Unmarshal(out.Bytes(), &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, expected) {
			t.Fatalf("CLI parity mismatch\nMCP: %#v\nCLI: %#v", v, expected)
		}
	}
	v := call(t, c, "brain_read", map[string]any{"path": "wiki/topic.md", "anchor": "detail-1"})
	if v["content"] != "Second detail." {
		t.Fatalf("read: %#v", v)
	}
}

func TestHTTPSearchWithoutHeadingMatchesCLI(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "headingless", body: "headinglessunique plain source text.\n"},
		{name: "preamble", body: "headinglessunique introductory text.\n\n# Details\n\nUnrelated section content.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := braintest.InitBrain(t)
			braintest.RunWrite(t, repo, tc.body, "sources/plain.md", "--reason", "test", "--actor", "test")
			c := httpClient(t, service(t, repo))
			got := call(t, c, "brain_search", map[string]any{"query": "headinglessunique"})
			var out bytes.Buffer
			if err := searchcmd.RunWithOutput([]string{"headinglessunique", "--brain", repo}, &out); err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal(out.Bytes(), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("CLI parity mismatch\nMCP: %#v\nCLI: %#v", got, want)
			}
			for _, field := range []string{"results", "recommended_sections"} {
				entries := got[field].([]any)
				if len(entries) == 0 {
					t.Fatalf("expected a matching %s entry", field)
				}
				entry := entries[0].(map[string]any)
				if entry["path"] != "sources/plain.md" {
					t.Fatalf("unexpected %s path: %#v", field, entry)
				}
				for _, optional := range []string{"anchor", "heading"} {
					if _, exists := entry[optional]; exists {
						t.Fatalf("expected %s.%s to be omitted: %#v", field, optional, entry)
					}
				}
			}
		})
	}
}

func TestReadBoundsAndDeniedPaths(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	for _, p := range []string{"../VERSION", "/etc/passwd", "wiki/../../x", "wiki\\topic.md", ".git/config", "AGENTS.md", ".agents/skills/a.md", "assets/a.md", "wiki/./topic.md"} {
		if got := errorCode(t, direct(t, s, "brain_read", map[string]any{"path": p})); got != "PATH_NOT_ALLOWED" {
			t.Fatalf("%s: %s", p, got)
		}
	}
	if got := errorCode(t, direct(t, s, "brain_read", map[string]any{"path": "wiki/missing.md"})); got != "NOT_FOUND" {
		t.Fatal(got)
	}
	if got := errorCode(t, direct(t, s, "brain_read", map[string]any{"path": "wiki/topic.md", "anchor": "missing"})); got != "ANCHOR_NOT_FOUND" {
		t.Fatal(got)
	}
	c := httpClient(t, s)
	full := call(t, c, "brain_read", map[string]any{"path": "wiki/topic.md"})["content"].(string)
	var accumulated strings.Builder
	offset := 0
	for {
		v := call(t, c, "brain_read", map[string]any{"path": "wiki/topic.md", "max_bytes": 7, "offset": offset})
		accumulated.WriteString(v["content"].(string))
		if v["has_more"] == false {
			break
		}
		offset = int(v["next_offset"].(float64))
	}
	if full != accumulated.String() {
		t.Fatal("pagination lost bytes")
	}
	middle := strings.Index(full, "é") + 1
	for _, args := range []map[string]any{{"offset": middle}, {"offset": middle - 1, "max_bytes": 1}, {"offset": len(full) + 1}, {"max_bytes": 65537}, {"unknown": true}} {
		args["path"] = "wiki/topic.md"
		if got := errorCode(t, direct(t, s, "brain_read", args)); got != "INVALID_ARGUMENT" {
			t.Fatal(got)
		}
	}
	// A symlink added to private state cannot escape os.Root.
	if err := os.Symlink("/etc/passwd", filepath.Join(repo, "sources", "escape.md")); err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, direct(t, s, "brain_read", map[string]any{"path": "sources/escape.md"})); got != "PATH_NOT_ALLOWED" {
		t.Fatal(got)
	}
}

func TestHTTPGuardsAndUnready(t *testing.T) {
	repo := fixture(t)
	s, err := New(Config{Brain: repo})
	if err != nil {
		t.Fatal(err)
	}
	for p, code := range map[string]int{"/livez": 200, "/readyz": 503} {
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", p, nil))
		if rr.Code != code {
			t.Fatalf("%s: %d", p, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Origin", "https://evil.example")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "http://evil.example/mcp", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:1234"
	s.Handler().ServeHTTP(rr, req)
	if rr.Code < 400 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(strings.Repeat(" ", requestMax+1)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code == 200 {
		t.Fatal("oversized request accepted")
	}
	c := httpClient(t, s)
	if call(t, c, "brain_status", map[string]any{})["ready"] != false {
		t.Fatal("unready status wrong")
	}
}

func TestHTTPInputSchemaValidation(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	c := httpClient(t, s)
	for _, args := range []map[string]any{{"query": ""}, {"query": 123}, {"query": "x", "limit": 1.5}, {"query": "x", "limit": 21}, {"query": "x", "revision": "HEAD"}, {"query": "x", "brain": "/etc"}, {"query": "x", "tags": []any{nil}}} {
		r, err := c.CallTool(t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "brain_search", Arguments: args}})
		if err == nil && !r.IsError {
			t.Fatalf("schema accepted %#v", args)
		}
	}
	// There is no CLI parser between MCP and the search engine.
	call(t, c, "brain_search", map[string]any{"query": "--help"})
}

func TestDirectBrainEditsAndIndexRefresh(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	c := httpClient(t, s)
	if _, err := os.Stat(filepath.Join(repo, ".git")); !os.IsNotExist(err) {
		t.Fatal("fixture should not be a Git repository")
	}
	braintest.RunWrite(t, repo, "# Latest\n\nnewunique latest content\n", "sources/latest.md", "--reason", "test", "--actor", "test")
	v := call(t, c, "brain_read", map[string]any{"path": "sources/latest.md"})
	if !strings.Contains(v["content"].(string), "newunique") {
		t.Fatal(v)
	}
	status := call(t, c, "brain_status", map[string]any{})
	if status["index_state"] != "stale" || status["ready"] != false {
		t.Fatal(status)
	}
	v = call(t, c, "brain_search", map[string]any{"query": "newunique"})
	if len(v["results"].([]any)) == 0 {
		t.Fatal("updated source missing from search")
	}
	if call(t, c, "brain_status", map[string]any{})["ready"] != true {
		t.Fatal("index not refreshed")
	}
	if err := os.Remove(searchindex.SearchIndexPath(repo)); err != nil {
		t.Fatal(err)
	}
	if call(t, c, "brain_status", map[string]any{})["index_state"] != "missing" {
		t.Fatal("missing index not reported")
	}
	call(t, c, "brain_search", map[string]any{"query": "newunique"})
}

func TestConcurrentSearchCancellationAndWriterLock(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	var wg sync.WaitGroup
	errs := make(chan string, 12)
	for range 12 {
		wg.Go(func() {
			r, err := s.search(t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query": "searchunique"}}})
			if err != nil || r.IsError {
				errs <- fmt.Sprintf("%v %#v", err, r)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	s.gate <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, _ := s.search(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query": "searchunique"}}})
	<-s.gate
	if errorCode(t, r) != "TIMEOUT" {
		t.Fatal("cancelled wait did not stop")
	}
	lock, err := brainlock.Acquire(repo, "test-write")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if errorCode(t, direct(t, s, "brain_read", map[string]any{"path": "wiki/topic.md"})) != "BUSY" {
		t.Fatal("read ignored writer lock")
	}
	r = direct(t, s, "brain_search", map[string]any{"query": "searchunique"})
	if !r.IsError {
		t.Fatal("search ignored writer lock")
	}
}

func TestInvalidBrainDoesNotRepairOrRebuild(t *testing.T) {
	repo := fixture(t)
	broken := braintest.ReadFile(t, repo, "tags.md") + "\nmanual drift\n"
	braintest.WriteFile(t, repo, "tags.md", broken)
	s, err := New(Config{Brain: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Prepare(t.Context()); err == nil {
		t.Fatal("invalid brain indexed")
	}
	if !direct(t, s, "brain_search", map[string]any{"query": "searchunique"}).IsError {
		t.Fatal("invalid brain searched")
	}
	if braintest.ReadFile(t, repo, "tags.md") != broken {
		t.Fatal("preparation repaired brain")
	}
}

func TestSymlinksWithinBrainAndCacheAreRejected(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	if err := os.Symlink("../AGENTS.md", filepath.Join(repo, "sources", "alias.md")); err != nil {
		t.Fatal(err)
	}
	if errorCode(t, direct(t, s, "brain_read", map[string]any{"path": "sources/alias.md"})) != "PATH_NOT_ALLOWED" {
		t.Fatal("instruction alias readable")
	}
	if err := os.RemoveAll(filepath.Join(repo, ".brain")); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(repo, ".brain")); err != nil {
		t.Fatal(err)
	}
	if !direct(t, s, "brain_search", map[string]any{"query": "searchunique"}).IsError {
		t.Fatal("symlink cache accepted")
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 0 {
		t.Fatal("external directory mutated")
	}
}

func TestWarmIndexReuseAndResponseLimit(t *testing.T) {
	repo := fixture(t)
	s := service(t, repo)
	before, err := os.Stat(searchindex.SearchIndexPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if direct(t, s, "brain_search", map[string]any{"query": "searchunique"}).IsError {
			t.Fatal("search failed")
		}
	}
	after, err := os.Stat(searchindex.SearchIndexPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("warm query rebuilt index")
	}
	r, err := result(map[string]any{"content": strings.Repeat("x", responseMax)})
	if err != nil || errorCode(t, r) != "RESPONSE_TOO_LARGE" {
		t.Fatal("oversized result accepted")
	}
}

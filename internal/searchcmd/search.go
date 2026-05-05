package searchcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain      string
	Query      string
	Limit      int
	Kind       string
	PathPrefix string
	JSON       bool
	Help       bool
}

type jsonOutput struct {
	Query                string       `json:"query"`
	QueryMode            string       `json:"query_mode"`
	Results              []jsonResult `json:"results"`
	RecommendedReadOrder []string     `json:"recommended_read_order"`
	StopRule             string       `json:"stop_rule"`
}

type jsonResult struct {
	ID        string   `json:"id"`
	SectionID string   `json:"section_id"`
	Path      string   `json:"path"`
	Anchor    string   `json:"anchor,omitempty"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Heading   string   `json:"heading,omitempty"`
	Summary   string   `json:"summary"`
	Tags      []string `json:"tags"`
	Score     float64  `json:"score"`
	Snippet   string   `json:"snippet"`
	Sources   []string `json:"sources"`
	Links     []string `json:"links"`
}

func Run(args []string) error {
	return RunWithOutput(args, os.Stdout)
}

func RunWithOutput(args []string, out io.Writer) error {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp(out)
		return err
	}
	if opts.Help {
		printHelp(out)
		return nil
	}

	brainDir, err := resolveBrain(opts.Brain)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := ensureSearchIndex(ctx, brainDir); err != nil {
		return err
	}

	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(brainDir))
	if err != nil {
		return fmt.Errorf("open search index: %w; run lumbrera index --rebuild --brain %s", err, brainDir)
	}
	defer db.Close()

	response, err := searchindex.Search(ctx, db, opts.Query, searchindex.SearchOptions{
		Limit:      opts.Limit,
		Kind:       opts.Kind,
		PathPrefix: opts.PathPrefix,
	})
	if err != nil {
		return err
	}
	return writeJSON(out, response)
}

func ensureSearchIndex(ctx context.Context, brainDir string) error {
	status, err := searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	switch status.State {
	case searchindex.StatusFresh:
		return nil
	case searchindex.StatusMissing, searchindex.StatusStale:
		return autoRebuild(ctx, brainDir, status.State)
	case searchindex.StatusIncompatible:
		return fmt.Errorf("search index is incompatible: %s; run lumbrera index --rebuild --brain %s", status.Reason, brainDir)
	default:
		return fmt.Errorf("search index has unknown status %q; run lumbrera index --status --brain %s", status.State, brainDir)
	}
}

func autoRebuild(ctx context.Context, brainDir string, state searchindex.StatusState) error {
	lock, err := brainlock.Acquire(brainDir, "search-index")
	if err != nil {
		return fmt.Errorf("search index is %s and automatic rebuild could not acquire lock: %w; run lumbrera index --rebuild --brain %s", state, err, brainDir)
	}
	defer func() { _ = lock.Release() }()

	status, err := searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	if status.State == searchindex.StatusFresh {
		return nil
	}
	if status.State == searchindex.StatusIncompatible {
		return fmt.Errorf("search index is incompatible: %s; run lumbrera index --rebuild --brain %s", status.Reason, brainDir)
	}
	if status.State != searchindex.StatusMissing && status.State != searchindex.StatusStale {
		return fmt.Errorf("search index has unknown status %q; run lumbrera index --status --brain %s", status.State, brainDir)
	}

	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("cannot automatically rebuild search index because brain verification failed: %w; run lumbrera verify --brain %s", err, brainDir)
	}
	if err := searchindex.RebuildBrain(ctx, brainDir); err != nil {
		return fmt.Errorf("search index is %s and automatic rebuild failed: %w; run lumbrera index --rebuild --brain %s", state, err, brainDir)
	}
	status, err = searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	if status.State != searchindex.StatusFresh {
		return fmt.Errorf("automatic rebuild completed but search index is %s: %s; run lumbrera index --rebuild --brain %s", status.State, status.Reason, brainDir)
	}
	return nil
}

func parseArgs(args []string) (options, error) {
	var opts options
	var queryParts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inlineValue, hasInlineValue := splitInlineFlag(arg)
		switch name {
		case "--help", "-h", "help":
			return options{Help: true}, nil
		case "--brain", "--repo":
			value, next, err := optionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Brain = value
			i = next
		case "--limit":
			value, next, err := optionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			limit, err := strconv.Atoi(value)
			if err != nil {
				return options{}, fmt.Errorf("invalid --limit %q", value)
			}
			opts.Limit = limit
			i = next
		case "--kind":
			value, next, err := optionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Kind = value
			i = next
		case "--path":
			value, next, err := optionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.PathPrefix = value
			i = next
		case "--json":
			if hasInlineValue {
				return options{}, fmt.Errorf("--json does not accept a value")
			}
			opts.JSON = true
		default:
			if strings.HasPrefix(arg, "--") {
				return options{}, fmt.Errorf("unknown search option %q", arg)
			}
			queryParts = append(queryParts, arg)
		}
	}

	opts.Query = strings.TrimSpace(strings.Join(queryParts, " "))
	if opts.Query == "" {
		return options{}, fmt.Errorf("search requires a query")
	}
	return opts, nil
}

func splitInlineFlag(arg string) (name, value string, ok bool) {
	if !strings.HasPrefix(arg, "--") {
		return arg, "", false
	}
	name, value, ok = strings.Cut(arg, "=")
	return name, value, ok
}

func optionValue(args []string, index int, flag, inlineValue string, hasInlineValue bool) (string, int, error) {
	if hasInlineValue {
		if strings.TrimSpace(inlineValue) == "" {
			return "", index, fmt.Errorf("%s requires a non-empty value", flag)
		}
		return inlineValue, index, nil
	}
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	value := args[index+1]
	if strings.TrimSpace(value) == "" {
		return "", index, fmt.Errorf("%s requires a non-empty value", flag)
	}
	return value, index + 1, nil
}

func resolveBrain(brainDir string) (string, error) {
	if strings.TrimSpace(brainDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		brainDir = cwd
	}
	abs, err := filepath.Abs(brainDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func writeJSON(out io.Writer, response searchindex.SearchResponse) error {
	payload := jsonOutput{
		Query:                response.Query,
		QueryMode:            response.QueryMode,
		Results:              make([]jsonResult, 0, len(response.Results)),
		RecommendedReadOrder: nonNilStrings(response.RecommendedReadOrder),
		StopRule:             response.StopRule,
	}
	for _, result := range response.Results {
		payload.Results = append(payload.Results, jsonResult{
			ID:        result.DocumentID,
			SectionID: result.SectionID,
			Path:      result.Path,
			Anchor:    result.Anchor,
			Kind:      result.Kind,
			Title:     result.Title,
			Heading:   result.Heading,
			Summary:   result.Summary,
			Tags:      nonNilStrings(result.Tags),
			Score:     result.Score,
			Snippet:   result.Snippet,
			Sources:   nonNilStrings(result.Sources),
			Links:     nonNilStrings(result.Links),
		})
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Search a Lumbrera brain with the local SQLite lexical index.

Usage:
  lumbrera search <query> [--brain <path>] [--limit <n>] [--kind all|wiki|source] [--path <prefix>] [--json]

Behavior:
  - output is JSON only in this version
  - missing or stale indexes are rebuilt automatically once
  - incompatible indexes require lumbrera index --rebuild

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --repo <path>       deprecated alias for --brain
  --limit <n>         max results, default 5, maximum 20
  --kind <value>      restrict to all, wiki, or source; default all
  --path <prefix>     restrict to a repo path prefix
  --json              accepted for compatibility; output is always JSON`)
}

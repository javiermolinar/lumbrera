package searchcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/cmdutil"
	"github.com/javiermolinar/lumbrera/internal/indexruntime"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
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
	Query                string                   `json:"query"`
	QueryMode            string                   `json:"query_mode"`
	RecommendedSections  []jsonRecommendedSection `json:"recommended_sections"`
	AgentInstructions    jsonAgentInstructions    `json:"agent_instructions"`
	Coverage             map[string]any           `json:"coverage"`
	Results              []jsonResult             `json:"results"`
	RecommendedReadOrder []string                 `json:"recommended_read_order"`
	StopRule             string                   `json:"stop_rule"`
}

type jsonAgentInstructions struct {
	ReadFirst string   `json:"read_first"`
	DoNot     []string `json:"do_not"`
	Fallback  string   `json:"fallback"`
}

type jsonRecommendedSection struct {
	SectionID string `json:"section_id"`
	Target    string `json:"target"`
	Path      string `json:"path"`
	Anchor    string `json:"anchor,omitempty"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Heading   string `json:"heading,omitempty"`
	Reason    string `json:"reason"`
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

	brainDir, err := cliutil.ResolveBrain(opts.Brain)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := indexruntime.EnsureFresh(ctx, brainDir); err != nil {
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

func parseArgs(args []string) (options, error) {
	var opts options
	var queryParts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inlineValue, hasInlineValue := cmdutil.SplitInlineFlag(arg)
		switch name {
		case "--help", "-h", "help":
			return options{Help: true}, nil
		case "--brain", "--repo":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Brain = value
			i = next
		case "--limit":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
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
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Kind = value
			i = next
		case "--path":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
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

func writeJSON(out io.Writer, response searchindex.SearchResponse) error {
	payload := jsonOutput{
		Query:               response.Query,
		QueryMode:           response.QueryMode,
		RecommendedSections: make([]jsonRecommendedSection, 0, len(response.RecommendedSections)),
		AgentInstructions: jsonAgentInstructions{
			ReadFirst: response.AgentInstructions.ReadFirst,
			DoNot:     cmdutil.NonNilStrings(response.AgentInstructions.DoNot),
			Fallback:  response.AgentInstructions.Fallback,
		},
		Coverage:             jsonCoverage(response.Coverage),
		Results:              make([]jsonResult, 0, len(response.Results)),
		RecommendedReadOrder: cmdutil.NonNilStrings(response.RecommendedReadOrder),
		StopRule:             response.StopRule,
	}
	for _, section := range response.RecommendedSections {
		payload.RecommendedSections = append(payload.RecommendedSections, jsonRecommendedSection{
			SectionID: section.SectionID,
			Target:    section.Target,
			Path:      section.Path,
			Anchor:    section.Anchor,
			Kind:      section.Kind,
			Title:     section.Title,
			Heading:   section.Heading,
			Reason:    section.Reason,
		})
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
			Tags:      cmdutil.NonNilStrings(result.Tags),
			Score:     result.Score,
			Snippet:   result.Snippet,
			Sources:   cmdutil.NonNilStrings(result.Sources),
			Links:     cmdutil.NonNilStrings(result.Links),
		})
	}
	return cmdutil.WriteJSON(out, payload)
}

func jsonCoverage(coverage searchindex.SearchCoverage) map[string]any {
	payload := make(map[string]any, len(coverage.Entities)+1)
	for key, value := range coverage.Entities {
		payload[key] = value
	}
	payload["missing"] = cmdutil.NonNilStrings(coverage.Missing)
	return payload
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Search a Lumbrera brain with the local SQLite lexical index.

Usage:
  lumbrera search <query> [--brain <path>] [--limit <n>] [--kind all|wiki|source] [--path <prefix>] [--json]

Behavior:
  - output is JSON only in this version
  - recommended_sections is the primary deterministic read plan for agents
  - agent_instructions and coverage describe safe fallback and entity coverage
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

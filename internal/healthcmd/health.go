package healthcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain      string
	Kind       string
	Limit      int
	PathPrefix string
	TargetPath string
	JSON       bool
	Help       bool
}

type jsonOutput struct {
	Candidates []jsonCandidate `json:"candidates"`
	StopRule   string          `json:"stop_rule"`
}

type jsonCandidate struct {
	Type              string       `json:"type"`
	Confidence        string       `json:"confidence"`
	Score             float64      `json:"score"`
	Pages             []string     `json:"pages"`
	Sources           []string     `json:"sources"`
	Reasons           []jsonReason `json:"reasons"`
	SuggestedQueries  []string     `json:"suggested_queries"`
	ReviewInstruction string       `json:"review_instruction"`
}

type jsonReason struct {
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
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
	if err := ensureSearchIndex(ctx, brainDir); err != nil {
		return err
	}

	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(brainDir))
	if err != nil {
		return fmt.Errorf("open search index: %w; run lumbrera index --rebuild --brain %s", err, brainDir)
	}
	defer db.Close()

	pathPrefix := opts.PathPrefix
	if opts.TargetPath != "" {
		pathPrefix = opts.TargetPath
	}
	response, err := searchindex.HealthCandidates(ctx, db, searchindex.CandidateOptions{
		Limit:      opts.Limit,
		Kind:       opts.Kind,
		PathPrefix: pathPrefix,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, response)
	}
	return writeHuman(out, response)
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

	if err := verify.Check(brainDir, verify.Options{}); err != nil {
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
	var positional []string
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
				return options{}, fmt.Errorf("unknown health option %q", arg)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) > 1 {
		return options{}, fmt.Errorf("health accepts at most one positional wiki/source path")
	}
	if len(positional) == 1 {
		if strings.TrimSpace(opts.PathPrefix) != "" {
			return options{}, fmt.Errorf("use either --path or a positional path, not both")
		}
		path, _, err := pathpolicy.NormalizeTargetPath(positional[0])
		if err != nil {
			return options{}, err
		}
		opts.TargetPath = path
	}
	if !isValidCandidateKind(opts.Kind) {
		return options{}, fmt.Errorf("invalid candidate kind %q", opts.Kind)
	}
	return opts, nil
}

func isValidCandidateKind(kind string) bool {
	switch kind {
	case "", searchindex.CandidateKindAll, searchindex.CandidateKindDuplicates, searchindex.CandidateKindLinks, searchindex.CandidateKindSources, searchindex.CandidateKindOrphans:
		return true
	default:
		return false
	}
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

func writeJSON(out io.Writer, response searchindex.CandidateResponse) error {
	payload := jsonOutput{
		Candidates: make([]jsonCandidate, 0, len(response.Candidates)),
		StopRule:   response.StopRule,
	}
	for _, candidate := range response.Candidates {
		item := jsonCandidate{
			Type:              candidate.Type,
			Confidence:        candidate.Confidence,
			Score:             candidate.Score,
			Pages:             nonNilStrings(candidate.Pages),
			Sources:           nonNilStrings(candidate.Sources),
			Reasons:           make([]jsonReason, 0, len(candidate.Reasons)),
			SuggestedQueries:  nonNilStrings(candidate.SuggestedQueries),
			ReviewInstruction: candidate.ReviewInstruction,
		}
		for _, reason := range candidate.Reasons {
			item.Reasons = append(item.Reasons, jsonReason{Code: reason.Code, Value: reason.Value})
		}
		payload.Candidates = append(payload.Candidates, item)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

func writeHuman(out io.Writer, response searchindex.CandidateResponse) error {
	if len(response.Candidates) == 0 {
		_, err := fmt.Fprintf(out, "No health candidates found.\nstop_rule: %s\n", response.StopRule)
		return err
	}
	for i, candidate := range response.Candidates {
		if _, err := fmt.Fprintf(out, "%d. %s %s score=%.3f\n", i+1, candidate.Type, candidate.Confidence, candidate.Score); err != nil {
			return err
		}
		if len(candidate.Pages) > 0 {
			if _, err := fmt.Fprintf(out, "   pages: %s\n", strings.Join(candidate.Pages, ", ")); err != nil {
				return err
			}
		}
		if len(candidate.Sources) > 0 {
			if _, err := fmt.Fprintf(out, "   sources: %s\n", strings.Join(candidate.Sources, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "   reasons: %s\n", formatReasons(candidate.Reasons)); err != nil {
			return err
		}
		if len(candidate.SuggestedQueries) > 0 {
			if _, err := fmt.Fprintf(out, "   next: %s; %s\n", formatSuggestedQueries(candidate.SuggestedQueries), candidate.ReviewInstruction); err != nil {
				return err
			}
		} else if candidate.ReviewInstruction != "" {
			if _, err := fmt.Fprintf(out, "   next: %s\n", candidate.ReviewInstruction); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(out, "\nstop_rule: %s\n", response.StopRule)
	return err
}

func formatReasons(reasons []searchindex.CandidateReason) string {
	if len(reasons) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason.Value == "" {
			parts = append(parts, reason.Code)
			continue
		}
		parts = append(parts, reason.Code+"="+reason.Value)
	}
	return strings.Join(parts, ", ")
}

func formatSuggestedQueries(queries []string) string {
	parts := make([]string, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("search %q", query))
	}
	return strings.Join(parts, "; ")
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Return deterministic Lumbrera health/consolidation review candidates.

Usage:
  lumbrera health [wiki/page.md|sources/source.md] [--brain <path>] [--path <prefix>] [--kind all|duplicates|links|sources|orphans] [--limit <n>] [--json]

Behavior:
  - candidates are deterministic review hints, not semantic drift diagnoses
  - output is read-only and does not mutate brain Markdown
  - missing or stale indexes are rebuilt automatically once
  - incompatible indexes require lumbrera index --rebuild

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --repo <path>       deprecated alias for --brain
  --path <prefix>     restrict to a repo path prefix
  --kind <value>      restrict to all, duplicates, links, sources, or orphans; default all
  --limit <n>         max candidates, default 10, maximum 50
  --json              emit JSON instead of compact human output`)
}

package searchcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/deletecmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

func TestSearchAutoRebuildsMissingIndexAndOutputsJSON(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes mention searchunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Search topic\n\nSearch body mentions searchunique.\n", "wiki/search-topic.md", "--title", "Search topic", "--summary", "Search topic summary.", "--tag", "search", "--source", "sources/raw.md", "--reason", "Create search topic", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"searchunique", "--brain", repo, "--json"}, &out); err != nil {
		t.Fatalf("search failed: %v", err)
	}
	payload := decodeOutput(t, out.Bytes())
	if payload.Query != "searchunique" || payload.QueryMode != searchindex.QueryModeAND {
		t.Fatalf("unexpected payload metadata: %#v", payload)
	}
	if len(payload.Results) == 0 {
		t.Fatalf("search returned no results: %s", out.String())
	}
	if len(payload.RecommendedReadOrder) == 0 || len(payload.RecommendedSections) == 0 || payload.StopRule == "" {
		t.Fatalf("missing recommended read order/sections/stop rule: %#v", payload)
	}
	if payload.RecommendedSections[0].Reason == "" {
		t.Fatalf("recommended section missing reason: %#v", payload.RecommendedSections[0])
	}
	if payload.AgentInstructions.ReadFirst != "recommended_sections" || len(payload.AgentInstructions.DoNot) == 0 || payload.AgentInstructions.Fallback == "" {
		t.Fatalf("missing agent instructions: %#v", payload.AgentInstructions)
	}
	if _, ok := payload.Coverage["missing"]; !ok {
		t.Fatalf("coverage missing 'missing' field: %#v", payload.Coverage)
	}
	if _, err := os.Stat(searchindex.SearchIndexPath(repo)); err != nil {
		t.Fatalf("search should auto-create index: %v", err)
	}
}

func TestSearchAutoRebuildsStaleIndex(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes mention oldunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody mentions oldunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	var first bytes.Buffer
	if err := RunWithOutput([]string{"oldu unique", "--brain", repo}, &first); err != nil {
		// The query intentionally has no match. It is only used to trigger initial rebuild.
		t.Fatalf("initial search failed: %v", err)
	}
	braintest.RunWrite(t, repo, "# Unreferenced\n\nThis unreferenced source mentions freshunique.\n", "sources/unreferenced.md", "--reason", "Preserve unreferenced source", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"freshunique", "--brain", repo}, &out); err != nil {
		t.Fatalf("stale auto-rebuild search failed: %v", err)
	}
	payload := decodeOutput(t, out.Bytes())
	if len(payload.Results) == 0 || payload.Results[0].Path != "sources/unreferenced.md" {
		t.Fatalf("unexpected stale rebuild results: %#v", payload.Results)
	}
}

func TestSearchRejectsVerifyDriftDuringAutoRebuild(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
	braintest.WriteFile(t, repo, "tags.md", braintest.ReadFile(t, repo, "tags.md")+"\nManual drift.\n")

	var out bytes.Buffer
	err := RunWithOutput([]string{"topic", "--brain", repo}, &out)
	if err == nil {
		t.Fatal("search succeeded with verify drift, want error")
	}
	if !strings.Contains(err.Error(), "verify") || !strings.Contains(err.Error(), "tags.md") {
		t.Fatalf("search error = %v, want verify tags.md error", err)
	}
}

func TestSearchFiltersAndFlagsAfterQuery(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes mention filterunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody mentions filterunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"filterunique", "--brain=" + repo, "--kind=wiki", "--path=wiki/", "--tag=topic", "--source", "sources/raw.md", "--limit=1"}, &out); err != nil {
		t.Fatalf("filtered search failed: %v", err)
	}
	payload := decodeOutput(t, out.Bytes())
	if len(payload.Results) != 1 {
		t.Fatalf("result count = %d, want 1: %#v", len(payload.Results), payload.Results)
	}
	if payload.Results[0].Kind != searchindex.KindWiki || !strings.HasPrefix(payload.Results[0].Path, "wiki/") {
		t.Fatalf("unexpected filtered result: %#v", payload.Results[0])
	}
}

func TestSearchNotesDefaultFilterRankingAndFreshness(t *testing.T) {
	repo := braintest.InitBrain(t)
	var bootstrap bytes.Buffer
	if err := RunWithOutput([]string{"bootstrapunique", "--brain", repo}, &bootstrap); err != nil {
		t.Fatalf("build initial search index: %v", err)
	}
	status, err := searchindex.CheckStatus(context.Background(), repo)
	if err != nil || status.State != searchindex.StatusFresh {
		t.Fatalf("initial search status = %#v err=%v, want fresh", status, err)
	}

	braintest.RunWrite(t, repo, "# Observation\n\nLifecycleunique initial behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "Lifecycleunique first-party behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	status, err = searchindex.CheckStatus(context.Background(), repo)
	if err != nil || status.State != searchindex.StatusStale {
		t.Fatalf("status after note create = %#v err=%v, want stale", status, err)
	}
	braintest.RunWrite(t, repo, "# Guidance\n\nLifecycleunique canonical guidance.\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Lifecycleunique canonical guidance.", "--tag", "operations", "--source", "notes/observation.md",
		"--reason", "Create guidance", "--actor", "test")

	var initial bytes.Buffer
	if err := RunWithOutput([]string{"lifecycleunique", "--brain", repo}, &initial); err != nil {
		t.Fatalf("default note search: %v", err)
	}
	payload := decodeOutput(t, initial.Bytes())
	if len(payload.Results) < 2 || payload.Results[0].Kind != searchindex.KindWiki {
		t.Fatalf("default search did not rank wiki first: %#v", payload.Results)
	}
	foundNote := false
	for _, result := range payload.Results {
		if result.Kind == searchindex.KindNote {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("default search omitted note: %#v", payload.Results)
	}

	var filtered bytes.Buffer
	if err := RunWithOutput([]string{"lifecycleunique", "--brain", repo, "--kind", "note"}, &filtered); err != nil {
		t.Fatalf("note-filtered search: %v", err)
	}
	for _, result := range decodeOutput(t, filtered.Bytes()).Results {
		if result.Kind != searchindex.KindNote {
			t.Fatalf("--kind note returned %#v", result)
		}
	}

	var byEvidence bytes.Buffer
	if err := RunWithOutput([]string{"lifecycleunique", "--brain", repo, "--source", "notes/observation.md"}, &byEvidence); err != nil {
		t.Fatalf("note evidence filter: %v", err)
	}
	evidenceResults := decodeOutput(t, byEvidence.Bytes()).Results
	if len(evidenceResults) == 0 {
		t.Fatal("note evidence filter returned no wiki result")
	}
	for _, result := range evidenceResults {
		if result.Kind != searchindex.KindWiki || result.Path != "wiki/guidance.md" {
			t.Fatalf("note evidence filter returned %#v", result)
		}
	}

	braintest.RunWrite(t, repo, "# Observation\n\nFreshnoteunique updated behavior.\n", "notes/observation.md",
		"--reason", "Update observation", "--actor", "test")
	status, err = searchindex.CheckStatus(context.Background(), repo)
	if err != nil || status.State != searchindex.StatusStale {
		t.Fatalf("status after note update = %#v err=%v, want stale", status, err)
	}
	var updated bytes.Buffer
	if err := RunWithOutput([]string{"freshnoteunique", "--brain", repo, "--kind", "note"}, &updated); err != nil {
		t.Fatalf("search updated note: %v", err)
	}
	if results := decodeOutput(t, updated.Bytes()).Results; len(results) == 0 || results[0].Path != "notes/observation.md" {
		t.Fatalf("updated note not indexed: %#v", results)
	}

	if err := deletecmd.Run([]string{"wiki/guidance.md", "--brain", repo, "--reason", "Remove guidance", "--actor", "test"}); err != nil {
		t.Fatalf("delete guidance: %v", err)
	}
	if err := deletecmd.Run([]string{"notes/observation.md", "--brain", repo, "--reason", "Remove observation", "--actor", "test"}); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	status, err = searchindex.CheckStatus(context.Background(), repo)
	if err != nil || status.State != searchindex.StatusStale {
		t.Fatalf("status after note delete = %#v err=%v, want stale", status, err)
	}
	var deleted bytes.Buffer
	if err := RunWithOutput([]string{"freshnoteunique", "--brain", repo}, &deleted); err != nil {
		t.Fatalf("search after note delete: %v", err)
	}
	for _, result := range decodeOutput(t, deleted.Bytes()).Results {
		if result.Path == "notes/observation.md" {
			t.Fatalf("deleted note remained indexed: %#v", result)
		}
	}
}

func TestSearchInvalidArgsAndHelp(t *testing.T) {
	if err := RunWithOutput(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("search without query succeeded, want error")
	}
	if err := RunWithOutput([]string{"query", "--unknown"}, &bytes.Buffer{}); err == nil {
		t.Fatal("search with unknown flag succeeded, want error")
	}
	if err := RunWithOutput([]string{"query", "--json=false"}, &bytes.Buffer{}); err == nil {
		t.Fatal("search with --json value succeeded, want error")
	}
	if err := RunWithOutput([]string{"--help"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("search help failed: %v", err)
	}
}

func decodeOutput(t *testing.T, content []byte) jsonOutput {
	t.Helper()
	var payload jsonOutput
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode JSON output %q: %v", string(content), err)
	}
	return payload
}

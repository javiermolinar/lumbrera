package searchcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestSearchAutoRebuildsMissingIndexAndOutputsJSON(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention searchunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Search topic\n\nSearch body mentions searchunique.\n", "wiki/search-topic.md", "--title", "Search topic", "--summary", "Search topic summary.", "--tag", "search", "--source", "sources/raw.md", "--reason", "Create search topic", "--actor", "test")

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
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention oldunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody mentions oldunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	var first bytes.Buffer
	if err := RunWithOutput([]string{"oldu unique", "--brain", repo}, &first); err != nil {
		// The query intentionally has no match. It is only used to trigger initial rebuild.
		t.Fatalf("initial search failed: %v", err)
	}
	writeFile(t, repo, "sources/unreferenced.md", "# Unreferenced\n\nThis unreferenced source mentions freshunique.\n")

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
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
	writeFile(t, repo, "tags.md", readFile(t, repo, "tags.md")+"\nManual drift.\n")

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
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention filterunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody mentions filterunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"filterunique", "--brain=" + repo, "--kind=wiki", "--path=wiki/", "--limit=1"}, &out); err != nil {
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

func initBrain(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return repo
}

func runWrite(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--brain", repo}, args...)
	if err := writecmd.Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	absPath := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func readFile(t *testing.T, repo, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
}

func decodeOutput(t *testing.T, content []byte) jsonOutput {
	t.Helper()
	var payload jsonOutput
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode JSON output %q: %v", string(content), err)
	}
	return payload
}

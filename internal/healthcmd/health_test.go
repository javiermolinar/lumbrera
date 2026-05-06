package healthcmd

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

func TestHealthAutoRebuildsMissingIndexAndOutputsJSON(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention healthunique retention.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# First topic\n\nHealthunique retention compaction notes.\n", "wiki/topic-first.md", "--title", "First topic", "--summary", "Retention compaction notes.", "--tag", "retention", "--tag", "healthunique", "--source", "sources/raw.md", "--reason", "Create first topic", "--actor", "test")
	runWrite(t, repo, "# Second topic\n\nHealthunique retention compaction overlap.\n", "wiki/topic-second.md", "--title", "Second topic", "--summary", "Retention compaction overlap.", "--tag", "retention", "--tag", "healthunique", "--source", "sources/raw.md", "--reason", "Create second topic", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"--brain", repo, "--kind", "duplicates", "--limit", "1", "--json"}, &out); err != nil {
		t.Fatalf("health failed: %v", err)
	}
	payload := decodeOutput(t, out.Bytes())
	if len(payload.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %s", len(payload.Candidates), out.String())
	}
	candidate := payload.Candidates[0]
	if candidate.Type != searchindex.CandidateTypePossibleDuplicate || candidate.Confidence == "" || candidate.Score == 0 {
		t.Fatalf("unexpected candidate metadata: %#v", candidate)
	}
	if len(candidate.Pages) != 2 || len(candidate.Reasons) == 0 || len(candidate.SuggestedQueries) == 0 || candidate.ReviewInstruction == "" {
		t.Fatalf("candidate missing expected fields: %#v", candidate)
	}
	if payload.StopRule == "" {
		t.Fatal("missing stop_rule")
	}
	if _, err := os.Stat(searchindex.SearchIndexPath(repo)); err != nil {
		t.Fatalf("health should auto-create index: %v", err)
	}
}

func TestHealthJSONOutputContractFixture(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention fixtureunique retention.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# First topic\n\nFixtureunique retention compaction notes.\n", "wiki/topic-first.md", "--title", "First topic", "--summary", "Retention compaction notes.", "--tag", "retention", "--tag", "fixtureunique", "--source", "sources/raw.md", "--reason", "Create first topic", "--actor", "test")
	runWrite(t, repo, "# Second topic\n\nFixtureunique retention compaction overlap.\n", "wiki/topic-second.md", "--title", "Second topic", "--summary", "Retention compaction overlap.", "--tag", "retention", "--tag", "fixtureunique", "--source", "sources/raw.md", "--reason", "Create second topic", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"--brain", repo, "--kind", "duplicates", "--limit", "1", "--json"}, &out); err != nil {
		t.Fatalf("health failed: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "health_duplicates.golden.json"))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	if out.String() != string(want) {
		t.Fatalf("health JSON output mismatch\nwant:\n%s\ngot:\n%s", string(want), out.String())
	}
}

func TestHealthHumanOutputAndPositionalFilter(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention humanunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Alpha\n\nHumanunique alpha retention.\n", "wiki/topic-alpha.md", "--title", "Alpha", "--summary", "Humanunique alpha.", "--tag", "humanunique", "--source", "sources/raw.md", "--reason", "Create alpha", "--actor", "test")
	runWrite(t, repo, "# Beta\n\nHumanunique beta retention.\n", "wiki/topic-beta.md", "--title", "Beta", "--summary", "Humanunique beta.", "--tag", "humanunique", "--source", "sources/raw.md", "--reason", "Create beta", "--actor", "test")
	runWrite(t, repo, "# Gamma\n\nOther material.\n", "wiki/gamma.md", "--title", "Gamma", "--summary", "Gamma.", "--tag", "gamma", "--source", "sources/raw.md", "--reason", "Create gamma", "--actor", "test")

	var out bytes.Buffer
	if err := RunWithOutput([]string{"wiki/topic-alpha.md", "--brain", repo, "--limit", "5"}, &out); err != nil {
		t.Fatalf("health failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "wiki/topic-alpha.md") || !strings.Contains(text, "reasons:") || !strings.Contains(text, "stop_rule:") {
		t.Fatalf("unexpected human output:\n%s", text)
	}
	if strings.Contains(text, "wiki/gamma.md, wiki/topic-beta.md") || strings.Contains(text, "wiki/topic-beta.md, wiki/gamma.md") {
		t.Fatalf("positional filter returned unrelated gamma/beta pair:\n%s", text)
	}
}

func TestHealthInvalidArgsAndHelp(t *testing.T) {
	if err := RunWithOutput([]string{"--unknown"}, &bytes.Buffer{}); err == nil {
		t.Fatal("health with unknown flag succeeded, want error")
	}
	if err := RunWithOutput([]string{"--json=false"}, &bytes.Buffer{}); err == nil {
		t.Fatal("health with --json value succeeded, want error")
	}
	if err := RunWithOutput([]string{"--kind", "bad"}, &bytes.Buffer{}); err == nil {
		t.Fatal("health with invalid kind succeeded, want error")
	}
	if err := RunWithOutput([]string{"wiki/a.md", "wiki/b.md"}, &bytes.Buffer{}); err == nil {
		t.Fatal("health with two positional paths succeeded, want error")
	}
	if err := RunWithOutput([]string{"wiki/a.md", "--path", "wiki/"}, &bytes.Buffer{}); err == nil {
		t.Fatal("health with positional path and --path succeeded, want error")
	}
	if err := RunWithOutput([]string{"--help"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("health help failed: %v", err)
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

func decodeOutput(t *testing.T, content []byte) jsonOutput {
	t.Helper()
	var payload jsonOutput
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode JSON output %q: %v", string(content), err)
	}
	return payload
}

package writecmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EInitSourceWriteWikiWriteInTmp(t *testing.T) {
	tmp, err := os.MkdirTemp("/tmp", "lumbrera-e2e-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "lumbrera")
	runCommand(t, root, "", "go", "build", "-o", bin, "./cmd/lumbrera")

	repo := filepath.Join(tmp, "brain")
	runCommand(t, root, "", bin, "init", repo)
	runCommand(t, repo, "# E2E source\n\nThis source describes Lumbrera write behavior.\n", bin, "write", "sources/2026/05/04/e2e-source.md", "--brain", repo, "--reason", "Preserve E2E source", "--actor", "e2e")
	runCommand(t, repo, "# E2E write page\n\nThe write command preserves sources and creates wiki pages.\n", bin, "write", "wiki/e2e-write-page.md", "--brain", repo, "--title", "E2E write page", "--summary", "E2E write behavior summary.", "--source", "sources/2026/05/04/e2e-source.md", "--reason", "Distill E2E source", "--actor", "e2e", "--tag", "e2e")
	runCommand(t, repo, "", bin, "verify", "--brain", repo)

	assertFileContains(t, repo, "wiki/e2e-write-page.md", "schema: document-v1")
	assertFileContains(t, repo, "wiki/e2e-write-page.md", "## Sources")
	assertFileContains(t, repo, "SOURCES.md", "[E2E source](sources/2026/05/04/e2e-source.md)")
	assertFileContains(t, repo, "INDEX.md", "[E2E write page](wiki/e2e-write-page.md)")
	assertFileNotContains(t, repo, "BRAIN.sum", "sources/2026/05/04/e2e-source.md sha256:")
	assertFileContains(t, repo, "BRAIN.sum", "wiki/e2e-write-page.md sha256:")
	assertFileContains(t, repo, "CHANGELOG.md", "[source] [e2e]: Preserve E2E source")
	assertFileContains(t, repo, "CHANGELOG.md", "[create] [e2e]: Distill E2E source")
	assertFileContains(t, repo, "tags.md", "- e2e (1)")

	// The v3 acceptance path needs no external source: record a first-party note,
	// synthesize a wiki page backed only by that note, verify and search it, update
	// the note, then delete the sole evidence and cascade-delete the wiki page.
	runCommand(t, repo, "# Queue observation\n\nRestarting resumed the queue.\n", bin, "write", "notes/queue-observation.md", "--brain", repo, "--title", "Queue observation", "--summary", "Restarting resumed the stuck queue.", "--tag", "operations", "--reason", "Record queue observation", "--actor", "e2e")
	runCommand(t, repo, "# Queue recovery\n\nRestart the worker after confirming the queue is stuck.\n", bin, "write", "wiki/queue-recovery.md", "--brain", repo, "--title", "Queue recovery", "--summary", "Recover a stuck queue by restarting its worker.", "--tag", "operations", "--source", "notes/queue-observation.md", "--reason", "Synthesize note evidence", "--actor", "e2e")
	runCommand(t, repo, "", bin, "verify", "--brain", repo)
	results := runCommandOutput(t, repo, "", bin, "search", "restart stuck queue", "--brain", repo, "--json")
	var searchPayload struct {
		Results []struct {
			Path string `json:"path"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(results), &searchPayload); err != nil {
		t.Fatalf("decode search output: %v\n%s", err, results)
	}
	wikiResult, noteResult := -1, -1
	for i, result := range searchPayload.Results {
		switch result.Path {
		case "wiki/queue-recovery.md":
			if wikiResult < 0 {
				wikiResult = i
			}
		case "notes/queue-observation.md":
			if noteResult < 0 {
				noteResult = i
			}
		}
	}
	if wikiResult < 0 || noteResult < 0 || wikiResult > noteResult {
		t.Fatalf("default search did not return wiki before note:\n%s", results)
	}
	assertFileContains(t, repo, "NOTES.md", "[Queue observation](notes/queue-observation.md)")
	assertFileContains(t, repo, "wiki/queue-recovery.md", "notes/queue-observation.md")
	assertFileContains(t, repo, "BRAIN.sum", "notes/queue-observation.md sha256:")

	runCommand(t, repo, "# Queue observation\n\nRestarting resumed the queue after ownership release.\n", bin, "write", "notes/queue-observation.md", "--brain", repo, "--reason", "Clarify queue observation", "--actor", "e2e")
	runCommand(t, repo, "", bin, "verify", "--brain", repo)
	updatedResults := runCommandOutput(t, repo, "", bin, "search", "ownership release", "--brain", repo, "--kind", "note", "--json")
	if !strings.Contains(updatedResults, `"path": "notes/queue-observation.md"`) {
		t.Fatalf("updated note was not searchable:\n%s", updatedResults)
	}

	runCommand(t, repo, "", bin, "delete", "notes/queue-observation.md", "--brain", repo, "--reason", "Remove disproven observation", "--actor", "e2e")
	runCommand(t, repo, "", bin, "verify", "--brain", repo)
	assertMissing(t, repo, "notes/queue-observation.md")
	assertMissing(t, repo, "wiki/queue-recovery.md")
}

func runCommand(t *testing.T, dir, stdin, name string, args ...string) {
	t.Helper()
	_ = runCommandOutput(t, dir, stdin, name, args...)
}

func runCommandOutput(t *testing.T, dir, stdin, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

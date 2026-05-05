package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/verify"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestCheckDoesNotRepairMissingWikiDocumentID(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	withoutID := removeIDLine(readFile(t, path))
	if err := os.WriteFile(path, []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verify.Check(repo, verify.Options{}); err == nil {
		t.Fatal("Check repaired or accepted a missing document ID; want pure validation error")
	}
	if strings.Contains(readFile(t, path), "id: doc_") {
		t.Fatal("Check mutated the wiki file by repairing the missing ID")
	}

	if err := verify.Run(repo, verify.Options{}); err != nil {
		t.Fatalf("Run should still repair missing document ID: %v", err)
	}
	if !strings.Contains(readFile(t, path), "id: doc_") {
		t.Fatal("Run did not repair missing document ID")
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func removeIDLine(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "id: doc_") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

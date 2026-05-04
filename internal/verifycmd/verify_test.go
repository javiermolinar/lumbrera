package verifycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestVerifyPassesForInitializedBrain(t *testing.T) {
	repo := initBrain(t)

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestVerifyRejectsManifestDrift(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	content := strings.Replace(readFile(t, repo, "wiki/topic.md"), "Body.", "Changed body.", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo})
	if err == nil {
		t.Fatal("expected verify to reject BRAIN.sum drift")
	}
	if !strings.Contains(err.Error(), "BRAIN.sum") {
		t.Fatalf("expected BRAIN.sum drift error, got %v", err)
	}
}

func TestVerifyRepairsMissingWikiDocumentID(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	withoutID := removeIDLine(readFile(t, repo, "wiki/topic.md"))
	if err := os.WriteFile(filepath.Join(repo, "wiki", "topic.md"), []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should repair missing id: %v", err)
	}
	if !strings.Contains(readFile(t, repo, "wiki/topic.md"), "id: doc_") {
		t.Fatal("expected verify to add generated document id")
	}
}

func TestVerifyRejectsDuplicateWikiDocumentIDs(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# First\n\nBody.\n", "wiki/first.md", "--title", "First", "--summary", "First summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create first", "--actor", "test")
	runWrite(t, repo, "# Second\n\nBody.\n", "wiki/second.md", "--title", "Second", "--summary", "Second summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create second", "--actor", "test")

	firstID := idLine(readFile(t, repo, "wiki/first.md"))
	second := replaceIDLine(readFile(t, repo, "wiki/second.md"), firstID)
	if err := os.WriteFile(filepath.Join(repo, "wiki", "second.md"), []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo})
	if err == nil {
		t.Fatal("expected duplicate document id to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicates Lumbrera document id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestVerifyRejectsOversizedWikiPage(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	content := strings.Replace(readFile(t, repo, "wiki/topic.md"), "Body.", strings.Repeat("Line.\n", 401), 1)
	if err := os.WriteFile(filepath.Join(repo, "wiki", "topic.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo})
	if err == nil {
		t.Fatal("expected oversized wiki page to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds max wiki page length") {
		t.Fatalf("expected max length error, got %v", err)
	}
}

func TestVerifyAllowsRawSourceWithoutGeneratedFrontmatter(t *testing.T) {
	repo := initBrain(t)
	path := filepath.Join(repo, "sources", "raw.md")
	if err := os.WriteFile(path, []byte("# Raw source\n\nRaw notes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should ignore raw source frontmatter: %v", err)
	}
}

func TestVerifyRejectsUnexpectedRootMarkdown(t *testing.T) {
	repo := initBrain(t)
	if err := os.WriteFile(filepath.Join(repo, "rogue.md"), []byte("# Rogue\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo})
	if err == nil {
		t.Fatal("expected verify to reject root Markdown")
	}
	if !strings.Contains(err.Error(), "rogue.md") {
		t.Fatalf("expected error to mention rogue.md, got %v", err)
	}
}

func TestVerifyRejectsChangelogDrift(t *testing.T) {
	repo := initBrain(t)
	changelog := readFile(t, repo, "CHANGELOG.md") + "2026-05-04 [source] [test]: Pending source\n"
	if err := os.WriteFile(filepath.Join(repo, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err == nil {
		t.Fatal("expected verify to reject changelog drift")
	}
}

func TestVerifyRejectsTagsDrift(t *testing.T) {
	repo := initBrain(t)
	tags := readFile(t, repo, "tags.md") + "\nManual tag edit.\n"
	if err := os.WriteFile(filepath.Join(repo, "tags.md"), []byte(tags), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo})
	if err == nil {
		t.Fatal("expected verify to reject tags.md drift")
	}
	if !strings.Contains(err.Error(), "tags.md") {
		t.Fatalf("expected error to mention tags.md, got %v", err)
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

func readFile(t *testing.T, repo, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
}

func idLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "id: doc_") {
			return line
		}
	}
	return ""
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

func replaceIDLine(content, replacement string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "id: doc_") {
			lines = append(lines, replacement)
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

package verifycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/braintest"
)

func TestVerifyPassesForInitializedBrain(t *testing.T) {
	repo := braintest.InitBrain(t)

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestVerifyFixRegeneratesStaleIndex(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")

	// Manually add a source without lumbrera write.
	if err := os.WriteFile(filepath.Join(repo, "sources", "extra.md"), []byte("# Extra\n\nExtra notes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --fix, should fail.
	if err := Run([]string{"--brain", repo}); err == nil {
		t.Fatal("expected verify to fail with stale INDEX.md")
	}

	// With --fix, should pass.
	if err := Run([]string{"--brain", repo, "--fix"}); err != nil {
		t.Fatalf("verify --fix should regenerate stale files: %v", err)
	}

	// Subsequent verify without --fix should pass.
	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should pass after fix: %v", err)
	}

	// INDEX.md should include the manually-added source.
	index := braintest.ReadFile(t, repo, "INDEX.md")
	if !strings.Contains(index, "sources/extra.md") {
		t.Fatalf("expected INDEX.md to include extra.md, got:\n%s", index)
	}
}

func TestVerifyRejectsManifestDrift(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	content := strings.Replace(braintest.ReadFile(t, repo, "wiki/topic.md"), "Body.", "Changed body.", 1)
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
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	withoutID := removeIDLine(braintest.ReadFile(t, repo, "wiki/topic.md"))
	if err := os.WriteFile(filepath.Join(repo, "wiki", "topic.md"), []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should repair missing id: %v", err)
	}
	if !strings.Contains(braintest.ReadFile(t, repo, "wiki/topic.md"), "id: doc_") {
		t.Fatal("expected verify to add generated document id")
	}
}

func TestVerifyRejectsDuplicateWikiDocumentIDs(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# First\n\nBody.\n", "wiki/first.md", "--title", "First", "--summary", "First summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create first", "--actor", "test")
	braintest.RunWrite(t, repo, "# Second\n\nBody.\n", "wiki/second.md", "--title", "Second", "--summary", "Second summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create second", "--actor", "test")

	firstID := idLine(braintest.ReadFile(t, repo, "wiki/first.md"))
	second := replaceIDLine(braintest.ReadFile(t, repo, "wiki/second.md"), firstID)
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
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	content := strings.Replace(braintest.ReadFile(t, repo, "wiki/topic.md"), "Body.", strings.Repeat("Line.\n", 401), 1)
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
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	if strings.HasPrefix(braintest.ReadFile(t, repo, "sources/raw.md"), "---") {
		t.Fatal("source write should preserve raw source without generated frontmatter")
	}

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should allow raw source without frontmatter: %v", err)
	}
}

func TestVerifyIgnoresNonLumbreraRootFiles(t *testing.T) {
	repo := braintest.InitBrain(t)
	// Arbitrary files at root (e.g. .github/, rogue.md) should be ignored.
	if err := os.WriteFile(filepath.Join(repo, "rogue.md"), []byte("# Rogue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".github", "workflows", "ci.yml"), []byte("name: CI\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err != nil {
		t.Fatalf("verify should ignore non-Lumbrera root files: %v", err)
	}
}

func TestVerifyRejectsChangelogDrift(t *testing.T) {
	repo := braintest.InitBrain(t)
	changelog := braintest.ReadFile(t, repo, "CHANGELOG.md") + "2026-05-04 [source] [test]: Pending source\n"
	if err := os.WriteFile(filepath.Join(repo, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo}); err == nil {
		t.Fatal("expected verify to reject changelog drift")
	}
}

func TestVerifyRejectsTagsDrift(t *testing.T) {
	repo := braintest.InitBrain(t)
	tags := braintest.ReadFile(t, repo, "tags.md") + "\nManual tag edit.\n"
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

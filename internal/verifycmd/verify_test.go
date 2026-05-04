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
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

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

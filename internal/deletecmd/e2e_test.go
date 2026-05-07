package deletecmd

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/testfs"
)

func TestE2EDeleteSourceCascadeInTmp(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "lumbrera")
	runCmd(t, root, "", "go", "build", "-o", bin, "./cmd/lumbrera")

	repo := filepath.Join(tmp, "brain")
	runCmd(t, root, "", bin, "init", repo)

	// Add two sources.
	runCmd(t, repo, "# Source A\n\n## Evidence\n\nKey fact.\n", bin, "write", "sources/a.md", "--brain", repo, "--reason", "Add A", "--actor", "e2e")
	runCmd(t, repo, "# Source B\n\nMore facts.\n", bin, "write", "sources/b.md", "--brain", repo, "--reason", "Add B", "--actor", "e2e")

	// Wiki with only source A (will be cascade-deleted).
	runCmd(t, repo, "# Doomed\n\nClaim. [source: ../sources/a.md#evidence]\n", bin, "write", "wiki/doomed.md", "--brain", repo, "--title", "Doomed", "--summary", "Doomed summary.", "--tag", "doomed", "--source", "sources/a.md", "--reason", "Create doomed", "--actor", "e2e")

	// Wiki with both sources and a link to doomed (survives, gets cleaned).
	runCmd(t, repo, "# Survivor\n\nSee [Doomed](./doomed.md). Uses both. [source: ../sources/a.md#evidence]\n", bin, "write", "wiki/survivor.md", "--brain", repo, "--title", "Survivor", "--summary", "Survivor summary.", "--tag", "survivor", "--source", "sources/a.md", "--source", "sources/b.md", "--reason", "Create survivor", "--actor", "e2e")

	// Delete source A — should cascade-delete doomed and clean survivor.
	runCmd(t, repo, "", bin, "delete", "sources/a.md", "--brain", repo, "--reason", "Remove poison source", "--actor", "e2e")

	// Verify passes.
	runCmd(t, repo, "", bin, "verify", "--brain", repo)

	// Source A gone.
	assertFileMissing(t, repo, "sources/a.md")
	// Source B remains.
	assertFileExists(t, repo, "sources/b.md")
	// Doomed cascade-deleted.
	assertFileMissing(t, repo, "wiki/doomed.md")
	// Survivor remains.
	assertFileExists(t, repo, "wiki/survivor.md")

	survivor := testfs.ReadFile(t, repo, "wiki/survivor.md")
	if strings.Contains(survivor, "[source:") {
		t.Fatalf("survivor still has source citation:\n%s", survivor)
	}
	if strings.Contains(survivor, "[Doomed]") {
		t.Fatalf("survivor still links to doomed:\n%s", survivor)
	}
	if !strings.Contains(survivor, "See Doomed.") {
		t.Fatalf("expected unwrapped link in survivor:\n%s", survivor)
	}

	// INDEX should not mention deleted files.
	index := testfs.ReadFile(t, repo, "INDEX.md")
	if strings.Contains(index, "sources/a.md") {
		t.Fatalf("INDEX still references deleted source:\n%s", index)
	}
	if strings.Contains(index, "doomed") {
		t.Fatalf("INDEX still references cascade-deleted wiki:\n%s", index)
	}

	// CHANGELOG should record the cascade.
	changelog := testfs.ReadFile(t, repo, "CHANGELOG.md")
	if !strings.Contains(changelog, "Remove poison source") {
		t.Fatalf("CHANGELOG missing delete reason:\n%s", changelog)
	}
	if !strings.Contains(changelog, "cascade from sources/a.md") {
		t.Fatalf("CHANGELOG missing cascade entry:\n%s", changelog)
	}
}

func TestE2EWriteDeleteDeprecationDelegatesToDelete(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "lumbrera")
	runCmd(t, root, "", "go", "build", "-o", bin, "./cmd/lumbrera")

	repo := filepath.Join(tmp, "brain")
	runCmd(t, root, "", bin, "init", repo)
	runCmd(t, repo, "# Source\n\nFacts.\n", bin, "write", "sources/raw.md", "--brain", repo, "--reason", "Add source", "--actor", "e2e")
	runCmd(t, repo, "# Topic\n\nBody.\n", bin, "write", "wiki/topic.md", "--brain", repo, "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "e2e")

	// write --delete should still work (via CLI deprecation shim).
	cmd := exec.Command(bin, "write", "wiki/topic.md", "--brain", repo, "--delete", "--reason", "Remove topic", "--actor", "e2e")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("write --delete failed: %v\n%s", err, out)
	}
	// Should print deprecation warning.
	if !strings.Contains(string(out), "deprecated") {
		t.Fatalf("expected deprecation warning, got:\n%s", out)
	}

	assertFileMissing(t, repo, "wiki/topic.md")
	runCmd(t, repo, "", bin, "verify", "--brain", repo)
}

// --- helpers ---

func runCmd(t *testing.T, dir, stdin, name string, args ...string) {
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
}

func assertFileExists(t *testing.T, repo, rel string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	cmd := exec.Command("test", "-f", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected %s to exist", rel)
	}
}

func assertFileMissing(t *testing.T, repo, rel string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	cmd := exec.Command("test", "-f", path)
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected %s to be missing", rel)
	}
}

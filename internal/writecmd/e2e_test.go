package writecmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EInitSourceWriteWikiWriteInTmp(t *testing.T) {
	setGitIdentityEnv(t)
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
	remote := filepath.Join(tmp, "origin.git")
	runCommand(t, root, "", bin, "init", repo)
	runCommand(t, root, "", "git", "init", "--bare", remote)
	runCommand(t, repo, "", "git", "remote", "add", "origin", remote)
	runCommand(t, repo, "", "git", "push", "-u", "origin", "main")
	runCommand(t, repo, "# E2E source\n\nThis source describes Lumbrera write behavior.\n", bin, "write", "sources/2026/05/04/e2e-source.md", "--repo", repo, "--title", "E2E source", "--reason", "Preserve E2E source", "--actor", "e2e")
	runCommand(t, repo, "# E2E write page\n\nThe write command preserves sources and creates wiki pages.\n", bin, "write", "wiki/e2e-write-page.md", "--repo", repo, "--title", "E2E write page", "--source", "sources/2026/05/04/e2e-source.md", "--reason", "Distill E2E source", "--actor", "e2e", "--tag", "e2e")
	runCommand(t, repo, "", bin, "verify", "--repo", repo)

	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "3")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
	assertFileContains(t, repo, "wiki/e2e-write-page.md", "schema: document-v1")
	assertFileContains(t, repo, "wiki/e2e-write-page.md", "## Sources")
	assertFileContains(t, repo, "INDEX.md", "- 2026/\n  - 05/\n    - 04/\n      - [E2E source](sources/2026/05/04/e2e-source.md)")
	assertFileContains(t, repo, "INDEX.md", "[E2E write page](wiki/e2e-write-page.md)")
	assertFileContains(t, repo, "BRAIN.sum", "sources/2026/05/04/e2e-source.md sha256:")
	assertFileContains(t, repo, "BRAIN.sum", "wiki/e2e-write-page.md sha256:")
	assertFileContains(t, repo, "CHANGELOG.md", "[source] [e2e]: Preserve E2E source")
	assertFileContains(t, repo, "CHANGELOG.md", "[create] [e2e]: Distill E2E source")
}

func runCommand(t *testing.T, dir, stdin, name string, args ...string) {
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

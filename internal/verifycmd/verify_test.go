package verifycmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestVerifyPassesForInitializedBrain(t *testing.T) {
	repo := initBrain(t)

	if err := Run([]string{"--repo", repo}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestVerifyRejectsManifestDrift(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	path := filepath.Join(repo, "sources", "raw.md")
	content := strings.Replace(readFile(t, repo, "sources/raw.md"), "Raw notes.", "Changed raw notes.", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--repo", repo})
	if err == nil {
		t.Fatal("expected verify to reject BRAIN.sum drift")
	}
	if !strings.Contains(err.Error(), "BRAIN.sum") {
		t.Fatalf("expected BRAIN.sum drift error, got %v", err)
	}
}

func TestVerifyRejectsUnexpectedRootMarkdown(t *testing.T) {
	repo := initBrain(t)
	if err := os.WriteFile(filepath.Join(repo, "rogue.md"), []byte("# Rogue\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--repo", repo})
	if err == nil {
		t.Fatal("expected verify to reject root Markdown")
	}
	if !strings.Contains(err.Error(), "rogue.md") {
		t.Fatalf("expected error to mention rogue.md, got %v", err)
	}
}

func TestVerifySkipChangelogAllowsPendingChangelog(t *testing.T) {
	repo := initBrain(t)
	changelog := readFile(t, repo, "CHANGELOG.md") + "2026-05-04 [source] [test]: Pending source\n"
	if err := os.WriteFile(filepath.Join(repo, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--repo", repo}); err == nil {
		t.Fatal("expected normal verify to reject pending changelog drift")
	}
	if err := Run([]string{"--repo", repo, "--skip-changelog"}); err != nil {
		t.Fatalf("expected skip-changelog verify to pass, got %v", err)
	}
}

func initBrain(t *testing.T) string {
	t.Helper()
	setGitIdentityEnv(t)
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	configureRemote(t, repo)
	return repo
}

func configureRemote(t *testing.T, repo string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "origin.git")
	runCommand(t, filepath.Dir(remote), "git", "init", "--bare", remote)
	runCommand(t, repo, "git", "remote", "add", "origin", remote)
	runCommand(t, repo, "git", "push", "-u", "origin", "main")
}

func runCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func runWrite(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--repo", repo}, args...)
	if err := writecmd.Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func setGitIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func readFile(t *testing.T, repo, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
}

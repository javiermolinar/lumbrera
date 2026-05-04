package synccmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/repolock"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestSyncNoopOnCleanCurrentRepo(t *testing.T) {
	repo, remote := initBrainWithRemote(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	if err := Run([]string{"--repo", repo}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertBareGitOutput(t, remote, []string{"rev-list", "--count", "main"}, "2")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestSyncRepairsCommittedGeneratedDriftAndPushes(t *testing.T) {
	repo, remote := initBrainWithRemote(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	indexPath := filepath.Join(repo, "INDEX.md")
	if err := os.WriteFile(indexPath, []byte("# Broken generated index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, repo, "", "git", "add", "INDEX.md")
	runCommand(t, repo, "", "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "test: break generated index")
	runCommand(t, repo, "", "git", "-c", "core.hooksPath=/dev/null", "push")

	if err := Run([]string{"--repo", repo}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, syncCommitSubject)
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "4")
	assertBareGitOutput(t, remote, []string{"rev-list", "--count", "main"}, "4")
	assertFileContains(t, repo, "INDEX.md", "[Raw source](sources/raw.md)")
	assertFileContains(t, repo, "CHANGELOG.md", syncCommitSubject)
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestSyncDiscardsDirtyGeneratedFiles(t *testing.T) {
	repo, _ := initBrainWithRemote(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	if err := os.WriteFile(filepath.Join(repo, "INDEX.md"), []byte("# Manual generated edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--repo", repo}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertFileContains(t, repo, "INDEX.md", "[Raw source](sources/raw.md)")
	if strings.Contains(readFile(t, repo, "INDEX.md"), "Manual generated edit") {
		t.Fatal("sync left dirty generated edit in INDEX.md")
	}
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestSyncRejectsDirtyContentFiles(t *testing.T) {
	repo, _ := initBrainWithRemote(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	if err := os.WriteFile(filepath.Join(repo, "sources", "raw.md"), []byte("# Direct source edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--repo", repo})
	if err == nil {
		t.Fatal("expected sync to reject dirty content file")
	}
	if !strings.Contains(err.Error(), "non-generated changes") {
		t.Fatalf("expected non-generated changes error, got %v", err)
	}
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "M sources/raw.md")
}

func TestSyncRequiresConfiguredUpstream(t *testing.T) {
	setGitIdentityEnv(t)
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	err := Run([]string{"--repo", repo})
	if err == nil {
		t.Fatal("expected sync to require configured upstream")
	}
	if !strings.Contains(err.Error(), "configured upstream remote") {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func TestSyncPushesPreservedLocalWriteCommit(t *testing.T) {
	repo, remote := initBrainWithRemote(t)
	runCommand(t, repo, "", "git", "remote", "set-url", "--push", "origin", filepath.Join(t.TempDir(), "missing.git"))

	err := writecmd.Run([]string{"sources/raw.md", "--repo", repo, "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test"}, strings.NewReader("# Raw source\n\nRaw notes.\n"))
	if err == nil {
		t.Fatal("expected write push failure")
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertBareGitOutput(t, remote, []string{"rev-list", "--count", "main"}, "1")

	runCommand(t, repo, "", "git", "remote", "set-url", "--push", "origin", remote)
	if err := Run([]string{"--repo", repo}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertBareGitOutput(t, remote, []string{"rev-list", "--count", "main"}, "2")
	assertBareGitOutput(t, remote, []string{"log", "--format=%s", "-1", "main"}, "[source] [test]: Preserve raw source")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestSyncRejectsConcurrentRepoLock(t *testing.T) {
	repo, _ := initBrainWithRemote(t)
	lock, err := repolock.Acquire(repo, "test")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	err = Run([]string{"--repo", repo})
	if err == nil {
		t.Fatal("expected sync to reject concurrent repo lock")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error, got %v", err)
	}
}

func initBrainWithRemote(t *testing.T) (string, string) {
	t.Helper()
	setGitIdentityEnv(t)
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	remote := filepath.Join(t.TempDir(), "origin.git")
	runCommand(t, filepath.Dir(remote), "", "git", "init", "--bare", remote)
	runCommand(t, repo, "", "git", "remote", "add", "origin", remote)
	runCommand(t, repo, "", "git", "push", "-u", "origin", "main")
	return repo, remote
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

func assertGitOutput(t *testing.T, repo string, args []string, want string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != want {
		t.Fatalf("git %v got %q want %q", args, got, want)
	}
}

func assertBareGitOutput(t *testing.T, gitDir string, args []string, want string) {
	t.Helper()
	fullArgs := append([]string{"--git-dir", gitDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", fullArgs, err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != want {
		t.Fatalf("git %v got %q want %q", fullArgs, got, want)
	}
}

func assertFileContains(t *testing.T, repo, rel, want string) {
	t.Helper()
	got := readFile(t, repo, rel)
	if !strings.Contains(got, want) {
		t.Fatalf("expected %s to contain %q, got:\n%s", rel, want, got)
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

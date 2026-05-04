package initcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitMissingDirectory(t *testing.T) {
	setGitIdentityEnv(t)
	repo := filepath.Join(t.TempDir(), "brain")

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	assertFile(t, repo, ".brain/VERSION", brainVersion)
	assertExists(t, repo, "sources")
	assertExists(t, repo, "wiki")
	assertExists(t, repo, "INDEX.md")
	assertExists(t, repo, "CHANGELOG.md")
	assertExists(t, repo, "BRAIN.sum")
	assertExists(t, repo, "AGENTS.md")
	assertSymlink(t, repo, "CLAUDE.md", "AGENTS.md")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "name: lumbrera-ingest")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "pass --title")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "name: lumbrera-query")
	assertFileContains(t, repo, ".agents/skills/lumbrera-sync/SKILL.md", "name: lumbrera-sync")
	assertFileContains(t, repo, ".agents/skills/lumbrera-sync/SKILL.md", "lumbrera sync --repo <repo>")
	assertFileContains(t, repo, ".agents/skills/lumbrera-lint/SKILL.md", "name: lumbrera-lint")
	assertSymlink(t, repo, ".claude", ".agents")
	assertGitOutput(t, repo, []string{"config", "core.hooksPath"}, ".brain/hooks")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[init] [lumbrera]: Initialize Lumbrera brain")
	assertGitOutput(t, repo, []string{"log", "--format=%an <%ae>", "-1"}, "Test <test@example.invalid>")

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "1")
}

func TestInitAllowsCommittedGitHubBoilerplate(t *testing.T) {
	setGitIdentityEnv(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Brain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGitWithIdentity(t, repo, "commit", "-m", "Initial commit")

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init with boilerplate failed: %v", err)
	}

	assertFile(t, repo, "README.md", "# Brain")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[init] [lumbrera]: Initialize Lumbrera brain")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestInitInsideParentGitRepoCreatesRepoAtRequestedPath(t *testing.T) {
	setGitIdentityEnv(t)
	parent := t.TempDir()
	runGit(t, parent, "init", "-b", "main")
	runGitWithIdentity(t, parent, "commit", "--allow-empty", "-m", "Parent initial commit")

	repo := filepath.Join(parent, "brain")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init inside parent git repo failed: %v", err)
	}

	assertExists(t, repo, ".git")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[init] [lumbrera]: Initialize Lumbrera brain")
	assertGitOutput(t, parent, []string{"log", "--format=%s", "-1"}, "Parent initial commit")
	assertNoGitConfig(t, parent, "core.hooksPath")
}

func TestInitResumesPartialScaffoldAfterCommitFailure(t *testing.T) {
	setGitIdentityEnv(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "commit.gpgsign", "true")
	runGit(t, repo, "config", "gpg.program", "false")

	err := Run([]string{repo})
	if err == nil {
		t.Fatal("expected init to fail when git commit cannot sign")
	}
	assertFile(t, repo, ".brain/VERSION", brainVersion)
	assertNoHead(t, repo)

	runGit(t, repo, "config", "commit.gpgsign", "false")
	if err := Run([]string{repo}); err != nil {
		t.Fatalf("expected init to resume partial scaffold: %v", err)
	}

	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[init] [lumbrera]: Initialize Lumbrera brain")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestInitResumesPartialScaffoldOnExistingBoilerplateRepo(t *testing.T) {
	setGitIdentityEnv(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Brain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGitWithIdentity(t, repo, "commit", "-m", "Initial commit")
	runGit(t, repo, "config", "commit.gpgsign", "true")
	runGit(t, repo, "config", "gpg.program", "false")

	err := Run([]string{repo})
	if err == nil {
		t.Fatal("expected init to fail when git commit cannot sign")
	}
	assertFile(t, repo, ".brain/VERSION", brainVersion)
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "Initial commit")

	runGit(t, repo, "config", "commit.gpgsign", "false")
	if err := Run([]string{repo}); err != nil {
		t.Fatalf("expected init to resume partial scaffold on existing repo: %v", err)
	}

	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[init] [lumbrera]: Initialize Lumbrera brain")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestInitRejectsExistingContentRepo(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "notes.md"), []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{repo})
	if err == nil {
		t.Fatal("expected init to reject existing content repo")
	}
	if !strings.Contains(err.Error(), "notes.md") {
		t.Fatalf("expected error to mention notes.md, got %v", err)
	}
}

func TestInitRejectsInvalidBrainDirectory(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".brain"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{repo})
	if err == nil {
		t.Fatal("expected init to reject invalid .brain directory")
	}
	if !strings.Contains(err.Error(), ".brain") {
		t.Fatalf("expected error to mention .brain, got %v", err)
	}
}

func setGitIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func assertExists(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, rel)); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func assertFile(t *testing.T, repo, rel, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, rel))
	if err != nil {
		t.Fatalf("expected to read %s: %v", rel, err)
	}
	got := strings.TrimSpace(string(content))
	if got != want {
		t.Fatalf("unexpected %s content: got %q want %q", rel, got, want)
	}
}

func assertSymlink(t *testing.T, repo, rel, wantTarget string) {
	t.Helper()
	target, err := os.Readlink(filepath.Join(repo, rel))
	if err != nil {
		t.Fatalf("expected %s to be a symlink: %v", rel, err)
	}
	if target != wantTarget {
		t.Fatalf("unexpected %s symlink target: got %q want %q", rel, target, wantTarget)
	}
}

func assertFileContains(t *testing.T, repo, rel, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, rel))
	if err != nil {
		t.Fatalf("expected to read %s: %v", rel, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("expected %s to contain %q", rel, want)
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

func assertNoGitConfig(t *testing.T, repo, key string) {
	t.Helper()
	cmd := exec.Command("git", "config", "--local", "--get", key)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected git config %s to be unset, got %q", key, strings.TrimSpace(string(out)))
	}
}

func assertNoHead(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected repo to have no HEAD, got %q", strings.TrimSpace(string(out)))
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitWithIdentity(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

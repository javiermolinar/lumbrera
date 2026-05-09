package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitMissingDirectory(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "brain")

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	assertFile(t, repo, "VERSION", brainVersion)
	assertFileExact(t, repo, "INDEX.md", indexContent)
	assertFileExact(t, repo, "SOURCES.md", sourcesIndexContent)
	assertFileExact(t, repo, "ASSETS.md", assetsIndexContent)
	assertFileExact(t, repo, "CHANGELOG.md", changelogContent)
	assertFileExact(t, repo, "BRAIN.sum", brainSumContent)
	assertFileExact(t, repo, "tags.md", tagsContent)
	assertFileExact(t, repo, "AGENTS.md", agentsContent)
	assertFileExact(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", ingestSkillContent)
	assertFileExact(t, repo, ".agents/skills/lumbrera-query/SKILL.md", querySkillContent)
	assertFileExact(t, repo, ".agents/skills/lumbrera-health/SKILL.md", healthSkillContent)
	assertExists(t, repo, "sources")
	assertExists(t, repo, "sources/design")
	assertExists(t, repo, "sources/reference")
	assertExists(t, repo, "wiki")
	assertExists(t, repo, "wiki/design")
	assertExists(t, repo, "assets")
	assertExists(t, repo, "INDEX.md")
	assertExists(t, repo, "SOURCES.md")
	assertExists(t, repo, "ASSETS.md")
	assertExists(t, repo, "CHANGELOG.md")
	assertExists(t, repo, "BRAIN.sum")
	assertExists(t, repo, "tags.md")
	assertExists(t, repo, "CHANGELOG.md")
	assertFileContains(t, repo, ".gitignore", ".brain/")
	assertExists(t, repo, "AGENTS.md")
	assertFileContains(t, repo, "AGENTS.md", "## Read")
	assertFileContains(t, repo, "AGENTS.md", "## Write")
	assertFileContains(t, repo, "AGENTS.md", "Do not edit generated files")
	assertFileContains(t, repo, "AGENTS.md", "hard maximum of 400 Markdown body lines")
	assertFileContains(t, repo, "AGENTS.md", "lumbrera verify --brain .")
	assertFileContains(t, repo, "AGENTS.md", "Team Git/GitHub errors")
	assertFileContains(t, repo, "AGENTS.md", "Do not commit conflict markers")
	assertSymlink(t, repo, "CLAUDE.md", "AGENTS.md")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "name: lumbrera-ingest")
	assertFileContains(t, repo, "AGENTS.md", "symptom → cause → fix")
	assertFileContains(t, repo, "AGENTS.md", "Inline source citations")

	assertFileContains(t, repo, "AGENTS.md", "inline citations complement it")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "pass --title")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "1-5 --tag")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "overlap before writing")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "Search for overlap before writing")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "--source sources/<source>.md")
	assertFileContains(t, repo, ".agents/skills/lumbrera-ingest/SKILL.md", "update existing page, create new page")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "name: lumbrera-query")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "lumbrera search \"<question>\" --json")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "recommended_read_order")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "recommended_sections")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "primary product contract")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "Check coverage")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "--tag <tag>")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "--source sources/<source>.md")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "Do not start by scanning the repo")
	assertFileContains(t, repo, ".agents/skills/lumbrera-query/SKILL.md", "Do not infer frequency")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "name: lumbrera-health")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "## Goal")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "lumbrera health --json")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "candidates, not conclusions")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "exact filters")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "Missing-link triage")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "Duplicate/consolidation helper")
	assertFileContains(t, repo, ".agents/skills/lumbrera-health/SKILL.md", "high-risk claims")
	assertSymlink(t, repo, ".claude", ".agents")
	assertMissing(t, repo, ".git")
	assertMissing(t, repo, ".brain/hooks")

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
}

func TestInitAllowsBoilerplate(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Brain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init with boilerplate failed: %v", err)
	}

	assertFile(t, repo, "README.md", "# Brain")
	assertFileContains(t, repo, ".gitignore", "dist/")
	assertFileContains(t, repo, ".gitignore", ".brain/")
	assertExists(t, repo, "VERSION")
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

func TestInitIgnoresBrainCacheDirectory(t *testing.T) {
	repo := t.TempDir()
	// A leftover .brain/ cache directory should not prevent init.
	if err := os.Mkdir(filepath.Join(repo, ".brain"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{repo}); err != nil {
		t.Fatalf("init should succeed with leftover .brain/ cache: %v", err)
	}
	assertExists(t, repo, "VERSION")
}

func assertExists(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func assertMissing(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", rel, err)
	}
}

func assertFile(t *testing.T, repo, rel, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("expected to read %s: %v", rel, err)
	}
	got := strings.TrimSpace(string(content))
	if got != want {
		t.Fatalf("unexpected %s content: got %q want %q", rel, got, want)
	}
}

func assertFileExact(t *testing.T, repo, rel, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("expected to read %s: %v", rel, err)
	}
	if string(content) != want {
		t.Fatalf("unexpected %s content:\ngot:\n%s\nwant:\n%s", rel, string(content), want)
	}
}

func assertSymlink(t *testing.T, repo, rel, wantTarget string) {
	t.Helper()
	target, err := os.Readlink(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("expected %s to be a symlink: %v", rel, err)
	}
	if target != wantTarget {
		t.Fatalf("unexpected %s symlink target: got %q want %q", rel, target, wantTarget)
	}
}

func assertFileContains(t *testing.T, repo, rel, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("expected to read %s: %v", rel, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("expected %s to contain %q", rel, want)
	}
}

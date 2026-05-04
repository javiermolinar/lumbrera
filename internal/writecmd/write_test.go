package writecmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
)

func TestWriteSourceAndWikiCreateCommitGeneratedFiles(t *testing.T) {
	repo := initBrain(t)

	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/2026/05/04/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Related\n\nCompanion page.\n", "wiki/related.md", "--title", "Related", "--source", "sources/2026/05/04/raw.md", "--reason", "Create related", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nSee [Related](./related.md).\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/2026/05/04/raw.md", "--reason", "Create topic", "--actor", "test", "--tag", "design")

	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "4")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[create] [test]: Create topic")

	wiki := readFile(t, repo, "wiki/topic.md")
	meta, body, has, err := frontmatter.Split([]byte(wiki))
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected generated frontmatter")
	}
	if meta.Title != "Topic" || meta.Lumbrera.Kind != "wiki" {
		t.Fatalf("unexpected wiki metadata: %+v", meta)
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "design" {
		t.Fatalf("unexpected tags: %#v", meta.Tags)
	}
	if len(meta.Lumbrera.Sources) != 1 || meta.Lumbrera.Sources[0] != "sources/2026/05/04/raw.md" {
		t.Fatalf("unexpected sources: %#v", meta.Lumbrera.Sources)
	}
	if len(meta.Lumbrera.Links) != 1 || meta.Lumbrera.Links[0] != "wiki/related.md" {
		t.Fatalf("unexpected links: %#v", meta.Lumbrera.Links)
	}
	if !strings.Contains(body, "## Sources\n\n- [Raw](../sources/2026/05/04/raw.md)") {
		t.Fatalf("expected generated Sources section, got:\n%s", body)
	}

	assertFileContains(t, repo, "INDEX.md", "[Raw source](sources/2026/05/04/raw.md)")
	assertFileContains(t, repo, "INDEX.md", "[Topic](wiki/topic.md)")
	assertFileContains(t, repo, "BRAIN.sum", "sources/2026/05/04/raw.md sha256:")
	assertFileContains(t, repo, "BRAIN.sum", "wiki/topic.md sha256:")
	assertFileContains(t, repo, "CHANGELOG.md", "[source] [test]: Preserve raw source")
	assertFileContains(t, repo, "CHANGELOG.md", "[create] [test]: Create topic")
}

func TestWriteAppendUpdateAndDeleteWiki(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\n## Notes\n\nInitial.\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	runWrite(t, repo, "Appended note.\n", "wiki/topic.md", "--append", "Notes", "--source", "sources/raw.md", "--reason", "Append note", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Initial.\n\nAppended note.")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[append] [test]: Append note")

	runWrite(t, repo, "# Topic\n\nReplacement.\n", "wiki/topic.md", "--source", "sources/raw.md", "--reason", "Replace topic", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Replacement.")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[update] [test]: Replace topic")

	runWrite(t, repo, "", "wiki/topic.md", "--delete", "--reason", "Remove topic", "--actor", "test")
	assertMissing(t, repo, "wiki/topic.md")
	assertGitOutput(t, repo, []string{"log", "--format=%s", "-1"}, "[delete] [test]: Remove topic")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRejectsEmptyAppendFlag(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\n## Notes\n\nInitial.\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	assertWriteError(t, repo, "Should append.\n", "wiki/topic.md", "--append=", "--source", "sources/raw.md", "--reason", "Append with empty section", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Initial.")
	if strings.Contains(readFile(t, repo, "wiki/topic.md"), "Should append") {
		t.Fatal("failed empty append wrote snippet into page")
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "3")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRejectsAppendToGeneratedSourcesSection(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	assertWriteError(t, repo, "This would be discarded.\n", "wiki/topic.md", "--append", "Sources", "--source", "sources/raw.md", "--reason", "Append to generated sources", "--actor", "test")
	if strings.Contains(readFile(t, repo, "wiki/topic.md"), "This would be discarded") {
		t.Fatal("append to generated Sources section wrote snippet into page")
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "3")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRejectsDeleteDirectoryTarget(t *testing.T) {
	repo := initBrain(t)
	dir := filepath.Join(repo, "wiki", "ghost.md")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	assertWriteError(t, repo, "", "wiki/ghost.md", "--delete", "--reason", "Delete ghost directory", "--actor", "test")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected directory target to remain, info=%v err=%v", info, err)
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "1")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRejectsMissingInternalLinksAndRollsBack(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	assertWriteError(t, repo, "# Missing link\n\nSee [Missing](./missing.md).\n", "wiki/missing-link.md", "--title", "Missing link", "--source", "sources/raw.md", "--reason", "Create missing link", "--actor", "test")
	assertMissing(t, repo, "wiki/missing-link.md")
	if strings.Contains(readFile(t, repo, "CHANGELOG.md"), "Create missing link") {
		t.Fatal("failed write left pending changelog entry")
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRejectsMissingAnchorsAndRollsBack(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\n## Evidence\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	cases := []struct {
		target string
		title  string
		body   string
		reason string
	}{
		{"wiki/bad-anchor.md", "Bad anchor", "# Bad anchor\n\nSee [Evidence](../sources/raw.md#missing).\n", "Create bad anchor"},
		{"wiki/bad-punctuation-anchor.md", "Bad punctuation anchor", "# Bad punctuation anchor\n\nSee [Evidence](../sources/raw.md#evidence.).\n", "Create bad punctuation anchor"},
		{"wiki/bad-query-anchor.md", "Bad query anchor", "# Bad query anchor\n\nSee [Evidence](../sources/raw.md#evidence?bad).\n", "Create bad query anchor"},
		{"wiki/bad-citation-anchor.md", "Bad citation anchor", "# Bad citation anchor\n\nClaim. [source: ../sources/raw.md#missing]\n", "Create bad citation anchor"},
	}
	for _, tc := range cases {
		assertWriteError(t, repo, tc.body, tc.target, "--title", tc.title, "--source", "sources/raw.md", "--reason", tc.reason, "--actor", "test")
		assertMissing(t, repo, tc.target)
		if strings.Contains(readFile(t, repo, "CHANGELOG.md"), tc.reason) {
			t.Fatalf("failed write left pending changelog entry for %q", tc.reason)
		}
	}
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteExtractsSourceCitationsIntoGeneratedSources(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw A\n\n## Context\n\nA.\n", "sources/raw-a.md", "--title", "Raw A", "--reason", "Preserve raw A", "--actor", "test")
	runWrite(t, repo, "# Raw B\n\n## Claim Detail\n\nB.\n", "sources/raw-b.md", "--title", "Raw B", "--reason", "Preserve raw B", "--actor", "test")

	runWrite(t, repo, "# Topic\n\nImportant claim. [source: ../sources/raw-b.md#claim-detail]\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw-a.md", "--reason", "Create cited topic", "--actor", "test")
	wiki := readFile(t, repo, "wiki/topic.md")
	meta, body, has, err := frontmatter.Split([]byte(wiki))
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected generated frontmatter")
	}
	if len(meta.Lumbrera.Sources) != 2 || meta.Lumbrera.Sources[0] != "sources/raw-a.md" || meta.Lumbrera.Sources[1] != "sources/raw-b.md" {
		t.Fatalf("unexpected sources from claim citation: %#v", meta.Lumbrera.Sources)
	}
	if !strings.Contains(body, "[source: ../sources/raw-b.md#claim-detail]") {
		t.Fatalf("expected inline source citation to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "- [Raw A](../sources/raw-a.md)") || !strings.Contains(body, "- [Raw B](../sources/raw-b.md)") {
		t.Fatalf("expected generated Sources section to include cited source, got:\n%s", body)
	}
}

func TestWriteIgnoresInlineCodeAndExternalSourceText(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw text can mention external markers. [source: https://example.com/report]\n\nRaw text can also mention local-looking markers without turning into citations. [source: ../sources/missing.md#missing]\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	runWrite(t, repo, "# Topic\n\nLiteral citation syntax: `[source: ../sources/raw.md#missing]`.\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
	wiki := readFile(t, repo, "wiki/topic.md")
	meta, body, has, err := frontmatter.Split([]byte(wiki))
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected generated frontmatter")
	}
	if len(meta.Lumbrera.Sources) != 1 || meta.Lumbrera.Sources[0] != "sources/raw.md" {
		t.Fatalf("unexpected sources from literal citation text: %#v", meta.Lumbrera.Sources)
	}
	if !strings.Contains(body, "`[source: ../sources/raw.md#missing]`") {
		t.Fatalf("expected inline code citation text to be preserved, got:\n%s", body)
	}
}

func TestWritePreflightRejectsExistingBrokenAnchor(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\n## Evidence\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nSee [Evidence](../sources/raw.md#evidence).\n", "wiki/topic.md", "--title", "Topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	rawPath := filepath.Join(repo, "sources", "raw.md")
	raw := strings.Replace(readFile(t, repo, "sources/raw.md"), "## Evidence", "## Renamed", 1)
	if err := os.WriteFile(rawPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, repo, "", "git", "add", "sources/raw.md")
	runCommand(t, repo, "", "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "test: break source anchor")

	assertWriteError(t, repo, "# New source\n\nNotes.\n", "sources/new.md", "--title", "New source", "--reason", "Preserve new source", "--actor", "test")
	assertMissing(t, repo, "sources/new.md")
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "4")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestWriteRollsBackWhenCommitFails(t *testing.T) {
	repo := initBrain(t)
	hook := filepath.Join(repo, ".brain", "hooks", "commit-msg")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runCommand(t, repo, "", "git", "add", ".brain/hooks/commit-msg")
	runCommand(t, repo, "", "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "test: break commit hook")

	assertWriteError(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	assertMissing(t, repo, "sources/raw.md")
	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func TestValidateDocumentRejectsStaleGeneratedFrontmatter(t *testing.T) {
	repo := initBrain(t)
	path := filepath.Join(repo, "wiki", "stale.md")
	meta := frontmatter.New("wiki", "Stale", "", nil, []string{"sources/other.md"}, []string{"wiki/wrong.md"})
	content, err := frontmatter.Attach(meta, "# Stale\n\nSee [Right](./right.md).\n\n## Sources\n\n- [Raw](../sources/raw.md)\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateDocuments(repo); err == nil {
		t.Fatal("expected stale frontmatter to be rejected")
	}
}

func TestWriteRejectsInvalidMutations(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	assertWriteError(t, repo, "# Raw source\n\nChanged.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Change raw source", "--actor", "test")
	assertWriteError(t, repo, "---\ntitle: Manual\n---\n\n# Manual\n", "wiki/manual.md", "--title", "Manual", "--source", "sources/raw.md", "--reason", "Create manual", "--actor", "test")
	assertWriteError(t, repo, "# Manual sources\n\n## Sources\n\n- [Raw](../sources/raw.md)\n", "wiki/manual-sources.md", "--title", "Manual sources", "--source", "sources/raw.md", "--reason", "Create manual sources", "--actor", "test")
	assertWriteError(t, repo, "# Missing source\n", "wiki/missing.md", "--title", "Missing source", "--source", "sources/missing.md", "--reason", "Create missing", "--actor", "test")
	assertWriteError(t, repo, "# Untitled\n", "wiki/untitled.md", "--source", "sources/raw.md", "--reason", "Create untitled", "--actor", "test")
	assertWriteError(t, repo, "# Bad\n", "../bad.md", "--title", "Bad", "--reason", "Bad path", "--actor", "test")
	assertWriteError(t, repo, "# Bad\n", "sources/../wiki/bad.md", "--title", "Bad", "--source", "sources/raw.md", "--reason", "Bad clean path", "--actor", "test")

	assertGitOutput(t, repo, []string{"rev-list", "--count", "HEAD"}, "2")
	assertGitOutput(t, repo, []string{"status", "--porcelain"}, "")
}

func initBrain(t *testing.T) string {
	t.Helper()
	setGitIdentityEnv(t)
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return repo
}

func runWrite(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--repo", repo}, args...)
	if err := Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func assertWriteError(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--repo", repo}, args...)
	if err := Run(fullArgs, strings.NewReader(stdin)); err == nil {
		t.Fatalf("write %v unexpectedly succeeded", fullArgs)
	}
}

func setGitIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
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

func readFile(t *testing.T, repo, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
}

func assertFileContains(t *testing.T, repo, rel, want string) {
	t.Helper()
	got := readFile(t, repo, rel)
	if !strings.Contains(got, want) {
		t.Fatalf("expected %s to contain %q, got:\n%s", rel, want, got)
	}
}

func assertMissing(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", rel, err)
	}
}

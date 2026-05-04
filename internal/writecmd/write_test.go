package writecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
)

func TestWriteSourceAndWikiCreateGeneratedFiles(t *testing.T) {
	repo := initBrain(t)

	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/2026/05/04/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	if frontmatter.StartsWithFrontmatter([]byte(readFile(t, repo, "sources/2026/05/04/raw.md"))) {
		t.Fatal("source writes should preserve raw source content without generated frontmatter")
	}
	runWrite(t, repo, "# Related\n\nCompanion page.\n", "wiki/related.md", "--title", "Related", "--summary", "Related companion page.", "--tag", "related", "--source", "sources/2026/05/04/raw.md", "--reason", "Create related", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nSee [Related](./related.md).\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--source", "sources/2026/05/04/raw.md", "--reason", "Create topic", "--actor", "test", "--tag", "design")

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
	if meta.Lumbrera.ID == "" {
		t.Fatal("expected generated document id")
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

	assertFileContains(t, repo, "INDEX.md", "[Raw](sources/2026/05/04/raw.md)")
	assertFileContains(t, repo, "INDEX.md", "[Topic](wiki/topic.md)")
	assertFileNotContains(t, repo, "BRAIN.sum", "sources/2026/05/04/raw.md sha256:")
	assertFileContains(t, repo, "BRAIN.sum", "wiki/topic.md sha256:")
	assertFileContains(t, repo, "CHANGELOG.md", "[source] [test]: Preserve raw source")
	assertFileContains(t, repo, "CHANGELOG.md", "[create] [test]: Create topic")
	assertFileContains(t, repo, "tags.md", "## design")
	assertFileContains(t, repo, "tags.md", "- [Topic](wiki/topic.md) — Topic summary.")
	assertFileContains(t, repo, ".brain/ops.log", `"operation":"create"`)
}

func TestWriteAppendUpdateAndDeleteWiki(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\n## Notes\n\nInitial.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	runWrite(t, repo, "Appended note.\n", "wiki/topic.md", "--append", "Notes", "--source", "sources/raw.md", "--reason", "Append note", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Initial.\n\nAppended note.")
	assertFileContains(t, repo, "CHANGELOG.md", "[append] [test]: Append note")

	beforeUpdateMeta, _, _, err := frontmatter.Split([]byte(readFile(t, repo, "wiki/topic.md")))
	if err != nil {
		t.Fatal(err)
	}
	runWrite(t, repo, "# Topic\n\nReplacement.\n", "wiki/topic.md", "--source", "sources/raw.md", "--reason", "Replace topic", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Replacement.")
	afterUpdateMeta, _, _, err := frontmatter.Split([]byte(readFile(t, repo, "wiki/topic.md")))
	if err != nil {
		t.Fatal(err)
	}
	if beforeUpdateMeta.Lumbrera.ID == "" || afterUpdateMeta.Lumbrera.ID != beforeUpdateMeta.Lumbrera.ID {
		t.Fatalf("expected update to preserve document id: before=%q after=%q", beforeUpdateMeta.Lumbrera.ID, afterUpdateMeta.Lumbrera.ID)
	}
	assertFileContains(t, repo, "CHANGELOG.md", "[update] [test]: Replace topic")

	runWrite(t, repo, "", "wiki/topic.md", "--delete", "--reason", "Remove topic", "--actor", "test")
	assertMissing(t, repo, "wiki/topic.md")
	assertFileContains(t, repo, "CHANGELOG.md", "[delete] [test]: Remove topic")
}

func TestWriteRejectsEmptyAppendFlag(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\n## Notes\n\nInitial.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	assertWriteError(t, repo, "Should append.\n", "wiki/topic.md", "--append=", "--source", "sources/raw.md", "--reason", "Append with empty section", "--actor", "test")
	assertFileContains(t, repo, "wiki/topic.md", "Initial.")
	if strings.Contains(readFile(t, repo, "wiki/topic.md"), "Should append") {
		t.Fatal("failed empty append wrote snippet into page")
	}
}

func TestWriteRejectsAppendToGeneratedSourcesSection(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	assertWriteError(t, repo, "This would be discarded.\n", "wiki/topic.md", "--append", "Sources", "--source", "sources/raw.md", "--reason", "Append to generated sources", "--actor", "test")
	if strings.Contains(readFile(t, repo, "wiki/topic.md"), "This would be discarded") {
		t.Fatal("append to generated Sources section wrote snippet into page")
	}
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
}

func TestWriteRejectsMissingInternalLinksAndRollsBack(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test")

	assertWriteError(t, repo, "# Missing link\n\nSee [Missing](./missing.md).\n", "wiki/missing-link.md", "--title", "Missing link", "--summary", "Missing link summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create missing link", "--actor", "test")
	assertMissing(t, repo, "wiki/missing-link.md")
	if strings.Contains(readFile(t, repo, "CHANGELOG.md"), "Create missing link") {
		t.Fatal("failed write left pending changelog entry")
	}
	if strings.Contains(readFile(t, repo, ".brain/ops.log"), "Create missing link") {
		t.Fatal("failed write left operation log entry")
	}
}

func TestWriteAllowsRawSourceLinksWhenCheckingAnchors(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nSee [external relative doc](../outside.md).\n\n## Evidence\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")

	runWrite(t, repo, "# Topic\n\nSee [Evidence](../sources/raw.md#evidence).\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
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
		assertWriteError(t, repo, tc.body, tc.target, "--title", tc.title, "--summary", tc.title+" summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", tc.reason, "--actor", "test")
		assertMissing(t, repo, tc.target)
		if strings.Contains(readFile(t, repo, "CHANGELOG.md"), tc.reason) {
			t.Fatalf("failed write left changelog entry for %q", tc.reason)
		}
	}
}

func TestWriteExtractsSourceCitationsIntoGeneratedSources(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw A\n\n## Context\n\nA.\n", "sources/raw-a.md", "--title", "Raw A", "--reason", "Preserve raw A", "--actor", "test")
	runWrite(t, repo, "# Raw B\n\n## Claim Detail\n\nB.\n", "sources/raw-b.md", "--title", "Raw B", "--reason", "Preserve raw B", "--actor", "test")

	runWrite(t, repo, "# Topic\n\nImportant claim. [source: ../sources/raw-b.md#claim-detail]\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw-a.md", "--reason", "Create cited topic", "--actor", "test")
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

	runWrite(t, repo, "# Topic\n\nLiteral citation syntax: `[source: ../sources/raw.md#missing]`.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
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
	runWrite(t, repo, "# Topic\n\nSee [Evidence](../sources/raw.md#evidence).\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	rawPath := filepath.Join(repo, "sources", "raw.md")
	raw := strings.Replace(readFile(t, repo, "sources/raw.md"), "## Evidence", "## Renamed", 1)
	if err := os.WriteFile(rawPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	regenerateChecksumsOnly(t, repo)

	assertWriteError(t, repo, "# New source\n\nNotes.\n", "sources/new.md", "--title", "New source", "--reason", "Preserve new source", "--actor", "test")
	assertMissing(t, repo, "sources/new.md")
}

func TestValidateDocumentRejectsStaleGeneratedFrontmatter(t *testing.T) {
	repo := initBrain(t)
	path := filepath.Join(repo, "wiki", "stale.md")
	meta := frontmatter.New("wiki", "Stale", "Stale summary.", []string{"stale"}, []string{"sources/other.md"}, []string{"wiki/wrong.md"})
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
	assertWriteError(t, repo, "---\ntitle: Manual\n---\n\n# Manual\n", "wiki/manual.md", "--title", "Manual", "--summary", "Manual summary.", "--tag", "manual", "--source", "sources/raw.md", "--reason", "Create manual", "--actor", "test")
	assertWriteError(t, repo, "# Manual sources\n\n## Sources\n\n- [Raw](../sources/raw.md)\n", "wiki/manual-sources.md", "--title", "Manual sources", "--summary", "Manual sources summary.", "--tag", "manual", "--source", "sources/raw.md", "--reason", "Create manual sources", "--actor", "test")
	assertWriteError(t, repo, "# Missing source\n", "wiki/missing.md", "--title", "Missing source", "--summary", "Missing source summary.", "--tag", "missing", "--source", "sources/missing.md", "--reason", "Create missing", "--actor", "test")
	assertWriteError(t, repo, "# Untitled\n", "wiki/untitled.md", "--source", "sources/raw.md", "--reason", "Create untitled", "--actor", "test")
	assertWriteError(t, repo, "# No summary\n", "wiki/no-summary.md", "--title", "No summary", "--tag", "missing", "--source", "sources/raw.md", "--reason", "Create no summary", "--actor", "test")
	assertWriteError(t, repo, "# No tags\n", "wiki/no-tags.md", "--title", "No tags", "--summary", "No tags summary.", "--source", "sources/raw.md", "--reason", "Create no tags", "--actor", "test")
	assertWriteError(t, repo, "# Too many tags\n", "wiki/too-many-tags.md", "--title", "Too many tags", "--summary", "Too many tags summary.", "--tag", "one", "--tag", "two", "--tag", "three", "--tag", "four", "--tag", "five", "--tag", "six", "--source", "sources/raw.md", "--reason", "Create too many tags", "--actor", "test")
	assertWriteError(t, repo, "# Bad\n", "../bad.md", "--title", "Bad", "--reason", "Bad path", "--actor", "test")
	assertWriteError(t, repo, "# Bad\n", "sources/../wiki/bad.md", "--title", "Bad", "--source", "sources/raw.md", "--reason", "Bad clean path", "--actor", "test")
}

func TestWriteRejectsConcurrentBrainLock(t *testing.T) {
	repo := initBrain(t)
	lock, err := brainlock.Acquire(repo, "test")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	err = Run([]string{"sources/raw.md", "--brain", repo, "--title", "Raw source", "--reason", "Preserve raw source", "--actor", "test"}, strings.NewReader("# Raw source\n"))
	if err == nil {
		t.Fatal("expected write to reject concurrent brain lock")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error, got %v", err)
	}
	assertMissing(t, repo, "sources/raw.md")
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
	if err := Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func assertWriteError(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--brain", repo}, args...)
	if err := Run(fullArgs, strings.NewReader(stdin)); err == nil {
		t.Fatalf("write %v unexpectedly succeeded", fullArgs)
	}
}

func regenerateChecksumsOnly(t *testing.T, repo string) {
	t.Helper()
	content := readFile(t, repo, "BRAIN.sum")
	if !strings.Contains(content, "sha256:") {
		t.Fatal("expected BRAIN.sum to have checksums")
	}
	// Let the next write preflight get past checksum drift so it can catch the
	// broken anchor. This mimics a capable direct editor updating generated sums.
	if err := os.WriteFile(filepath.Join(repo, "BRAIN.sum"), []byte(strings.Replace(content, "sha256:", "sha256:", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := generate.FilesForRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		t.Fatal(err)
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

func assertFileNotContains(t *testing.T, repo, rel, unwanted string) {
	t.Helper()
	got := readFile(t, repo, rel)
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected %s not to contain %q, got:\n%s", rel, unwanted, got)
	}
}

func assertMissing(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", rel, err)
	}
}

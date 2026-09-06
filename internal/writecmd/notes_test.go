package writecmd

import (
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func TestWriteNoteLifecycleAndWikiBackedOnlyByNote(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Observation\n\nInitial detail.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "A first-party observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	notePolicy, _ := brain.PolicyForKind(brain.KindNote)
	before, _, _, err := frontmatter.SplitForPolicy([]byte(testfs.ReadFile(t, repo, "notes/observation.md")), notePolicy)
	if err != nil {
		t.Fatal(err)
	}

	runWrite(t, repo, "# Observation\n\nReplacement detail.\n", "notes/observation.md",
		"--reason", "Refine observation", "--actor", "test")
	runWrite(t, repo, "Appended detail.\n", "notes/observation.md",
		"--append", "Details", "--reason", "Extend observation", "--actor", "test")

	after, body, _, err := frontmatter.SplitForPolicy([]byte(testfs.ReadFile(t, repo, "notes/observation.md")), notePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if after.Lumbrera.ID != before.Lumbrera.ID {
		t.Fatalf("note ID changed: before=%q after=%q", before.Lumbrera.ID, after.Lumbrera.ID)
	}
	if before.Lumbrera.ModifiedDate == "" || after.Lumbrera.ModifiedDate == "" {
		t.Fatalf("note lifecycle did not retain a generated modified date: before=%q after=%q", before.Lumbrera.ModifiedDate, after.Lumbrera.ModifiedDate)
	}
	if !strings.Contains(body, "## Details\n\nAppended detail.") || strings.Contains(body, "## Sources") {
		t.Fatalf("unexpected note body:\n%s", body)
	}

	runWrite(t, repo, "# Guidance\n\nUse the [recorded observation](../notes/observation.md).\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Guidance backed by first-party evidence.", "--tag", "operations",
		"--source", "notes/observation.md", "--reason", "Synthesize observation", "--actor", "test")

	wiki, wikiBody, _, err := frontmatter.Split([]byte(testfs.ReadFile(t, repo, "wiki/guidance.md")))
	if err != nil {
		t.Fatal(err)
	}
	if len(wiki.Lumbrera.Sources) != 1 || wiki.Lumbrera.Sources[0] != "notes/observation.md" {
		t.Fatalf("wiki evidence = %#v, want note", wiki.Lumbrera.Sources)
	}
	if len(wiki.Lumbrera.Links) != 1 || wiki.Lumbrera.Links[0] != "notes/observation.md" {
		t.Fatalf("wiki managed links = %#v, want note", wiki.Lumbrera.Links)
	}
	if !strings.Contains(wikiBody, "- [Observation](../notes/observation.md)") {
		t.Fatalf("wiki Sources section does not contain note:\n%s", wikiBody)
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("verify note-backed wiki: %v", err)
	}
	assertFileContains(t, repo, "NOTES.md", "[Observation](notes/observation.md) — A first-party observation.")
	assertFileContains(t, repo, "BRAIN.sum", "notes/observation.md sha256:")
	assertFileContains(t, repo, "tags.md", "- operations (2)")
}

func TestWriteWikiAcceptsInlineNoteCitationAsSoleEvidence(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Observation\n\n## Result\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "A first-party observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	runWrite(t, repo, "# Guidance\n\nClaim. [source: ../notes/observation.md#result]\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Guidance backed by an inline note citation.", "--tag", "operations",
		"--reason", "Synthesize cited observation", "--actor", "test")

	meta, body, _, err := frontmatter.Split([]byte(testfs.ReadFile(t, repo, "wiki/guidance.md")))
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Lumbrera.Sources) != 1 || meta.Lumbrera.Sources[0] != "notes/observation.md" {
		t.Fatalf("inline note evidence = %#v", meta.Lumbrera.Sources)
	}
	if !strings.Contains(body, "[source: ../notes/observation.md#result]") || !strings.Contains(body, "- [Observation](../notes/observation.md)") {
		t.Fatalf("inline note citation or generated Sources entry missing:\n%s", body)
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("verify inline note evidence: %v", err)
	}
}

func TestWriteNoteRejectsEvidenceAndSourcesSection(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw\n\nEvidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	runWrite(t, repo, "# Existing wiki\n\nKnowledge.\n", "wiki/existing.md",
		"--title", "Existing wiki", "--summary", "Existing synthesized knowledge.", "--tag", "existing", "--source", "sources/raw.md",
		"--reason", "Create existing wiki", "--actor", "test")

	assertWriteError(t, repo, "# Invalid wiki evidence\n", "wiki/wiki-evidence.md",
		"--title", "Invalid wiki evidence", "--summary", "A wiki cannot back another wiki.", "--tag", "invalid", "--source", "wiki/existing.md",
		"--reason", "Try wiki evidence", "--actor", "test")
	assertWriteError(t, repo, "# Missing note evidence\n", "wiki/missing-note.md",
		"--title", "Missing note evidence", "--summary", "Missing note evidence must fail.", "--tag", "invalid", "--source", "notes/missing.md",
		"--reason", "Try missing note evidence", "--actor", "test")
	assertWriteError(t, repo, "# Unsafe note evidence\n", "wiki/unsafe-note.md",
		"--title", "Unsafe note evidence", "--summary", "Unsafe note evidence must fail.", "--tag", "invalid", "--source", "notes/../notes/missing.md",
		"--reason", "Try unsafe note evidence", "--actor", "test")

	assertWriteError(t, repo, "# Invalid\n\nBody.\n", "notes/with-source.md",
		"--title", "Invalid", "--summary", "Invalid note.", "--tag", "invalid", "--source", "sources/raw.md",
		"--reason", "Invalid note", "--actor", "test")
	assertWriteError(t, repo, "# Invalid\n\nClaim. [source: ../sources/raw.md]\n", "notes/with-citation.md",
		"--title", "Invalid", "--summary", "Invalid note.", "--tag", "invalid",
		"--reason", "Invalid citation", "--actor", "test")
	assertWriteError(t, repo, "# Invalid external citation\n\nClaim. [source: https://example.com/report]\n", "notes/with-external-citation.md",
		"--title", "Invalid external citation", "--summary", "External citation syntax is invalid in a note.", "--tag", "invalid",
		"--reason", "Invalid external citation", "--actor", "test")
	runWrite(t, repo, "# Literal syntax\n\nLiteral syntax in code: `[source: ../sources/raw.md]`.\n", "notes/literal-syntax.md",
		"--title", "Literal syntax", "--summary", "Citation syntax shown as literal code.", "--tag", "examples",
		"--reason", "Record literal syntax", "--actor", "test")
	assertWriteError(t, repo, "# Invalid\n\n## Sources\n\n- [Raw](../sources/raw.md)\n", "notes/with-section.md",
		"--title", "Invalid", "--summary", "Invalid note.", "--tag", "invalid",
		"--reason", "Invalid section", "--actor", "test")
}

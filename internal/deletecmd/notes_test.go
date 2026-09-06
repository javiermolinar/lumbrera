package deletecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/testfs"
)

func TestDeleteUnreferencedNote(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Observation\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observed behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	runDelete(t, repo, "notes/observation.md", "--reason", "Remove observation", "--actor", "test")
	assertMissing(t, repo, "notes/observation.md")
	assertVerify(t, repo)
}

func TestDeleteNoteRemovesEvidenceAndPreservesWikiWithSource(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nRaw evidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\n## Detail\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observed behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	braintest.RunWrite(t, repo, "# Guidance\n\nClaim. [source: ../notes/observation.md#detail]\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Guidance with mixed evidence.", "--tag", "operations",
		"--source", "sources/raw.md", "--source", "notes/observation.md",
		"--reason", "Create guidance", "--actor", "test")

	runDelete(t, repo, "notes/observation.md", "--reason", "Remove observation", "--actor", "test")

	assertMissing(t, repo, "notes/observation.md")
	assertExists(t, repo, "wiki/guidance.md")
	content := testfs.ReadFile(t, repo, "wiki/guidance.md")
	meta, _, _, err := frontmatter.Split([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Lumbrera.Sources) != 1 || meta.Lumbrera.Sources[0] != "sources/raw.md" {
		t.Fatalf("remaining evidence = %#v, want raw source", meta.Lumbrera.Sources)
	}
	if strings.Contains(content, "observation.md") || strings.Contains(content, "[source:") {
		t.Fatalf("wiki still references deleted note:\n%s", content)
	}
	assertVerify(t, repo)
}

func TestDeleteNoteCascadeDeletesWikiWithNoRemainingEvidence(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Observation\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observed behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	braintest.RunWrite(t, repo, "# Guidance\n\nDerived guidance.\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Guidance from a note.", "--tag", "operations",
		"--source", "notes/observation.md", "--reason", "Create guidance", "--actor", "test")
	braintest.RunWrite(t, repo, "# Related note\n\nSee [Guidance](../wiki/guidance.md).\n", "notes/related.md",
		"--title", "Related note", "--summary", "A note linking to guidance.", "--tag", "operations",
		"--reason", "Record related note", "--actor", "test")

	runDelete(t, repo, "notes/observation.md", "--reason", "Remove sole evidence", "--actor", "test")

	assertMissing(t, repo, "notes/observation.md")
	assertMissing(t, repo, "wiki/guidance.md")
	related := testfs.ReadFile(t, repo, "notes/related.md")
	if strings.Contains(related, "[Guidance]") || !strings.Contains(related, "See Guidance.") {
		t.Fatalf("inbound link from note was not cleaned:\n%s", related)
	}
	assertVerify(t, repo)
}

func TestDeleteWikiCleansInboundLinkFromNote(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nRaw evidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Guidance\n\nGuidance.\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Canonical guidance.", "--tag", "operations", "--source", "sources/raw.md",
		"--reason", "Create guidance", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\nSee [Guidance](../wiki/guidance.md).\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observation linking to guidance.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	runDelete(t, repo, "wiki/guidance.md", "--reason", "Remove guidance", "--actor", "test")
	content := testfs.ReadFile(t, repo, "notes/observation.md")
	if strings.Contains(content, "../wiki/guidance.md") || !strings.Contains(content, "See Guidance.") {
		t.Fatalf("wiki link was not cleaned from note:\n%s", content)
	}
	assertVerify(t, repo)
}

func TestDeleteManagedDocumentsCleansNoteLinks(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Target note\n\nTarget.\n", "notes/target.md",
		"--title", "Target note", "--summary", "The target note.", "--tag", "notes",
		"--reason", "Create target", "--actor", "test")
	braintest.RunWrite(t, repo, "# Referrer\n\nSee [Target](./target.md).\n", "notes/referrer.md",
		"--title", "Referrer", "--summary", "A referring note.", "--tag", "notes",
		"--reason", "Create referrer", "--actor", "test")

	runDelete(t, repo, "notes/target.md", "--reason", "Remove target", "--actor", "test")
	referrer := testfs.ReadFile(t, repo, "notes/referrer.md")
	if strings.Contains(referrer, "[Target]") || !strings.Contains(referrer, "See Target.") {
		t.Fatalf("note-to-note link was not cleaned:\n%s", referrer)
	}
	assertVerify(t, repo)
}

func TestDeleteSourceCleansOrdinaryLinkFromNote(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nRaw detail.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\nSee [raw detail](../sources/raw.md).\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observation linking to a source.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	runDelete(t, repo, "sources/raw.md", "--reason", "Remove raw", "--actor", "test")
	content := testfs.ReadFile(t, repo, "notes/observation.md")
	if strings.Contains(content, "../sources/raw.md") || !strings.Contains(content, "See raw detail.") {
		t.Fatalf("ordinary source link was not cleaned from note:\n%s", content)
	}
	assertVerify(t, repo)
}

func TestDeleteAssetScrubsReferencesFromNote(t *testing.T) {
	repo := braintest.InitBrain(t)
	asset := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(asset, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	braintest.RunAssetWrite(t, repo, "assets/diagram.png", asset, "--reason", "Add diagram", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\n![Diagram](../assets/diagram.png)\n\nSurviving text.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observation with an asset.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	runDelete(t, repo, "assets/diagram.png", "--reason", "Remove diagram", "--actor", "test")
	content := testfs.ReadFile(t, repo, "notes/observation.md")
	if strings.Contains(content, "diagram.png") || !strings.Contains(content, "Surviving text.") {
		t.Fatalf("asset reference was not cleaned from note:\n%s", content)
	}
	assertVerify(t, repo)
}

func TestDeleteNotePreservesUnrelatedMarkdownFormatting(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nRaw evidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observed behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	body := "# Guidance\n\n" +
		"Claim. [source: ../notes/observation.md]\n\n" +
		"Literal: `[source: ../notes/observation.md]`.\n\n" +
		"```python\nif ready:\n    run_task()\n```\n\n" +
		"Indented code:\n\n    run_task()\n\n" +
		"Text  with  intentional  spacing.  \nNext line.\n"
	braintest.RunWrite(t, repo, body, "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Guidance with mixed evidence.", "--tag", "operations",
		"--source", "sources/raw.md", "--source", "notes/observation.md",
		"--reason", "Create guidance", "--actor", "test")

	runDelete(t, repo, "notes/observation.md", "--reason", "Remove observation", "--actor", "test")

	content := testfs.ReadFile(t, repo, "wiki/guidance.md")
	for _, want := range []string{
		"Literal: `[source: ../notes/observation.md]`.",
		"```python\nif ready:\n    run_task()\n```",
		"Indented code:\n\n    run_task()",
		"Text  with  intentional  spacing.  \nNext line.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("wiki formatting did not preserve %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Claim. [source:") {
		t.Fatalf("active note citation survived deletion:\n%s", content)
	}
	assertVerify(t, repo)
}

func TestDeleteWikiCleansTitledAndReferenceLinksFromNote(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nRaw evidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Guidance\n\nGuidance.\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Canonical guidance.", "--tag", "operations", "--source", "sources/raw.md",
		"--reason", "Create guidance", "--actor", "test")
	body := "# Observation\n\n" +
		"See [Guidance](../wiki/guidance.md \"canonical\").\n\n" +
		"See [reference guidance][guidance].\n\n" +
		"[guidance]: ../wiki/guidance.md \"canonical\"\n\n" +
		"Literal: `[Guidance](../wiki/guidance.md)`.\n"
	braintest.RunWrite(t, repo, body, "notes/observation.md",
		"--title", "Observation", "--summary", "An observation linking to guidance.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	runDelete(t, repo, "wiki/guidance.md", "--reason", "Remove guidance", "--actor", "test")

	content := testfs.ReadFile(t, repo, "notes/observation.md")
	for _, want := range []string{
		"See Guidance.",
		"See reference guidance.",
		"Literal: `[Guidance](../wiki/guidance.md)`.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("note link cleanup did not preserve %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "[guidance]:") {
		t.Fatalf("note retained deleted wiki reference definition:\n%s", content)
	}
	assertVerify(t, repo)
}

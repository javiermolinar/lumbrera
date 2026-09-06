package markdown

import (
	"strings"
	"testing"
)

func TestRemoveEvidenceCitationsPreservesUnrelatedSourceBytes(t *testing.T) {
	body := "# Guide\n\n" +
		"Claim[source: ../notes/observation.md].\n\n" +
		"- Listed claim[source: ../notes/observation.md#result].\n\n" +
		"Literal: `[source: ../notes/observation.md]`.\n\n" +
		"```md\n[source: ../notes/observation.md]\n```\n\n" +
		"Indented code:\n\n    run_task()\n\n" +
		"Text  with  intentional  spacing.  \n"

	got, err := RemoveEvidenceCitations(body, "wiki/guide.md", "notes/observation.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Guide\n\n" +
		"Claim.\n\n" +
		"- Listed claim.\n\n" +
		"Literal: `[source: ../notes/observation.md]`.\n\n" +
		"```md\n[source: ../notes/observation.md]\n```\n\n" +
		"Indented code:\n\n    run_task()\n\n" +
		"Text  with  intentional  spacing.  \n"
	if got != want {
		t.Fatalf("unexpected citation rewrite:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnwrapLinksToPathSupportsTitlesAndReferenceLinks(t *testing.T) {
	body := "# Note\n\n" +
		"See [Target](../wiki/target.md \"canonical\").\n\n" +
		"See [Target `]` sample](../wiki/target.md).\n\n" +
		"See [**Target reference**][target].\n\n" +
		"[target]: ../wiki/target.md \"canonical\"\n\n" +
		"Literal: `[Target](../wiki/target.md)`.\n\n" +
		"Text  remains.\n"

	got, err := UnwrapLinksToPath(body, "notes/note.md", "wiki/target.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"See Target.",
		"See Target `]` sample.",
		"See **Target reference**.",
		"Literal: `[Target](../wiki/target.md)`.",
		"Text  remains.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten body does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[target]:") {
		t.Fatalf("rewritten body retained target reference definition:\n%s", got)
	}
	analysis, err := AnalyzeWithOptions("notes/note.md", got, AnalyzeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range analysis.LinkReferences {
		if ref.Path == "wiki/target.md" {
			t.Fatalf("rewritten body retained target link: %#v\n%s", ref, got)
		}
	}
}

func TestRemoveLinksAndImagesToPathPreservesCodeAndFormatting(t *testing.T) {
	body := "# Note\n\n" +
		"![Diagram](../assets/diagram.png \"current\")\n\n" +
		"See [the diagram][diagram].\n\n" +
		"[diagram]: ../assets/diagram.png \"current\"\n\n" +
		"```md\n![Diagram](../assets/diagram.png)\n```\n\n" +
		"Text  remains.\n"

	got, err := RemoveLinksAndImagesToPath(body, "notes/note.md", "assets/diagram.png")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "[diagram]:") || strings.Contains(got, "See [the diagram]") {
		t.Fatalf("rewritten body retained an active asset reference:\n%s", got)
	}
	for _, want := range []string{
		"```md\n![Diagram](../assets/diagram.png)\n```",
		"Text  remains.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten body does not preserve %q:\n%s", want, got)
		}
	}
	analysis, err := AnalyzeWithOptions("notes/note.md", got, AnalyzeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range analysis.LinkReferences {
		if ref.Path == "assets/diagram.png" {
			t.Fatalf("rewritten body retained target asset link: %#v\n%s", ref, got)
		}
	}
}

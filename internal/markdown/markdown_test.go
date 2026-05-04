package markdown

import "testing"

func TestAnalyzeExtractsH1WikiLinksAndSources(t *testing.T) {
	body := `# Write command

See [Related](./related.md) and [absolute external](https://example.com).

## Sources

- [Raw](../../sources/2026/05/04/raw.md)
`
	analysis, err := Analyze("wiki/design/write-command.md", body)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.FirstH1 != "Write command" {
		t.Fatalf("unexpected H1 %q", analysis.FirstH1)
	}
	if len(analysis.Links) != 1 || analysis.Links[0] != "wiki/design/related.md" {
		t.Fatalf("unexpected links %#v", analysis.Links)
	}
	if len(analysis.Sources) != 1 || analysis.Sources[0] != "sources/2026/05/04/raw.md" {
		t.Fatalf("unexpected sources %#v", analysis.Sources)
	}
}

func TestAnalyzeExtractsAnchorsAndSourceCitations(t *testing.T) {
	body := `# Topic

## Section One!

See [this section](#section-one) and [raw notes](../sources/raw.md#raw-notes).

Important claim. [source: ../sources/raw.md#raw-notes]

## Section One
`
	analysis, err := Analyze("wiki/topic.md", body)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(analysis.Anchors, "section-one") || !contains(analysis.Anchors, "section-one-1") {
		t.Fatalf("expected duplicate heading anchors, got %#v", analysis.Anchors)
	}
	if contains(analysis.Links, "wiki/topic.md") {
		t.Fatalf("self anchor should not be included in path-only links: %#v", analysis.Links)
	}
	if !contains(analysis.Links, "sources/raw.md") {
		t.Fatalf("expected source link path, got %#v", analysis.Links)
	}
	if len(analysis.LinkReferences) != 2 {
		t.Fatalf("unexpected link references: %#v", analysis.LinkReferences)
	}
	if len(analysis.SourceCitations) != 1 || analysis.SourceCitations[0] != (Reference{Path: "sources/raw.md", Anchor: "raw-notes"}) {
		t.Fatalf("unexpected source citations: %#v", analysis.SourceCitations)
	}
}

func TestAnalyzeSkipsCodeAndExternalSourceText(t *testing.T) {
	body := "# Topic\n\n" +
		"Literal code: `[source: ../sources/raw.md#missing]`.\n\n" +
		"External source marker text is not a local citation. [source: https://example.com/report]\n\n" +
		"Important claim. [source: ../sources/raw.md#raw-notes]\n"
	analysis, err := Analyze("wiki/topic.md", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.SourceCitations) != 1 || analysis.SourceCitations[0] != (Reference{Path: "sources/raw.md", Anchor: "raw-notes"}) {
		t.Fatalf("unexpected source citations: %#v", analysis.SourceCitations)
	}
}

func TestNormalizeReferencePreservesFragmentText(t *testing.T) {
	ref, ok, err := NormalizeReference("wiki/topic.md", "../sources/raw.md#evidence.")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || ref != (Reference{Path: "sources/raw.md", Anchor: "evidence."}) {
		t.Fatalf("unexpected reference: ok=%v ref=%#v", ok, ref)
	}

	ref, ok, err = NormalizeReference("wiki/topic.md", "../sources/raw.md#evidence?bad")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || ref != (Reference{Path: "sources/raw.md", Anchor: "evidence?bad"}) {
		t.Fatalf("unexpected reference with query-like fragment: ok=%v ref=%#v", ok, ref)
	}
}

func TestAppendSourcesSectionRegeneratesSources(t *testing.T) {
	body := "# Topic\n\nBody.\n\n## Sources\n\n- [Old](../../sources/old.md)\n\n## Related\n\nMore.\n"
	got := AppendSourcesSection(body, "wiki/topic.md", []string{"sources/raw.md"})
	wantContains := "## Related\n\nMore.\n\n## Sources\n\n- [Raw](../sources/raw.md)\n"
	if got != "# Topic\n\nBody.\n\n## Related\n\nMore.\n\n## Sources\n\n- [Raw](../sources/raw.md)\n" {
		t.Fatalf("unexpected regenerated sources section:\n%s\nwant:\n%s", got, wantContains)
	}
}

func TestNormalizeLinkRejectsOutsideRepo(t *testing.T) {
	if _, err := NormalizeLink("wiki/topic.md", "../../outside.md"); err == nil {
		t.Fatal("expected outside repo link to be rejected")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

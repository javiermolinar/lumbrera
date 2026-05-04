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

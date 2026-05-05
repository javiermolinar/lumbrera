package markdown

import "testing"

func TestSplitSectionsUsesParserHeadings(t *testing.T) {
	body := "Intro paragraph.\n\n# Title\n\nTitle body.\n\n## Details\n\nDetail body.\n\n# Title\n\nSecond title body.\n"

	sections, err := SplitSections(body)
	if err != nil {
		t.Fatalf("split sections: %v", err)
	}
	want := []Section{
		{Ordinal: 1, Body: "Intro paragraph."},
		{Ordinal: 2, Heading: "Title", Anchor: "title", Level: 1, Body: "Title body."},
		{Ordinal: 3, Heading: "Details", Anchor: "details", Level: 2, Body: "Detail body."},
		{Ordinal: 4, Heading: "Title", Anchor: "title-1", Level: 1, Body: "Second title body."},
	}
	if len(sections) != len(want) {
		t.Fatalf("section count = %d, want %d: %#v", len(sections), len(want), sections)
	}
	for i := range want {
		if sections[i] != want[i] {
			t.Fatalf("section %d = %#v, want %#v", i, sections[i], want[i])
		}
	}
}

func TestSplitSectionsIgnoresFencedHeadings(t *testing.T) {
	body := "# Real\n\n```md\n# Not a heading\n```\n\nAfter.\n"

	sections, err := SplitSections(body)
	if err != nil {
		t.Fatalf("split sections: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("section count = %d, want 1: %#v", len(sections), sections)
	}
	if sections[0].Heading != "Real" || sections[0].Anchor != "real" || sections[0].Level != 1 {
		t.Fatalf("unexpected heading metadata: %#v", sections[0])
	}
	if sections[0].Body != "```md\n# Not a heading\n```\n\nAfter." {
		t.Fatalf("section body = %q", sections[0].Body)
	}
}

func TestSplitSectionsHandlesSetextHeadings(t *testing.T) {
	sections, err := SplitSections("Setext Title\n============\n\nBody.\n")
	if err != nil {
		t.Fatalf("split sections: %v", err)
	}
	want := Section{Ordinal: 1, Heading: "Setext Title", Anchor: "setext-title", Level: 1, Body: "Body."}
	if len(sections) != 1 || sections[0] != want {
		t.Fatalf("sections = %#v, want %#v", sections, []Section{want})
	}
}

func TestSplitSectionsWithoutHeadingsReturnsBodySection(t *testing.T) {
	sections, err := SplitSections("\n\nPlain body.\n")
	if err != nil {
		t.Fatalf("split sections: %v", err)
	}
	want := Section{Ordinal: 1, Body: "Plain body."}
	if len(sections) != 1 || sections[0] != want {
		t.Fatalf("sections = %#v, want %#v", sections, []Section{want})
	}
}

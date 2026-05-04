package frontmatter

import (
	"strings"
	"testing"
)

func TestRenderAndSplitGeneratedFrontmatter(t *testing.T) {
	doc := New("wiki", "Write command", "Describes how the write command works.", []string{"cli", "brain", "cli"}, []string{"sources/raw.md"}, []string{"wiki/related.md"})
	content, err := Attach(doc, "# Write command\n\nBody.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("expected frontmatter, got %q", content)
	}
	if !strings.Contains(content, "schema: document-v1") {
		t.Fatalf("expected schema in frontmatter:\n%s", content)
	}

	got, body, has, err := Split([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected frontmatter")
	}
	if got.Title != "Write command" || got.Lumbrera.Kind != "wiki" {
		t.Fatalf("unexpected document: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "brain" || got.Tags[1] != "cli" {
		t.Fatalf("tags not sorted/unique: %#v", got.Tags)
	}
	if body != "# Write command\n\nBody.\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestRenderRejectsMissingWikiSummaryAndTags(t *testing.T) {
	if _, err := Render(New("wiki", "Tempo architecture", "", []string{"tempo"}, []string{"sources/tempo-docs-combined.md"}, nil)); err == nil {
		t.Fatal("expected missing summary to be rejected")
	}
	if _, err := Render(New("wiki", "Tempo architecture", "Tempo architecture summary.", nil, []string{"sources/tempo-docs-combined.md"}, nil)); err == nil {
		t.Fatal("expected missing tags to be rejected")
	}
}

func TestRenderRejectsInvalidWikiSummary(t *testing.T) {
	if _, err := Render(New("wiki", "Summary", "Line one.\nLine two.", []string{"summary"}, []string{"sources/raw.md"}, nil)); err == nil {
		t.Fatal("expected multiline summary to be rejected")
	}
}

func TestRenderRejectsInvalidWikiTags(t *testing.T) {
	tooMany := []string{"one", "two", "three", "four", "five", "six"}
	if _, err := Render(New("wiki", "Tagged", "Tagged summary.", tooMany, []string{"sources/raw.md"}, nil)); err == nil {
		t.Fatal("expected too many tags to be rejected")
	}
	if _, err := Render(New("wiki", "Tagged", "Tagged summary.", []string{"Bad Tag"}, []string{"sources/raw.md"}, nil)); err == nil {
		t.Fatal("expected invalid tag slug to be rejected")
	}
}

func TestSplitRejectsNonLumbreraFrontmatter(t *testing.T) {
	_, _, has, err := Split([]byte("---\ntitle: Manual\n---\n\n# Manual\n"))
	if !has {
		t.Fatal("expected frontmatter to be detected")
	}
	if err == nil {
		t.Fatal("expected non-Lumbrera frontmatter to be rejected")
	}
}

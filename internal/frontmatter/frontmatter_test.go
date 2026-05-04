package frontmatter

import (
	"strings"
	"testing"
)

func TestRenderAndSplitGeneratedFrontmatter(t *testing.T) {
	doc := New("wiki", "Write command", "", []string{"cli", "brain", "cli"}, []string{"sources/raw.md"}, []string{"wiki/related.md"})
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

func TestSplitRejectsNonLumbreraFrontmatter(t *testing.T) {
	_, _, has, err := Split([]byte("---\ntitle: Manual\n---\n\n# Manual\n"))
	if !has {
		t.Fatal("expected frontmatter to be detected")
	}
	if err == nil {
		t.Fatal("expected non-Lumbrera frontmatter to be rejected")
	}
}

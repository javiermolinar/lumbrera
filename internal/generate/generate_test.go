package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func TestIndexForRepoUsesWikiMetadataAndReferencedSourcePaths(t *testing.T) {
	repo := t.TempDir()
	writeRaw(t, repo, "sources/2026/05/04/raw.md", "# Raw source\n")
	writeDoc(t, repo, "wiki/architecture/topic.md", frontmatter.New("wiki", "Topic title", "", nil, []string{"sources/2026/05/04/raw.md"}, nil), "# Ignored wiki H1\n")

	index, err := IndexForRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- 2026/\n  - 05/\n    - 04/\n      - [Raw](sources/2026/05/04/raw.md)",
		"- architecture/\n  - [Topic title](wiki/architecture/topic.md)",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("expected index to contain %q, got:\n%s", want, index)
		}
	}
}

func writeRaw(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDoc(t *testing.T, repo, rel string, meta frontmatter.Document, body string) {
	t.Helper()
	content, err := frontmatter.Attach(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

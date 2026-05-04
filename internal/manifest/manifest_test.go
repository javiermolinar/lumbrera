package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSortsEntries(t *testing.T) {
	got, err := Generate([]Entry{{Path: "wiki/b.md", Hash: "bbb"}, {Path: "sources/a.md", Hash: "aaa"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "lumbrera-sum-v1 sha256\nsources/a.md sha256:aaa\nwiki/b.md sha256:bbb\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEntriesForRepoIncludeOnlyWikiMarkdown(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "sources/raw.md", "# Raw\n")
	writeFile(t, repo, "wiki/topic.md", "# Topic\n")

	entries, err := EntriesForRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "wiki/topic.md" {
		t.Fatalf("expected only wiki entries, got %#v", entries)
	}
}

func TestHashContentNormalizesCRLF(t *testing.T) {
	if HashContent([]byte("a\r\nb\r\n")) != HashContent([]byte("a\nb\n")) {
		t.Fatal("expected CRLF and LF content to hash the same")
	}
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

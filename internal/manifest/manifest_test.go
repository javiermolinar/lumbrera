package manifest

import (
	"testing"

	"github.com/javiermolinar/lumbrera/internal/testfs"
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

func TestEntriesForRepoIncludeManagedMarkdown(t *testing.T) {
	repo := t.TempDir()
	testfs.WriteFile(t, repo, "sources/raw.md", "# Raw\n")
	testfs.WriteFile(t, repo, "notes/observation.md", "# Observation\n")
	testfs.WriteFile(t, repo, "wiki/topic.md", "# Topic\n")

	entries, err := EntriesForRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "notes/observation.md" || entries[1].Path != "wiki/topic.md" {
		t.Fatalf("expected note and wiki entries, got %#v", entries)
	}
}

func TestHashContentNormalizesCRLF(t *testing.T) {
	if HashContent([]byte("a\r\nb\r\n")) != HashContent([]byte("a\nb\n")) {
		t.Fatal("expected CRLF and LF content to hash the same")
	}
}

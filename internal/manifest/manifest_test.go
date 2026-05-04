package manifest

import "testing"

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

func TestHashContentNormalizesCRLF(t *testing.T) {
	if HashContent([]byte("a\r\nb\r\n")) != HashContent([]byte("a\nb\n")) {
		t.Fatal("expected CRLF and LF content to hash the same")
	}
}

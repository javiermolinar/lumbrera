package brainfs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownPathsSortedAcrossRoots(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "wiki/z.md", "# Z\n")
	writeFile(t, repo, "sources/a.md", "# A\n")
	writeFile(t, repo, "wiki/ignore.txt", "ignore\n")

	paths, err := MarkdownPaths(repo, []string{"wiki", "sources", "missing"})
	if err != nil {
		t.Fatalf("MarkdownPaths failed: %v", err)
	}
	want := []string{"sources/a.md", "wiki/z.md"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestWalkMarkdownRejectsMarkdownSymlink(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "wiki/target.md", "# Target\n")
	link := filepath.Join(repo, "wiki", "link.md")
	if err := os.Symlink("target.md", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := WalkMarkdown(repo, []string{"wiki"}, func(file MarkdownFile) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "wiki/link.md is not a regular Markdown file") {
		t.Fatalf("error = %v, want Markdown symlink rejection", err)
	}
}

func TestValidateDirectoryRejectsRootSymlink(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "real/a.md", "# A\n")
	if err := os.Symlink("real", filepath.Join(repo, "wiki")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	exists, err := ValidateDirectory(repo, "wiki", false)
	if err == nil || exists || !strings.Contains(err.Error(), "wiki must be a real directory") {
		t.Fatalf("exists=%v err=%v, want root symlink rejection", exists, err)
	}
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

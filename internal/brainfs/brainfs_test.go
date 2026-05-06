package brainfs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/testfs"
)

func TestMarkdownPathsSortedAcrossRoots(t *testing.T) {
	repo := t.TempDir()
	testfs.WriteFile(t, repo, "wiki/z.md", "# Z\n")
	testfs.WriteFile(t, repo, "sources/a.md", "# A\n")
	testfs.WriteFile(t, repo, "wiki/ignore.txt", "ignore\n")

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
	testfs.WriteFile(t, repo, "wiki/target.md", "# Target\n")
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
	testfs.WriteFile(t, repo, "real/a.md", "# A\n")
	if err := os.Symlink("real", filepath.Join(repo, "wiki")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	exists, err := ValidateDirectory(repo, "wiki", false)
	if err == nil || exists || !strings.Contains(err.Error(), "wiki must be a real directory") {
		t.Fatalf("exists=%v err=%v, want root symlink rejection", exists, err)
	}
}

package testfs

import (
	"os"
	"path/filepath"
	"testing"
)

func WriteFile(t testing.TB, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func ReadFile(t testing.TB, repo, rel string) string {
	t.Helper()
	return ReadPath(t, filepath.Join(repo, filepath.FromSlash(rel)))
}

func ReadPath(t testing.TB, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

package braintest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func InitBrain(t testing.TB) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return repo
}

func RunWrite(t testing.TB, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--brain", repo}, args...)
	if err := writecmd.Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func ReadFile(t testing.TB, repo, rel string) string {
	t.Helper()
	return testfs.ReadFile(t, repo, rel)
}

func WriteFile(t testing.TB, repo, rel, content string) {
	t.Helper()
	testfs.WriteFile(t, repo, rel, content)
}

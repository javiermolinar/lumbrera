package indexruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestEnsureFreshAutoRebuildsMissingIndex(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention runtimeunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody mentions runtimeunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	if err := EnsureFresh(context.Background(), repo); err != nil {
		t.Fatalf("EnsureFresh failed: %v", err)
	}
	status, err := searchindex.CheckStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}
	if status.State != searchindex.StatusFresh {
		t.Fatalf("status = %s, want fresh: %#v", status.State, status)
	}
}

func TestEnsureFreshRejectsVerifyDrift(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
	writeFile(t, repo, "tags.md", readFile(t, repo, "tags.md")+"\nManual drift.\n")

	err := EnsureFresh(context.Background(), repo)
	if err == nil {
		t.Fatal("EnsureFresh succeeded with generated-file drift, want error")
	}
	if !strings.Contains(err.Error(), "verify") || !strings.Contains(err.Error(), "tags.md") {
		t.Fatalf("error = %v, want verify tags.md error", err)
	}
}

func initBrain(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return repo
}

func runWrite(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--brain", repo}, args...)
	if err := writecmd.Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func readFile(t *testing.T, repo, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
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

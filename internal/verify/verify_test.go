package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/verify"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestFixRegeneratesStaleGeneratedFiles(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")

	// Manually add a source file without going through lumbrera write.
	path := filepath.Join(repo, "sources", "extra.md")
	if err := os.WriteFile(path, []byte("# Extra source\n\nExtra notes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Check should fail: INDEX.md is stale.
	if err := verify.Check(repo, verify.Options{}); err == nil {
		t.Fatal("expected Check to fail with stale INDEX.md")
	}

	// Run with Fix should regenerate and pass.
	if err := verify.Run(repo, verify.Options{Fix: true}); err != nil {
		t.Fatalf("Run with Fix should succeed: %v", err)
	}

	// Subsequent Check without Fix should also pass.
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("Check after Fix should pass: %v", err)
	}

	// SOURCES.md should now list the manually-added source.
	sources := testfs.ReadPath(t, filepath.Join(repo, "SOURCES.md"))
	if !strings.Contains(sources, "sources/extra.md") {
		t.Fatalf("expected SOURCES.md to include extra.md after fix, got:\n%s", sources)
	}
}

func TestStaleErrorIncludesLineDiff(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")

	// Manually add a source file to make INDEX.md stale.
	path := filepath.Join(repo, "sources", "extra.md")
	if err := os.WriteFile(path, []byte("# Extra source\n\nExtra notes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verify.Check(repo, verify.Options{})
	if err == nil {
		t.Fatal("expected stale error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "line ") {
		t.Fatalf("expected error to include line-level diff, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "verify --fix") {
		t.Fatalf("expected error to mention verify --fix, got: %s", errMsg)
	}
}

func TestFixWithoutStalenessIsIdempotent(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")

	// Run with Fix on an already-consistent brain should be a no-op.
	if err := verify.Run(repo, verify.Options{Fix: true}); err != nil {
		t.Fatalf("Fix on consistent brain should succeed: %v", err)
	}
}

func TestCheckDoesNotRepairMissingWikiDocumentID(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	withoutID := removeIDLine(testfs.ReadPath(t, path))
	if err := os.WriteFile(path, []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verify.Check(repo, verify.Options{}); err == nil {
		t.Fatal("Check repaired or accepted a missing document ID; want pure validation error")
	}
	if strings.Contains(testfs.ReadPath(t, path), "id: doc_") {
		t.Fatal("Check mutated the wiki file by repairing the missing ID")
	}

	if err := verify.Run(repo, verify.Options{}); err != nil {
		t.Fatalf("Run should still repair missing document ID: %v", err)
	}
	if !strings.Contains(testfs.ReadPath(t, path), "id: doc_") {
		t.Fatal("Run did not repair missing document ID")
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

func removeIDLine(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "id: doc_") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

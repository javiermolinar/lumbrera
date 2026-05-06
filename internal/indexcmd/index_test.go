package indexcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestIndexStatusMissingDoesNotCreateIndex(t *testing.T) {
	repo := initBrain(t)

	if err := Run([]string{"--brain", repo, "--status"}); err != nil {
		t.Fatalf("index status failed: %v", err)
	}
	if _, err := os.Stat(searchindex.SearchIndexPath(repo)); !os.IsNotExist(err) {
		t.Fatalf("status should not create index, stat err=%v", err)
	}
}

func TestIndexRebuildCreatesFreshIndex(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention indexunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody mentions topicunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	if err := Run([]string{"--brain", repo, "--rebuild"}); err != nil {
		t.Fatalf("index rebuild failed: %v", err)
	}
	status, err := searchindex.CheckStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("check status after rebuild: %v", err)
	}
	if status.State != searchindex.StatusFresh {
		t.Fatalf("status after rebuild = %q, want fresh: %#v", status.State, status)
	}
	assertIndexMatches(t, repo, "topicunique", 1)
	assertIndexMatches(t, repo, "indexunique", 1)
}

func TestIndexRebuildRepairsMissingModifiedDate(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes mention dateunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody mentions dateunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	withoutModifiedDate := removeModifiedDateLine(readFile(t, repo, "wiki/topic.md"))
	if err := os.WriteFile(path, []byte(withoutModifiedDate), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := generate.FilesForRepo(repo)
	if err != nil {
		t.Fatalf("generate files for legacy fixture: %v", err)
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		t.Fatalf("write generated files for legacy fixture: %v", err)
	}

	if err := Run([]string{"--brain", repo, "--rebuild"}); err != nil {
		t.Fatalf("index rebuild should repair missing modified date: %v", err)
	}
	meta, _, _, err := frontmatter.Split([]byte(readFile(t, repo, "wiki/topic.md")))
	if err != nil {
		t.Fatalf("read repaired wiki frontmatter: %v", err)
	}
	if meta.Lumbrera.ModifiedDate == "" {
		t.Fatal("index rebuild did not repair modified date")
	}
	assertIndexMatches(t, repo, "dateunique", 2)
}

func TestIndexRebuildRunsVerify(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "tags.md")
	if err := os.WriteFile(path, []byte(readFile(t, repo, "tags.md")+"\nManual drift.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo, "--rebuild"})
	if err == nil {
		t.Fatal("index rebuild succeeded with verify drift, want error")
	}
	if !strings.Contains(err.Error(), "verify") || !strings.Contains(err.Error(), "tags.md") {
		t.Fatalf("rebuild error = %v, want verify tags.md error", err)
	}
	if _, statErr := os.Stat(searchindex.SearchIndexPath(repo)); !os.IsNotExist(statErr) {
		t.Fatalf("failed rebuild should not create index, stat err=%v", statErr)
	}
}

func TestIndexRebuildDoesNotRepairMissingWikiDocumentID(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	path := filepath.Join(repo, "wiki", "topic.md")
	withoutID := removeIDLine(readFile(t, repo, "wiki/topic.md"))
	if err := os.WriteFile(path, []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo, "--rebuild"})
	if err == nil {
		t.Fatal("index rebuild repaired or accepted missing ID, want verify error")
	}
	if strings.Contains(readFile(t, repo, "wiki/topic.md"), "id: doc_") {
		t.Fatal("index rebuild mutated wiki file by repairing missing ID")
	}
	if _, statErr := os.Stat(searchindex.SearchIndexPath(repo)); !os.IsNotExist(statErr) {
		t.Fatalf("failed rebuild should not create index, stat err=%v", statErr)
	}
}

func TestIndexRejectsInvalidFlagCombination(t *testing.T) {
	if err := Run(nil); err == nil {
		t.Fatal("index without mode succeeded, want error")
	}
	if err := Run([]string{"--status", "--rebuild"}); err == nil {
		t.Fatal("index with both modes succeeded, want error")
	}
	if err := Run([]string{"unexpected", "--status"}); err == nil {
		t.Fatal("index with positional arg succeeded, want error")
	}
}

func TestIndexHelp(t *testing.T) {
	if err := Run([]string{"--help"}); err != nil {
		t.Fatalf("index help failed: %v", err)
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

func removeModifiedDateLine(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "modified_date:") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func assertIndexMatches(t *testing.T, repo, term string, want int) {
	t.Helper()
	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(repo))
	if err != nil {
		t.Fatalf("open search index: %v", err)
	}
	defer db.Close()

	var got int
	if err := db.QueryRow(`SELECT count(*) FROM sections_fts WHERE sections_fts MATCH ?`, term).Scan(&got); err != nil {
		t.Fatalf("count FTS matches for %q: %v", term, err)
	}
	if got != want {
		t.Fatalf("FTS matches for %q = %d, want %d", term, got, want)
	}
}

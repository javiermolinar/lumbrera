package searchindex

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func TestRebuildBrainBuildsSQLiteIndexFromRepoMarkdown(t *testing.T) {
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/unreferenced.md", "# Orphan Source\n\nThis source has orphanunique evidence.\n")
	writeFile(t, repo, "sources/raw.md", "# Raw Source\n\nRaw body mentions rawunique.\n")

	wikiID := "doc_0123456789abcdef0123456789abcdef"
	wikiMeta := frontmatter.NewWithID(
		wikiID,
		KindWiki,
		"Tempo limits",
		"Tempo limits summary.",
		[]string{"tempo", "limits"},
		[]string{"sources/raw.md"},
		nil,
	)
	wikiContent, err := frontmatter.Attach(wikiMeta, "# Tempo limits\n\nTempo body mentions tempounique.\n\n## Ingestion\n\nDistributors enforce limits.\n")
	if err != nil {
		t.Fatalf("attach wiki frontmatter: %v", err)
	}
	writeFile(t, repo, "wiki/tempo.md", wikiContent)

	if err := RebuildBrain(context.Background(), repo); err != nil {
		t.Fatalf("rebuild brain: %v", err)
	}
	if _, err := os.Stat(SearchIndexPath(repo)); err != nil {
		t.Fatalf("expected search index at %s: %v", SearchIndexPath(repo), err)
	}

	db, err := OpenSQLite(SearchIndexPath(repo))
	if err != nil {
		t.Fatalf("open rebuilt search index: %v", err)
	}
	defer db.Close()

	paths := queryColumn(t, db, `SELECT path FROM documents ORDER BY path`)
	wantPaths := []string{"sources/raw.md", "sources/unreferenced.md", "wiki/tempo.md"}
	assertStringSlice(t, paths, wantPaths, "document paths")

	sectionPaths := queryColumn(t, db, `SELECT path || ':' || CAST(ordinal AS TEXT) FROM sections ORDER BY rowid`)
	wantSectionPaths := []string{"sources/raw.md:1", "sources/unreferenced.md:1", "wiki/tempo.md:1", "wiki/tempo.md:2"}
	assertStringSlice(t, sectionPaths, wantSectionPaths, "section row order")

	if got := countFTSMatches(t, db, "tempounique"); got != 1 {
		t.Fatalf("wiki FTS matches = %d, want 1", got)
	}
	if got := countFTSMatches(t, db, "orphanunique"); got != 1 {
		t.Fatalf("unreferenced source FTS matches = %d, want 1", got)
	}

	meta := readMeta(t, db)
	for _, key := range []string{"schema_version", manifestHashMetaKey, indexedPathsHashMetaKey, indexerVersionMetaKey, markdownSectionsVersionMetaKey, manifestDebugMetaKey} {
		if meta[key] == "" {
			t.Fatalf("metadata %q is empty: %#v", key, meta)
		}
	}
	manifest := meta[manifestDebugMetaKey]
	for _, want := range []string{
		"schema_version=1",
		"indexer_version=1",
		"markdown_sections_version=1",
		"path=sources/raw.md ",
		"path=sources/unreferenced.md ",
		"path=wiki/tempo.md ",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest debug missing %q:\n%s", want, manifest)
		}
	}
}

func TestRecordsForRepoIsDeterministic(t *testing.T) {
	repo := newBrainRepo(t)
	writeFile(t, repo, "wiki/b.md", wikiContent(t, "doc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "B", "B summary.", "bravo"))
	writeFile(t, repo, "sources/z.md", "# Zeta\n\nZ body.\n")
	writeFile(t, repo, "sources/a.md", "# Alpha\n\nA body.\n")
	writeFile(t, repo, "wiki/a.md", wikiContent(t, "doc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "A", "A summary.", "alpha"))

	docs, sections, metadata, err := RecordsForRepo(repo)
	if err != nil {
		t.Fatalf("records for repo: %v", err)
	}

	var docPaths []string
	for _, doc := range docs {
		docPaths = append(docPaths, doc.Path)
	}
	assertStringSlice(t, docPaths, []string{"sources/a.md", "sources/z.md", "wiki/a.md", "wiki/b.md"}, "records document order")

	var sectionPaths []string
	for _, section := range sections {
		for _, doc := range docs {
			if doc.ID == section.DocumentID {
				sectionPaths = append(sectionPaths, doc.Path)
				break
			}
		}
	}
	assertStringSlice(t, sectionPaths, []string{"sources/a.md", "sources/z.md", "wiki/a.md", "wiki/b.md"}, "records section order")

	manifest := metadata[manifestDebugMetaKey]
	wantManifestOrder := []string{"path=sources/a.md", "path=sources/z.md", "path=wiki/a.md", "path=wiki/b.md"}
	last := -1
	for _, marker := range wantManifestOrder {
		idx := strings.Index(manifest, marker)
		if idx < 0 {
			t.Fatalf("manifest missing %q:\n%s", marker, manifest)
		}
		if idx <= last {
			t.Fatalf("manifest marker %q out of order:\n%s", marker, manifest)
		}
		last = idx
	}
}

func TestRebuildBrainRejectsNonBrainRepo(t *testing.T) {
	if err := RebuildBrain(context.Background(), t.TempDir()); err == nil {
		t.Fatal("RebuildBrain succeeded for non-brain repo, want error")
	}
}

func TestRebuildBrainRejectsSymlinkedRoots(t *testing.T) {
	t.Run("brain", func(t *testing.T) {
		repo := newBrainRepo(t)
		externalBrain := filepath.Join(t.TempDir(), "external-brain")
		if err := os.MkdirAll(externalBrain, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(externalBrain, "VERSION"), []byte(brain.Version+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(repo, ".brain")); err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, externalBrain, filepath.Join(repo, ".brain"))

		if err := RebuildBrain(context.Background(), repo); err == nil || !strings.Contains(err.Error(), ".brain") {
			t.Fatalf("RebuildBrain with symlinked .brain error = %v, want .brain error", err)
		}
	})

	t.Run("sources", func(t *testing.T) {
		repo := newBrainRepo(t)
		externalSources := filepath.Join(t.TempDir(), "external-sources")
		if err := os.MkdirAll(externalSources, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(repo, "sources")); err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, externalSources, filepath.Join(repo, "sources"))

		if err := RebuildBrain(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "sources") {
			t.Fatalf("RebuildBrain with symlinked sources error = %v, want sources error", err)
		}
	})
}

func TestRebuildBrainFailureDoesNotLeaveOrReplaceFinalIndex(t *testing.T) {
	t.Run("no final index is created", func(t *testing.T) {
		repo := newBrainRepo(t)
		writeFile(t, repo, "wiki/bad.md", "# Missing frontmatter\n")

		if err := RebuildBrain(context.Background(), repo); err == nil {
			t.Fatal("RebuildBrain succeeded with invalid wiki, want error")
		}
		if _, err := os.Stat(SearchIndexPath(repo)); !os.IsNotExist(err) {
			t.Fatalf("final search index should not exist after failed rebuild, stat err=%v", err)
		}
	})

	t.Run("existing final index is preserved", func(t *testing.T) {
		repo := newBrainRepo(t)
		writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw preserved.\n")
		writeFile(t, repo, "wiki/good.md", wikiContent(t, "doc_cccccccccccccccccccccccccccccccc", "Good", "Good summary.", "good"))
		if err := RebuildBrain(context.Background(), repo); err != nil {
			t.Fatalf("initial rebuild: %v", err)
		}

		writeFile(t, repo, "wiki/bad.md", "# Missing frontmatter\n")
		if err := RebuildBrain(context.Background(), repo); err == nil {
			t.Fatal("RebuildBrain succeeded with invalid wiki, want error")
		}

		db, err := OpenSQLite(SearchIndexPath(repo))
		if err != nil {
			t.Fatalf("open preserved index: %v", err)
		}
		defer db.Close()
		if got := countFTSMatches(t, db, "preserved"); got != 1 {
			t.Fatalf("preserved index matches = %d, want 1", got)
		}
	})
}

func newBrainRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, rel := range []string{".brain", "sources", "wiki"} {
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatalf("create %s: %v", rel, err)
		}
	}
	writeFile(t, repo, brain.MarkerPath, brain.Version+"\n")
	return repo
}

func wikiContent(t *testing.T, id, title, summary, tag string) string {
	t.Helper()
	meta := frontmatter.NewWithID(id, KindWiki, title, summary, []string{tag}, nil, nil)
	content, err := frontmatter.Attach(meta, "# "+title+"\n\nBody.\n")
	if err != nil {
		t.Fatalf("attach wiki content: %v", err)
	}
	return content
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	absPath := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func queryColumn(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query column: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column: %v", err)
	}
	return out
}

func readMeta(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT key, value FROM meta`)
	if err != nil {
		t.Fatalf("query meta: %v", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("scan meta: %v", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate meta: %v", err)
	}
	return out
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
}

func assertStringSlice(t *testing.T, got, want []string, name string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d: %#v", name, len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q; all=%#v", name, i, got[i], want[i], got)
		}
	}
}

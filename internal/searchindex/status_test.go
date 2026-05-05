package searchindex

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckStatusMissingFreshAndStale(t *testing.T) {
	ctx := context.Background()
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw status body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_dddddddddddddddddddddddddddddddd", "Topic", "Topic summary.", "topic"))

	status, err := CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check missing status: %v", err)
	}
	assertStatus(t, status, StatusMissing)
	if status.Exists {
		t.Fatalf("missing status exists = true: %#v", status)
	}
	if _, err := os.Stat(SearchIndexPath(repo)); !os.IsNotExist(err) {
		t.Fatalf("CheckStatus should not create index, stat err=%v", err)
	}

	if err := RebuildBrain(ctx, repo); err != nil {
		t.Fatalf("rebuild brain: %v", err)
	}
	status, err = CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check fresh status: %v", err)
	}
	assertStatus(t, status, StatusFresh)
	if !status.Exists || status.SchemaVersion != CurrentSchemaVersion || status.ManifestHash == "" || status.ExpectedHash == "" {
		t.Fatalf("unexpected fresh status fields: %#v", status)
	}

	writeFile(t, repo, "sources/new.md", "# New\n\nNew status body.\n")
	status, err = CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check stale status: %v", err)
	}
	assertStatus(t, status, StatusStale)
	if !strings.Contains(status.Reason, indexedPathsHashMetaKey) && !strings.Contains(status.Reason, manifestHashMetaKey) {
		t.Fatalf("stale reason = %q, want metadata difference", status.Reason)
	}
}

func TestCheckStatusDetectsContentHashStale(t *testing.T) {
	ctx := context.Background()
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nBefore status body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Topic", "Topic summary.", "topic"))
	if err := RebuildBrain(ctx, repo); err != nil {
		t.Fatalf("rebuild brain: %v", err)
	}

	writeFile(t, repo, "sources/raw.md", "# Raw\n\nAfter status body.\n")
	status, err := CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check stale status: %v", err)
	}
	assertStatus(t, status, StatusStale)
	if !strings.Contains(status.Reason, manifestHashMetaKey) {
		t.Fatalf("stale reason = %q, want manifest hash difference", status.Reason)
	}
}

func TestCheckStatusIncompatibleSchema(t *testing.T) {
	ctx := context.Background()
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_ffffffffffffffffffffffffffffffff", "Topic", "Topic summary.", "topic"))
	if err := RebuildBrain(ctx, repo); err != nil {
		t.Fatalf("rebuild brain: %v", err)
	}

	db := openIndexDB(t, repo)
	if _, err := db.Exec(`UPDATE meta SET value = '999' WHERE key = 'schema_version'`); err != nil {
		t.Fatalf("update schema version: %v", err)
	}
	_ = db.Close()

	status, err := CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check incompatible status: %v", err)
	}
	assertStatus(t, status, StatusIncompatible)
	if status.SchemaVersion != 999 || !strings.Contains(status.Reason, "schema version") {
		t.Fatalf("unexpected incompatible status: %#v", status)
	}
}

func TestCheckStatusIncompatibleMissingMetadata(t *testing.T) {
	ctx := context.Background()
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_11111111111111111111111111111111", "Topic", "Topic summary.", "topic"))
	if err := RebuildBrain(ctx, repo); err != nil {
		t.Fatalf("rebuild brain: %v", err)
	}

	db := openIndexDB(t, repo)
	if _, err := db.Exec(`DELETE FROM meta WHERE key = 'manifest_hash'`); err != nil {
		t.Fatalf("delete manifest hash: %v", err)
	}
	_ = db.Close()

	status, err := CheckStatus(ctx, repo)
	if err != nil {
		t.Fatalf("check missing metadata status: %v", err)
	}
	assertStatus(t, status, StatusStale)
	if !strings.Contains(status.Reason, "manifest_hash") {
		t.Fatalf("stale reason = %q, want missing manifest_hash", status.Reason)
	}
}

func TestCheckStatusIncompatibleIndexPath(t *testing.T) {
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_22222222222222222222222222222222", "Topic", "Topic summary.", "topic"))
	writeFile(t, repo, SearchIndexRelPath, "not sqlite\n")

	status, err := CheckStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("check incompatible path status: %v", err)
	}
	assertStatus(t, status, StatusIncompatible)
}

func TestCheckStatusRejectsNonBrainRepo(t *testing.T) {
	if _, err := CheckStatus(context.Background(), t.TempDir()); err == nil {
		t.Fatal("CheckStatus succeeded for non-brain repo, want error")
	}
}

func TestManifestMetadataForRepoIsDeterministic(t *testing.T) {
	repo := newBrainRepo(t)
	writeFile(t, repo, "wiki/b.md", wikiContent(t, "doc_33333333333333333333333333333333", "B", "B summary.", "bravo"))
	writeFile(t, repo, "sources/a.md", "# A\n\nA body.\n")

	first, err := ManifestMetadataForRepo(repo)
	if err != nil {
		t.Fatalf("first manifest metadata: %v", err)
	}
	second, err := ManifestMetadataForRepo(repo)
	if err != nil {
		t.Fatalf("second manifest metadata: %v", err)
	}
	for _, key := range []string{manifestHashMetaKey, indexedPathsHashMetaKey, indexerVersionMetaKey, markdownSectionsVersionMetaKey, manifestDebugMetaKey} {
		if first[key] == "" {
			t.Fatalf("first metadata %q is empty: %#v", key, first)
		}
		if first[key] != second[key] {
			t.Fatalf("metadata %q differs: %q vs %q", key, first[key], second[key])
		}
	}
}

func openIndexDB(t *testing.T, repo string) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(SearchIndexPath(repo))
	if err != nil {
		t.Fatalf("open index db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertStatus(t *testing.T, status Status, want StatusState) {
	t.Helper()
	if status.State != want {
		t.Fatalf("status state = %q, want %q: %#v", status.State, want, status)
	}
}

func TestCheckStatusRejectsSymlinkIndexPath(t *testing.T) {
	repo := newBrainRepo(t)
	writeFile(t, repo, "sources/raw.md", "# Raw\n\nRaw body.\n")
	writeFile(t, repo, "wiki/topic.md", wikiContent(t, "doc_44444444444444444444444444444444", "Topic", "Topic summary.", "topic"))
	target := filepath.Join(t.TempDir(), "outside.sqlite")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, target, SearchIndexPath(repo))

	status, err := CheckStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("check symlink index status: %v", err)
	}
	assertStatus(t, status, StatusIncompatible)
	if !strings.Contains(status.Reason, "symlink") {
		t.Fatalf("status reason = %q, want symlink", status.Reason)
	}
}

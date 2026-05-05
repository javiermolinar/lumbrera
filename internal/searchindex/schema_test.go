package searchindex

import (
	"context"
	"database/sql"
	"testing"
)

func TestCreateSchemaCreatesTablesIndexesAndVersion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := CreateSchema(ctx, db); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := CreateSchema(ctx, db); err != nil {
		t.Fatalf("create schema should be idempotent: %v", err)
	}

	version, exists, err := ReadSchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if !exists {
		t.Fatal("schema version does not exist")
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	assertObjectsExist(t, db, map[string]string{
		"meta":                          "table",
		"documents":                     "table",
		"sections":                      "table",
		"sections_fts":                  "table",
		"idx_documents_kind_path":       "index",
		"idx_sections_document_ordinal": "index",
		"idx_sections_kind_path":        "index",
		"idx_sections_path_ordinal":     "index",
	})
}

func TestReadSchemaVersionMissing(t *testing.T) {
	db := openTestDB(t)

	version, exists, err := ReadSchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("read missing schema version: %v", err)
	}
	if exists {
		t.Fatalf("schema version exists = true, want false with version %d", version)
	}
}

func TestExternalContentFTSRequiresExplicitRebuild(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := CreateSchema(ctx, db); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	insertSearchFixture(t, db)

	if got := countFTSMatches(t, db, "uniquefterm"); got != 0 {
		t.Fatalf("FTS matches before explicit rebuild = %d, want 0", got)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sections_fts(sections_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("rebuild FTS table: %v", err)
	}
	if got := countFTSMatches(t, db, "uniquefterm"); got != 1 {
		t.Fatalf("FTS matches after explicit rebuild = %d, want 1", got)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertObjectsExist(t *testing.T, db *sql.DB, expected map[string]string) {
	t.Helper()

	rows, err := db.Query(`SELECT name, type FROM sqlite_master WHERE name IN (`+placeholders(len(expected))+`)`, keys(expected)...)
	if err != nil {
		t.Fatalf("query sqlite schema objects: %v", err)
	}
	defer rows.Close()

	actual := make(map[string]string)
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			t.Fatalf("scan schema object: %v", err)
		}
		actual[name] = typ
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema objects: %v", err)
	}

	for name, typ := range expected {
		if actual[name] != typ {
			t.Fatalf("schema object %q type = %q, want %q; all objects: %#v", name, actual[name], typ, actual)
		}
	}
}

func insertSearchFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`INSERT INTO documents(
		id, path, kind, title, summary, tags_json, sources_json, links_json,
		tags_text, sources_text, links_text, hash, size_bytes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"doc_fixture", "wiki/fixture.md", "wiki", "Fixture", "Summary", "[]", "[]", "[]",
		"", "", "", "hash", 100,
	); err != nil {
		t.Fatalf("insert document fixture: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO sections(
		rowid, id, document_id, ordinal, path, kind, title, summary, tags_json, sources_json,
		links_json, tags_text, sources_text, links_text, heading, anchor, level, body
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		1, "doc_fixture#section-0001", "doc_fixture", 1, "wiki/fixture.md", "wiki", "Fixture", "Summary",
		"[]", "[]", "[]", "", "", "", "Heading", "heading", 1, "the uniquefterm appears in section body",
	); err != nil {
		t.Fatalf("insert section fixture: %v", err)
	}
}

func countFTSMatches(t *testing.T, db *sql.DB, term string) int {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sections_fts WHERE sections_fts MATCH ?`, term).Scan(&count); err != nil {
		t.Fatalf("count FTS matches for %q: %v", term, err)
	}
	return count
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := "?"
	for i := 1; i < n; i++ {
		out += ", ?"
	}
	return out
}

func keys(m map[string]string) []any {
	out := make([]any, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

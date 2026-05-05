package searchindex

import (
	"database/sql"
	"testing"
)

func TestProbeFTS5Available(t *testing.T) {
	db, err := sql.Open(sqliteDriverName, ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	defer db.Close()

	if err := ProbeFTS5(db); err != nil {
		t.Fatalf("expected FTS5 to be available: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_temp_master WHERE name = 'lumbrera_fts5_probe'`).Scan(&count); err != nil {
		t.Fatalf("query temp schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("probe left temporary tables behind: got %d", count)
	}
}

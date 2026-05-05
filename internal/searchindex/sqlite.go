package searchindex

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

// OpenSQLite opens a SQLite database using the project-selected driver and
// verifies that FTS5 virtual tables are available.
func OpenSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := ProbeFTS5(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// ProbeFTS5 returns an explicit error if the linked SQLite build does not
// support FTS5. The probe uses a temporary virtual table so it does not mutate
// the persistent search index schema.
func ProbeFTS5(db *sql.DB) error {
	if db == nil {
		return errors.New("sqlite FTS5 unavailable: nil database")
	}

	if _, err := db.Exec(`CREATE VIRTUAL TABLE temp.lumbrera_fts5_probe USING fts5(body)`); err != nil {
		return fmt.Errorf("sqlite FTS5 unavailable: %w", err)
	}
	if _, err := db.Exec(`DROP TABLE temp.lumbrera_fts5_probe`); err != nil {
		return fmt.Errorf("drop sqlite FTS5 probe table: %w", err)
	}
	return nil
}

package searchindex

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

// OpenSQLite opens a SQLite database using the project-selected driver.
func OpenSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

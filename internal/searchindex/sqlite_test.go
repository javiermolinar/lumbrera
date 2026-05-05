package searchindex

import "testing"

func TestOpenSQLite(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE smoke_test (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("execute sqlite smoke statement: %v", err)
	}
}

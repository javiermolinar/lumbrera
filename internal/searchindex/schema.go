package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const CurrentSchemaVersion = 1

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS meta(
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS documents(
		id TEXT PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		kind TEXT NOT NULL CHECK(kind IN ('wiki', 'source')),
		title TEXT NOT NULL,
		summary TEXT NOT NULL,
		tags_json TEXT NOT NULL,
		sources_json TEXT NOT NULL,
		links_json TEXT NOT NULL,
		tags_text TEXT NOT NULL,
		sources_text TEXT NOT NULL,
		links_text TEXT NOT NULL,
		hash TEXT NOT NULL,
		size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0)
	)`,
	`CREATE TABLE IF NOT EXISTS sections(
		rowid INTEGER PRIMARY KEY,
		id TEXT UNIQUE NOT NULL,
		document_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL CHECK(ordinal >= 1),
		path TEXT NOT NULL,
		kind TEXT NOT NULL CHECK(kind IN ('wiki', 'source')),
		title TEXT NOT NULL,
		summary TEXT NOT NULL,
		tags_json TEXT NOT NULL,
		sources_json TEXT NOT NULL,
		links_json TEXT NOT NULL,
		tags_text TEXT NOT NULL,
		sources_text TEXT NOT NULL,
		links_text TEXT NOT NULL,
		heading TEXT,
		anchor TEXT,
		level INTEGER CHECK(level IS NULL OR level >= 1),
		body TEXT NOT NULL,
		FOREIGN KEY(document_id) REFERENCES documents(id) ON DELETE CASCADE,
		UNIQUE(document_id, ordinal)
	)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS sections_fts USING fts5(
		title,
		path,
		summary,
		tags_text,
		sources_text,
		links_text,
		heading,
		body,
		content='sections',
		content_rowid='rowid'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_documents_kind_path ON documents(kind, path)`,
	`CREATE INDEX IF NOT EXISTS idx_sections_document_ordinal ON sections(document_id, ordinal)`,
	`CREATE INDEX IF NOT EXISTS idx_sections_kind_path ON sections(kind, path)`,
	`CREATE INDEX IF NOT EXISTS idx_sections_path_ordinal ON sections(path, ordinal)`,
}

// CreateSchema initializes the current search index schema. It is idempotent
// for databases that already contain the current schema.
func CreateSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("create search index schema: nil database")
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin search index schema transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, stmt := range schemaStatements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create search index schema: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO meta(key, value) VALUES('schema_version', ?)`, strconv.Itoa(CurrentSchemaVersion)); err != nil {
		return fmt.Errorf("write search index schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit search index schema transaction: %w", err)
	}
	committed = true
	return nil
}

// ReadSchemaVersion returns the recorded schema version. exists is false when
// the database has not been initialized with the search index meta table.
func ReadSchemaVersion(ctx context.Context, db *sql.DB) (version int, exists bool, err error) {
	if db == nil {
		return 0, false, errors.New("read search index schema version: nil database")
	}

	var value string
	err = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) || isMissingMetaTable(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read search index schema version: %w", err)
	}

	version, err = strconv.Atoi(value)
	if err != nil {
		return 0, true, fmt.Errorf("invalid search index schema version %q: %w", value, err)
	}
	return version, true, nil
}

func isMissingMetaTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table: meta")
}

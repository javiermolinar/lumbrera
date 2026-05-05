package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	KindWiki   = "wiki"
	KindSource = "source"
)

// Document is the normalized file-level projection inserted into the derived
// search index.
type Document struct {
	ID          string
	Path        string
	Kind        string
	Title       string
	Summary     string
	TagsJSON    string
	SourcesJSON string
	LinksJSON   string
	TagsText    string
	SourcesText string
	LinksText   string
	Hash        string
	SizeBytes   int64
}

// Section is the normalized section-level projection inserted into the derived
// search index. Denormalized document fields and stable section IDs are derived
// during rebuild.
type Section struct {
	DocumentID string
	Ordinal    int
	Heading    string
	Anchor     string
	Level      int
	Body       string
}

// RebuildRecords replaces all derived search index rows with the supplied
// normalized records. Input order does not affect database row order or section
// rowids.
func RebuildRecords(ctx context.Context, db *sql.DB, documents []Document, sections []Section, metadata map[string]string) error {
	if db == nil {
		return errors.New("rebuild search index records: nil database")
	}

	docs, byID, err := normalizeDocuments(documents)
	if err != nil {
		return err
	}
	secs, err := normalizeSections(sections, byID)
	if err != nil {
		return err
	}
	metaKeys := sortedKeys(metadata)

	if err := CreateSchema(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin search index rebuild transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	statements := []string{
		`DELETE FROM sections`,
		`DELETE FROM documents`,
		`DELETE FROM meta WHERE key <> 'schema_version'`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("clear search index rows: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version', ?)`, fmt.Sprint(CurrentSchemaVersion)); err != nil {
		return fmt.Errorf("write search index schema version: %w", err)
	}
	for _, key := range metaKeys {
		if key == "schema_version" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES(?, ?)`, key, metadata[key]); err != nil {
			return fmt.Errorf("write search index metadata %q: %w", key, err)
		}
	}

	for _, doc := range docs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents(
			id, path, kind, title, summary, tags_json, sources_json, links_json,
			tags_text, sources_text, links_text, hash, size_bytes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			doc.ID, doc.Path, doc.Kind, doc.Title, doc.Summary, doc.TagsJSON, doc.SourcesJSON, doc.LinksJSON,
			doc.TagsText, doc.SourcesText, doc.LinksText, doc.Hash, doc.SizeBytes,
		); err != nil {
			return fmt.Errorf("insert search index document %q: %w", doc.ID, err)
		}
	}

	for i, sec := range secs {
		doc := byID[sec.DocumentID]
		rowid := int64(i + 1)
		sectionID := SectionID(sec.DocumentID, sec.Ordinal)
		if _, err := tx.ExecContext(ctx, `INSERT INTO sections(
			rowid, id, document_id, ordinal, path, kind, title, summary, tags_json, sources_json,
			links_json, tags_text, sources_text, links_text, heading, anchor, level, body
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rowid, sectionID, sec.DocumentID, sec.Ordinal, doc.Path, doc.Kind, doc.Title, doc.Summary,
			doc.TagsJSON, doc.SourcesJSON, doc.LinksJSON, doc.TagsText, doc.SourcesText, doc.LinksText,
			nullString(sec.Heading), nullString(sec.Anchor), nullInt(sec.Level), sec.Body,
		); err != nil {
			return fmt.Errorf("insert search index section %q: %w", sectionID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO sections_fts(sections_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild search index FTS table: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit search index rebuild transaction: %w", err)
	}
	committed = true
	return nil
}

func SectionID(documentID string, ordinal int) string {
	return fmt.Sprintf("%s#section-%04d", documentID, ordinal)
}

func normalizeDocuments(input []Document) ([]Document, map[string]Document, error) {
	docs := append([]Document(nil), input...)
	for i := range docs {
		var err error
		docs[i].TagsJSON, err = normalizeJSONArray(docs[i].TagsJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize tags_json for document %q: %w", docs[i].ID, err)
		}
		docs[i].SourcesJSON, err = normalizeJSONArray(docs[i].SourcesJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize sources_json for document %q: %w", docs[i].ID, err)
		}
		docs[i].LinksJSON, err = normalizeJSONArray(docs[i].LinksJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize links_json for document %q: %w", docs[i].ID, err)
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path != docs[j].Path {
			return docs[i].Path < docs[j].Path
		}
		return docs[i].ID < docs[j].ID
	})

	byID := make(map[string]Document, len(docs))
	byPath := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		if err := validateDocument(doc); err != nil {
			return nil, nil, err
		}
		if _, exists := byID[doc.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate search index document id %q", doc.ID)
		}
		if _, exists := byPath[doc.Path]; exists {
			return nil, nil, fmt.Errorf("duplicate search index document path %q", doc.Path)
		}
		byID[doc.ID] = doc
		byPath[doc.Path] = struct{}{}
	}
	return docs, byID, nil
}

func validateDocument(doc Document) error {
	if doc.ID == "" {
		return errors.New("search index document id is required")
	}
	if doc.Path == "" {
		return fmt.Errorf("search index document %q path is required", doc.ID)
	}
	if doc.Kind != KindWiki && doc.Kind != KindSource {
		return fmt.Errorf("search index document %q has invalid kind %q", doc.ID, doc.Kind)
	}
	if doc.Title == "" {
		return fmt.Errorf("search index document %q title is required", doc.ID)
	}
	if doc.Hash == "" {
		return fmt.Errorf("search index document %q hash is required", doc.ID)
	}
	if doc.SizeBytes < 0 {
		return fmt.Errorf("search index document %q has negative size_bytes %d", doc.ID, doc.SizeBytes)
	}
	return nil
}

func normalizeSections(input []Section, docs map[string]Document) ([]Section, error) {
	secs := append([]Section(nil), input...)
	sort.Slice(secs, func(i, j int) bool {
		leftDoc := docs[secs[i].DocumentID]
		rightDoc := docs[secs[j].DocumentID]
		if leftDoc.Path != rightDoc.Path {
			return leftDoc.Path < rightDoc.Path
		}
		if secs[i].Ordinal != secs[j].Ordinal {
			return secs[i].Ordinal < secs[j].Ordinal
		}
		return SectionID(secs[i].DocumentID, secs[i].Ordinal) < SectionID(secs[j].DocumentID, secs[j].Ordinal)
	})

	seen := make(map[string]struct{}, len(secs))
	for _, sec := range secs {
		if err := validateSection(sec, docs); err != nil {
			return nil, err
		}
		key := SectionID(sec.DocumentID, sec.Ordinal)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate search index section %q", key)
		}
		seen[key] = struct{}{}
	}
	return secs, nil
}

func validateSection(sec Section, docs map[string]Document) error {
	if sec.DocumentID == "" {
		return errors.New("search index section document_id is required")
	}
	if _, exists := docs[sec.DocumentID]; !exists {
		return fmt.Errorf("search index section references unknown document %q", sec.DocumentID)
	}
	if sec.Ordinal < 1 {
		return fmt.Errorf("search index section for document %q has invalid ordinal %d", sec.DocumentID, sec.Ordinal)
	}
	return nil
}

func normalizeJSONArray(value string) (string, error) {
	if value == "" {
		return "[]", nil
	}

	var raw []any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return "", err
	}

	items := make([]string, 0, len(raw))
	for i, item := range raw {
		text, ok := item.(string)
		if !ok {
			return "", fmt.Errorf("array item %d is %T, want string", i, item)
		}
		items = append(items, text)
	}
	items = uniqueSortedStrings(items)

	encoded, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func uniqueSortedStrings(input []string) []string {
	items := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return []string{}
	}

	sort.Strings(items)
	out := items[:0]
	var previous string
	for i, item := range items {
		if i == 0 || item != previous {
			out = append(out, item)
			previous = item
		}
	}
	return out
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

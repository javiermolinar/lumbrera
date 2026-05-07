package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func RebuildRecords(ctx context.Context, db *sql.DB, documents []Document, sections []Section, metadata map[string]string) error {
	links, citations, tags, err := relationshipFactsFromDocuments(documents)
	if err != nil {
		return err
	}
	return RebuildRecordsWithFacts(ctx, db, documents, sections, links, citations, tags, metadata)
}

// RebuildRecordsWithFacts replaces all derived search index rows with supplied
// normalized records plus relationship fact rows. Input order does not affect
// database row order or rowids.
func RebuildRecordsWithFacts(ctx context.Context, db *sql.DB, documents []Document, sections []Section, links []DocumentLink, citations []DocumentCitation, tags []DocumentTag, metadata map[string]string) error {
	if db == nil {
		return errors.New("rebuild search index records: nil database")
	}

	docs, byID, byPath, err := normalizeDocuments(documents)
	if err != nil {
		return err
	}
	secs, err := normalizeSections(sections, byID)
	if err != nil {
		return err
	}
	linkRows, err := normalizeDocumentLinks(links, byID, byPath)
	if err != nil {
		return err
	}
	citationRows, err := normalizeDocumentCitations(citations, byID)
	if err != nil {
		return err
	}
	tagRows, err := normalizeDocumentTags(tags, byID)
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
		`DELETE FROM document_links`,
		`DELETE FROM document_citations`,
		`DELETE FROM document_tags`,
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
		tier := doc.Tier
		if tier == "" {
			tier = TierCanonical
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents(
			id, path, kind, tier, title, summary, tags_json, sources_json, links_json,
			tags_text, sources_text, links_text, modified_date, hash, size_bytes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			doc.ID, doc.Path, doc.Kind, tier, doc.Title, doc.Summary, doc.TagsJSON, doc.SourcesJSON, doc.LinksJSON,
			doc.TagsText, doc.SourcesText, doc.LinksText, doc.ModifiedDate, doc.Hash, doc.SizeBytes,
		); err != nil {
			return fmt.Errorf("insert search index document %q: %w", doc.ID, err)
		}
	}

	for i, link := range linkRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_links(
			rowid, from_document_id, from_path, from_anchor, to_path, to_anchor,
			to_document_id, link_text, source_section_id, kind
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			i+1, link.FromDocumentID, link.FromPath, link.FromAnchor, link.ToPath, link.ToAnchor,
			link.ToDocumentID, link.LinkText, link.SourceSectionID, link.Kind,
		); err != nil {
			return fmt.Errorf("insert search index document link %s -> %s: %w", link.FromPath, link.ToPath, err)
		}
	}

	for i, citation := range citationRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_citations(
			rowid, document_id, wiki_path, source_path, source_anchor,
			citation_text, section_id, citation_kind
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			i+1, citation.DocumentID, citation.WikiPath, citation.SourcePath, citation.SourceAnchor,
			citation.CitationText, citation.SectionID, citation.CitationKind,
		); err != nil {
			return fmt.Errorf("insert search index citation %s -> %s: %w", citation.WikiPath, citation.SourcePath, err)
		}
	}

	for i, tag := range tagRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_tags(rowid, document_id, path, tag) VALUES (?, ?, ?, ?)`,
			i+1, tag.DocumentID, tag.Path, tag.Tag,
		); err != nil {
			return fmt.Errorf("insert search index tag %s:%s: %w", tag.Path, tag.Tag, err)
		}
	}

	for i, sec := range secs {
		doc := byID[sec.DocumentID]
		rowid := int64(i + 1)
		sectionID := SectionID(sec.DocumentID, sec.Ordinal)
		if _, err := tx.ExecContext(ctx, `INSERT INTO sections(
			rowid, id, document_id, ordinal, path, kind, title, summary, tags_json, sources_json,
			links_json, tags_text, sources_text, links_text, modified_date, heading, anchor, level, body
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rowid, sectionID, sec.DocumentID, sec.Ordinal, doc.Path, doc.Kind, doc.Title, doc.Summary,
			doc.TagsJSON, doc.SourcesJSON, doc.LinksJSON, doc.TagsText, doc.SourcesText, doc.LinksText, doc.ModifiedDate,
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

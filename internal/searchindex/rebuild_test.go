package searchindex

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

func TestRebuildDeterministicAcrossInputOrderAndDatabases(t *testing.T) {
	ctx := context.Background()
	docs, sections := deterministicFixture()

	db1 := openTestDB(t)
	if err := RebuildRecords(ctx, db1, reversedDocuments(docs), reversedSections(sections), map[string]string{
		"manifest_hash":       "manifest-a",
		"indexed_paths_hash":  "paths-a",
		"schema_version":      "ignored",
		"parse_rules_version": "rules-a",
	}); err != nil {
		t.Fatalf("rebuild db1 from reversed input: %v", err)
	}

	db2 := openTestDB(t)
	if err := RebuildRecords(ctx, db2, docs, sections, map[string]string{
		"parse_rules_version": "rules-a",
		"indexed_paths_hash":  "paths-a",
		"manifest_hash":       "manifest-a",
	}); err != nil {
		t.Fatalf("rebuild db2 from sorted input: %v", err)
	}

	assertSameDump(t, dumpDocuments(t, db1), dumpDocuments(t, db2), "documents")
	assertSameDump(t, dumpSections(t, db1), dumpSections(t, db2), "sections")
	assertSameDump(t, dumpMeta(t, db1), dumpMeta(t, db2), "meta")
	assertSameDump(t, querySectionIDs(t, db1, "tempo"), querySectionIDs(t, db2, "tempo"), "query order")
}

func TestRebuildReplacesExistingRowsAndFTSIndex(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	docs, sections := deterministicFixture()

	if err := RebuildRecords(ctx, db, docs, sections, nil); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	if got := countFTSMatches(t, db, "objectstoreunique"); got != 1 {
		t.Fatalf("objectstoreunique matches after initial rebuild = %d, want 1", got)
	}

	nextDocs := []Document{{
		ID:          "doc_only",
		Path:        "wiki/only.md",
		Kind:        KindWiki,
		Title:       "Only page",
		Summary:     "Replacement summary",
		TagsJSON:    `["replacement"]`,
		SourcesJSON: `[]`,
		LinksJSON:   `[]`,
		TagsText:    "replacement",
		Hash:        "hash-only",
		SizeBytes:   42,
	}}
	nextSections := []Section{{
		DocumentID: "doc_only",
		Ordinal:    1,
		Heading:    "Replacement",
		Anchor:     "replacement",
		Level:      1,
		Body:       "new material about compaction",
	}}
	if err := RebuildRecords(ctx, db, nextDocs, nextSections, map[string]string{"manifest_hash": "manifest-b"}); err != nil {
		t.Fatalf("replacement rebuild: %v", err)
	}

	if got := countRows(t, db, "documents"); got != 1 {
		t.Fatalf("documents rows after replacement = %d, want 1", got)
	}
	if got := countRows(t, db, "sections"); got != 1 {
		t.Fatalf("sections rows after replacement = %d, want 1", got)
	}
	if got := countFTSMatches(t, db, "objectstoreunique"); got != 0 {
		t.Fatalf("old FTS matches after replacement = %d, want 0", got)
	}
	if got := countFTSMatches(t, db, "compaction"); got != 1 {
		t.Fatalf("new FTS matches after replacement = %d, want 1", got)
	}

	sectionsDump := dumpSections(t, db)
	if len(sectionsDump) != 1 || sectionsDump[0][0] != "1" || sectionsDump[0][1] != "doc_only#section-0001" {
		t.Fatalf("replacement section rowid/id = %#v, want rowid 1 doc_only#section-0001", sectionsDump)
	}
}

func TestRebuildDefaultsJSONArrays(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	docs := []Document{{
		ID:        "doc_defaults",
		Path:      "sources/defaults.md",
		Kind:      KindSource,
		Title:     "Defaults",
		Hash:      "hash-defaults",
		SizeBytes: 7,
	}}
	sections := []Section{{DocumentID: "doc_defaults", Ordinal: 1, Body: "body"}}
	if err := RebuildRecords(ctx, db, docs, sections, nil); err != nil {
		t.Fatalf("rebuild defaults fixture: %v", err)
	}

	var tagsJSON, sourcesJSON, linksJSON string
	if err := db.QueryRow(`SELECT tags_json, sources_json, links_json FROM documents WHERE id = 'doc_defaults'`).Scan(&tagsJSON, &sourcesJSON, &linksJSON); err != nil {
		t.Fatalf("read default json fields: %v", err)
	}
	if tagsJSON != "[]" || sourcesJSON != "[]" || linksJSON != "[]" {
		t.Fatalf("default json fields = %q %q %q, want [] [] []", tagsJSON, sourcesJSON, linksJSON)
	}
}

func TestRebuildNormalizesJSONArrays(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	docs := []Document{{
		ID:          "doc_json",
		Path:        "wiki/json.md",
		Kind:        KindWiki,
		Title:       "JSON",
		TagsJSON:    `["zeta", "alpha", "zeta"]`,
		SourcesJSON: `["sources/b.md", "sources/a.md", "sources/a.md"]`,
		LinksJSON:   `["wiki/b.md", "wiki/a.md"]`,
		Hash:        "hash-json",
		SizeBytes:   12,
	}}
	sections := []Section{{DocumentID: "doc_json", Ordinal: 1, Body: "body"}}
	if err := RebuildRecords(ctx, db, docs, sections, nil); err != nil {
		t.Fatalf("rebuild json normalization fixture: %v", err)
	}

	var tagsJSON, sourcesJSON, linksJSON string
	if err := db.QueryRow(`SELECT tags_json, sources_json, links_json FROM documents WHERE id = 'doc_json'`).Scan(&tagsJSON, &sourcesJSON, &linksJSON); err != nil {
		t.Fatalf("read normalized json fields: %v", err)
	}
	if tagsJSON != `["alpha","zeta"]` || sourcesJSON != `["sources/a.md","sources/b.md"]` || linksJSON != `["wiki/a.md","wiki/b.md"]` {
		t.Fatalf("normalized json fields = %q %q %q", tagsJSON, sourcesJSON, linksJSON)
	}
}

func TestRebuildRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	docs, sections := deterministicFixture()

	tests := []struct {
		name     string
		docs     []Document
		sections []Section
	}{
		{name: "missing document id", docs: []Document{{Path: "wiki/a.md", Kind: KindWiki, Title: "A", Hash: "h"}}, sections: nil},
		{name: "invalid kind", docs: []Document{{ID: "doc", Path: "wiki/a.md", Kind: "note", Title: "A", Hash: "h"}}, sections: nil},
		{name: "invalid tags json", docs: []Document{{ID: "doc", Path: "wiki/a.md", Kind: KindWiki, Title: "A", TagsJSON: `{"tag":"a"}`, Hash: "h"}}, sections: nil},
		{name: "invalid sources json", docs: []Document{{ID: "doc", Path: "wiki/a.md", Kind: KindWiki, Title: "A", SourcesJSON: `[1]`, Hash: "h"}}, sections: nil},
		{name: "duplicate document id", docs: []Document{
			{ID: "doc", Path: "wiki/a.md", Kind: KindWiki, Title: "A", Hash: "h"},
			{ID: "doc", Path: "wiki/b.md", Kind: KindWiki, Title: "B", Hash: "h"},
		}, sections: nil},
		{name: "unknown section document", docs: docs, sections: []Section{{DocumentID: "missing", Ordinal: 1, Body: "body"}}},
		{name: "invalid section ordinal", docs: docs, sections: []Section{{DocumentID: docs[0].ID, Ordinal: 0, Body: "body"}}},
		{name: "duplicate section", docs: docs, sections: []Section{
			{DocumentID: sections[0].DocumentID, Ordinal: sections[0].Ordinal, Body: "one"},
			{DocumentID: sections[0].DocumentID, Ordinal: sections[0].Ordinal, Body: "two"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RebuildRecords(ctx, db, tt.docs, tt.sections, nil); err == nil {
				t.Fatal("RebuildRecords succeeded, want error")
			}
		})
	}
}

func deterministicFixture() ([]Document, []Section) {
	docs := []Document{
		{
			ID:          "doc_tempo",
			Path:        "wiki/tempo.md",
			Kind:        KindWiki,
			Title:       "Tempo limits",
			Summary:     "Tempo ingestion and retention limits",
			TagsJSON:    `["tempo","limits"]`,
			SourcesJSON: `["sources/tempo-notes.md"]`,
			LinksJSON:   `["wiki/mimir.md"]`,
			TagsText:    "limits tempo",
			SourcesText: "sources/tempo-notes.md",
			LinksText:   "wiki/mimir.md",
			Hash:        "hash-tempo",
			SizeBytes:   100,
		},
		{
			ID:          "doc_source_tempo",
			Path:        "sources/tempo-notes.md",
			Kind:        KindSource,
			Title:       "Tempo notes",
			Summary:     "",
			TagsJSON:    `[]`,
			SourcesJSON: `[]`,
			LinksJSON:   `[]`,
			Hash:        "hash-source-tempo",
			SizeBytes:   80,
		},
		{
			ID:          "doc_mimir",
			Path:        "wiki/mimir.md",
			Kind:        KindWiki,
			Title:       "Mimir limits",
			Summary:     "Mimir tenant limits",
			TagsJSON:    `["mimir","limits"]`,
			SourcesJSON: `[]`,
			LinksJSON:   `["wiki/tempo.md"]`,
			TagsText:    "limits mimir",
			LinksText:   "wiki/tempo.md",
			Hash:        "hash-mimir",
			SizeBytes:   90,
		},
	}
	sections := []Section{
		{DocumentID: "doc_tempo", Ordinal: 2, Heading: "Retention", Anchor: "retention", Level: 2, Body: "tempo retention defaults use objectstoreunique storage"},
		{DocumentID: "doc_source_tempo", Ordinal: 1, Heading: "Raw tempo notes", Anchor: "raw-tempo-notes", Level: 1, Body: "tempo source evidence mentions retention and compaction"},
		{DocumentID: "doc_tempo", Ordinal: 1, Heading: "Ingestion", Anchor: "ingestion", Level: 2, Body: "tempo ingestion limits protect distributors"},
		{DocumentID: "doc_mimir", Ordinal: 1, Heading: "Tenant limits", Anchor: "tenant-limits", Level: 2, Body: "mimir tenant limits are separate from tempo"},
	}
	return docs, sections
}

func reversedDocuments(input []Document) []Document {
	out := append([]Document(nil), input...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func reversedSections(input []Section) []Section {
	out := append([]Section(nil), input...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func dumpDocuments(t *testing.T, db *sql.DB) [][]string {
	t.Helper()
	return queryStrings(t, db, `SELECT id, path, kind, title, summary, tags_json, sources_json, links_json, tags_text, sources_text, links_text, hash, CAST(size_bytes AS TEXT) FROM documents ORDER BY path, id`, 13)
}

func dumpSections(t *testing.T, db *sql.DB) [][]string {
	t.Helper()
	return queryStrings(t, db, `SELECT CAST(rowid AS TEXT), id, document_id, CAST(ordinal AS TEXT), path, kind, title, summary, tags_json, sources_json, links_json, tags_text, sources_text, links_text, COALESCE(heading, ''), COALESCE(anchor, ''), COALESCE(CAST(level AS TEXT), ''), body FROM sections ORDER BY rowid`, 18)
}

func dumpMeta(t *testing.T, db *sql.DB) [][]string {
	t.Helper()
	return queryStrings(t, db, `SELECT key, value FROM meta ORDER BY key`, 2)
}

func querySectionIDs(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT s.id
		FROM sections_fts
		JOIN sections s ON s.rowid = sections_fts.rowid
		WHERE sections_fts MATCH ?
		ORDER BY bm25(sections_fts, 5.0, 3.0, 4.0, 2.0, 2.0, 1.5, 3.0, 1.0) ASC,
			CASE s.kind WHEN 'wiki' THEN 0 ELSE 1 END,
			s.path ASC,
			s.ordinal ASC,
			s.id ASC`, query)
	if err != nil {
		t.Fatalf("query section ids: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan section id: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate section ids: %v", err)
	}
	return out
}

func queryStrings(t *testing.T, db *sql.DB, query string, columns int, args ...any) [][]string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("query strings: %v", err)
	}
	defer rows.Close()

	var out [][]string
	for rows.Next() {
		values := make([]sql.NullString, columns)
		scan := make([]any, columns)
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rows.Scan(scan...); err != nil {
			t.Fatalf("scan strings: %v", err)
		}
		row := make([]string, columns)
		for i, value := range values {
			if value.Valid {
				row[i] = value.String
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate strings: %v", err)
	}
	return out
}

func assertSameDump(t *testing.T, got, want any, name string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s differ\ngot:  %#v\nwant: %#v", name, got, want)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count rows in %s: %v", table, err)
	}
	return count
}

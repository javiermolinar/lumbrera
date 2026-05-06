package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	KindWiki   = "wiki"
	KindSource = "source"
)

// Document is the normalized file-level projection inserted into the derived
// search index.
type Document struct {
	ID           string
	Path         string
	Kind         string
	Title        string
	Summary      string
	TagsJSON     string
	SourcesJSON  string
	LinksJSON    string
	TagsText     string
	SourcesText  string
	LinksText    string
	ModifiedDate string
	Hash         string
	SizeBytes    int64
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

// DocumentLink is the normalized link graph fact inserted into the derived
// search index.
type DocumentLink struct {
	FromDocumentID  string
	FromPath        string
	FromAnchor      string
	ToPath          string
	ToAnchor        string
	ToDocumentID    string
	LinkText        string
	SourceSectionID string
	Kind            string
}

// DocumentCitation is the normalized source-citation fact inserted into the
// derived search index.
type DocumentCitation struct {
	DocumentID   string
	WikiPath     string
	SourcePath   string
	SourceAnchor string
	CitationText string
	SectionID    string
	CitationKind string
}

// DocumentTag is the normalized tag fact inserted into the derived search index.
type DocumentTag struct {
	DocumentID string
	Path       string
	Tag        string
}

// RebuildRecords replaces all derived search index rows with the supplied
// normalized records. Input order does not affect database row order or section
// rowids. Relationship fact rows are derived from document metadata.
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents(
			id, path, kind, title, summary, tags_json, sources_json, links_json,
			tags_text, sources_text, links_text, modified_date, hash, size_bytes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			doc.ID, doc.Path, doc.Kind, doc.Title, doc.Summary, doc.TagsJSON, doc.SourcesJSON, doc.LinksJSON,
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

func SectionID(documentID string, ordinal int) string {
	return fmt.Sprintf("%s#section-%04d", documentID, ordinal)
}

func normalizeDocuments(input []Document) ([]Document, map[string]Document, map[string]Document, error) {
	docs := append([]Document(nil), input...)
	for i := range docs {
		var err error
		docs[i].TagsJSON, err = normalizeJSONArray(docs[i].TagsJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("normalize tags_json for document %q: %w", docs[i].ID, err)
		}
		docs[i].SourcesJSON, err = normalizeJSONArray(docs[i].SourcesJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("normalize sources_json for document %q: %w", docs[i].ID, err)
		}
		docs[i].LinksJSON, err = normalizeJSONArray(docs[i].LinksJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("normalize links_json for document %q: %w", docs[i].ID, err)
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path != docs[j].Path {
			return docs[i].Path < docs[j].Path
		}
		return docs[i].ID < docs[j].ID
	})

	byID := make(map[string]Document, len(docs))
	byPath := make(map[string]Document, len(docs))
	for _, doc := range docs {
		if err := validateDocument(doc); err != nil {
			return nil, nil, nil, err
		}
		if _, exists := byID[doc.ID]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate search index document id %q", doc.ID)
		}
		if _, exists := byPath[doc.Path]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate search index document path %q", doc.Path)
		}
		byID[doc.ID] = doc
		byPath[doc.Path] = doc
	}
	return docs, byID, byPath, nil
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
	if doc.Kind == KindWiki {
		if strings.TrimSpace(doc.ModifiedDate) == "" {
			return fmt.Errorf("search index wiki document %q modified_date is required", doc.ID)
		}
		if _, err := time.Parse(modifiedDateLayout, doc.ModifiedDate); err != nil {
			return fmt.Errorf("search index wiki document %q modified_date %q must use YYYY-MM-DD", doc.ID, doc.ModifiedDate)
		}
	}
	if doc.Kind == KindSource && doc.ModifiedDate != "" {
		return fmt.Errorf("search index source document %q modified_date must be empty", doc.ID)
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

func relationshipFactsFromDocuments(documents []Document) ([]DocumentLink, []DocumentCitation, []DocumentTag, error) {
	links := []DocumentLink{}
	citations := []DocumentCitation{}
	tags := []DocumentTag{}
	for _, doc := range documents {
		if doc.Kind != KindWiki {
			continue
		}
		docTags, err := decodeStringArray(doc.TagsJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode tags for document %q: %w", doc.ID, err)
		}
		for _, tag := range docTags {
			tags = append(tags, DocumentTag{DocumentID: doc.ID, Path: doc.Path, Tag: tag})
		}

		docSources, err := decodeStringArray(doc.SourcesJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode sources for document %q: %w", doc.ID, err)
		}
		for _, source := range docSources {
			citations = append(citations, DocumentCitation{
				DocumentID:   doc.ID,
				WikiPath:     doc.Path,
				SourcePath:   source,
				CitationText: source,
				CitationKind: "frontmatter_source",
			})
		}

		docLinks, err := decodeStringArray(doc.LinksJSON)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode links for document %q: %w", doc.ID, err)
		}
		for _, link := range docLinks {
			links = append(links, DocumentLink{
				FromDocumentID: doc.ID,
				FromPath:       doc.Path,
				ToPath:         link,
			})
		}
	}
	return links, citations, tags, nil
}

func normalizeDocumentLinks(input []DocumentLink, docsByID, docsByPath map[string]Document) ([]DocumentLink, error) {
	links := append([]DocumentLink(nil), input...)
	for i := range links {
		links[i].FromDocumentID = strings.TrimSpace(links[i].FromDocumentID)
		links[i].FromPath = strings.TrimSpace(links[i].FromPath)
		links[i].FromAnchor = strings.TrimSpace(links[i].FromAnchor)
		links[i].ToPath = strings.TrimSpace(links[i].ToPath)
		links[i].ToAnchor = strings.TrimSpace(links[i].ToAnchor)
		links[i].ToDocumentID = strings.TrimSpace(links[i].ToDocumentID)
		links[i].LinkText = strings.TrimSpace(links[i].LinkText)
		links[i].SourceSectionID = strings.TrimSpace(links[i].SourceSectionID)
		links[i].Kind = strings.TrimSpace(links[i].Kind)
		fromDoc, ok := docsByID[links[i].FromDocumentID]
		if !ok {
			return nil, fmt.Errorf("document link references unknown from_document_id %q", links[i].FromDocumentID)
		}
		if links[i].FromPath == "" {
			links[i].FromPath = fromDoc.Path
		}
		if links[i].FromPath != fromDoc.Path {
			return nil, fmt.Errorf("document link from_path %q does not match document %q path %q", links[i].FromPath, fromDoc.ID, fromDoc.Path)
		}
		if links[i].ToPath == "" {
			return nil, fmt.Errorf("document link from %s has empty to_path", links[i].FromPath)
		}
		if toDoc, ok := docsByPath[links[i].ToPath]; ok {
			if links[i].ToDocumentID == "" {
				links[i].ToDocumentID = toDoc.ID
			} else if links[i].ToDocumentID != toDoc.ID {
				return nil, fmt.Errorf("document link to_document_id %q does not match path %q document %q", links[i].ToDocumentID, links[i].ToPath, toDoc.ID)
			}
		}
		if links[i].Kind == "" {
			links[i].Kind = kindForLinkedPath(links[i].ToPath)
		}
		if links[i].Kind != KindWiki && links[i].Kind != KindSource && links[i].Kind != "external" {
			return nil, fmt.Errorf("document link from %s has invalid kind %q", links[i].FromPath, links[i].Kind)
		}
	}
	sort.Slice(links, func(i, j int) bool { return documentLinkLess(links[i], links[j]) })
	return uniqueDocumentLinks(links), nil
}

func normalizeDocumentCitations(input []DocumentCitation, docsByID map[string]Document) ([]DocumentCitation, error) {
	citations := append([]DocumentCitation(nil), input...)
	for i := range citations {
		citations[i].DocumentID = strings.TrimSpace(citations[i].DocumentID)
		citations[i].WikiPath = strings.TrimSpace(citations[i].WikiPath)
		citations[i].SourcePath = strings.TrimSpace(citations[i].SourcePath)
		citations[i].SourceAnchor = strings.TrimSpace(citations[i].SourceAnchor)
		citations[i].CitationText = strings.TrimSpace(citations[i].CitationText)
		citations[i].SectionID = strings.TrimSpace(citations[i].SectionID)
		citations[i].CitationKind = strings.TrimSpace(citations[i].CitationKind)
		doc, ok := docsByID[citations[i].DocumentID]
		if !ok {
			return nil, fmt.Errorf("document citation references unknown document_id %q", citations[i].DocumentID)
		}
		if doc.Kind != KindWiki {
			return nil, fmt.Errorf("document citation references non-wiki document %q", doc.ID)
		}
		if citations[i].WikiPath == "" {
			citations[i].WikiPath = doc.Path
		}
		if citations[i].WikiPath != doc.Path {
			return nil, fmt.Errorf("document citation wiki_path %q does not match document %q path %q", citations[i].WikiPath, doc.ID, doc.Path)
		}
		if !strings.HasPrefix(citations[i].SourcePath, "sources/") {
			return nil, fmt.Errorf("document citation for %s has invalid source_path %q", citations[i].WikiPath, citations[i].SourcePath)
		}
		if citations[i].CitationKind != "frontmatter_source" && citations[i].CitationKind != "inline_source" {
			return nil, fmt.Errorf("document citation for %s has invalid citation_kind %q", citations[i].WikiPath, citations[i].CitationKind)
		}
		if citations[i].CitationText == "" {
			citations[i].CitationText = citations[i].SourcePath
		}
	}
	sort.Slice(citations, func(i, j int) bool { return documentCitationLess(citations[i], citations[j]) })
	return uniqueDocumentCitations(citations), nil
}

func normalizeDocumentTags(input []DocumentTag, docsByID map[string]Document) ([]DocumentTag, error) {
	tags := append([]DocumentTag(nil), input...)
	for i := range tags {
		tags[i].DocumentID = strings.TrimSpace(tags[i].DocumentID)
		tags[i].Path = strings.TrimSpace(tags[i].Path)
		tags[i].Tag = strings.TrimSpace(tags[i].Tag)
		doc, ok := docsByID[tags[i].DocumentID]
		if !ok {
			return nil, fmt.Errorf("document tag references unknown document_id %q", tags[i].DocumentID)
		}
		if doc.Kind != KindWiki {
			return nil, fmt.Errorf("document tag references non-wiki document %q", doc.ID)
		}
		if tags[i].Path == "" {
			tags[i].Path = doc.Path
		}
		if tags[i].Path != doc.Path {
			return nil, fmt.Errorf("document tag path %q does not match document %q path %q", tags[i].Path, doc.ID, doc.Path)
		}
		if tags[i].Tag == "" {
			return nil, fmt.Errorf("document tag for %s is empty", tags[i].Path)
		}
	}
	sort.Slice(tags, func(i, j int) bool { return documentTagLess(tags[i], tags[j]) })
	return uniqueDocumentTags(tags), nil
}

func kindForLinkedPath(path string) string {
	switch {
	case strings.HasPrefix(path, "wiki/"):
		return KindWiki
	case strings.HasPrefix(path, "sources/"):
		return KindSource
	default:
		return "external"
	}
}

func documentLinkLess(left, right DocumentLink) bool {
	leftKey := []string{left.FromPath, left.FromAnchor, left.ToPath, left.ToAnchor, left.ToDocumentID, left.LinkText, left.SourceSectionID, left.Kind}
	rightKey := []string{right.FromPath, right.FromAnchor, right.ToPath, right.ToAnchor, right.ToDocumentID, right.LinkText, right.SourceSectionID, right.Kind}
	for i := range leftKey {
		if leftKey[i] != rightKey[i] {
			return leftKey[i] < rightKey[i]
		}
	}
	return false
}

func documentCitationLess(left, right DocumentCitation) bool {
	leftKey := []string{left.WikiPath, left.SourcePath, left.SourceAnchor, left.CitationKind, left.CitationText, left.SectionID, left.DocumentID}
	rightKey := []string{right.WikiPath, right.SourcePath, right.SourceAnchor, right.CitationKind, right.CitationText, right.SectionID, right.DocumentID}
	for i := range leftKey {
		if leftKey[i] != rightKey[i] {
			return leftKey[i] < rightKey[i]
		}
	}
	return false
}

func documentTagLess(left, right DocumentTag) bool {
	if left.Tag != right.Tag {
		return left.Tag < right.Tag
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	return left.DocumentID < right.DocumentID
}

func uniqueDocumentLinks(input []DocumentLink) []DocumentLink {
	out := input[:0]
	seen := map[string]struct{}{}
	for _, link := range input {
		key := strings.Join([]string{link.FromDocumentID, link.FromPath, link.FromAnchor, link.ToPath, link.ToAnchor, link.ToDocumentID, link.LinkText, link.SourceSectionID, link.Kind}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, link)
	}
	return out
}

func uniqueDocumentCitations(input []DocumentCitation) []DocumentCitation {
	out := input[:0]
	seen := map[string]struct{}{}
	for _, citation := range input {
		key := strings.Join([]string{citation.DocumentID, citation.WikiPath, citation.SourcePath, citation.SourceAnchor, citation.CitationText, citation.SectionID, citation.CitationKind}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, citation)
	}
	return out
}

func uniqueDocumentTags(input []DocumentTag) []DocumentTag {
	out := input[:0]
	seen := map[string]struct{}{}
	for _, tag := range input {
		key := tag.DocumentID + "\x00" + tag.Tag
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
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

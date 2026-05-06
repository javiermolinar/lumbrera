package searchindex

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

func loadCandidateDocuments(ctx context.Context, db *sql.DB) (map[string]*candidateDocument, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, path, kind, title, summary, modified_date FROM documents ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("read candidate documents: %w", err)
	}
	defer rows.Close()

	docs := map[string]*candidateDocument{}
	for rows.Next() {
		doc := &candidateDocument{
			Outgoing: map[string]struct{}{},
			Incoming: map[string]struct{}{},
		}
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.Kind, &doc.Title, &doc.Summary, &doc.ModifiedDate); err != nil {
			return nil, fmt.Errorf("scan candidate document: %w", err)
		}
		docs[doc.Path] = doc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate documents: %w", err)
	}
	return docs, nil
}

func loadCandidateFacts(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	if err := loadCandidateTags(ctx, db, docs); err != nil {
		return err
	}
	if err := loadCandidateCitations(ctx, db, docs); err != nil {
		return err
	}
	if err := loadCandidateLinks(ctx, db, docs); err != nil {
		return err
	}
	return nil
}

func loadCandidateTags(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT path, tag FROM document_tags ORDER BY path, tag`)
	if err != nil {
		return fmt.Errorf("read candidate document tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var docPath, tag string
		if err := rows.Scan(&docPath, &tag); err != nil {
			return fmt.Errorf("scan candidate document tag: %w", err)
		}
		if doc := docs[docPath]; doc != nil {
			doc.Tags = append(doc.Tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate document tags: %w", err)
	}
	for _, doc := range docs {
		doc.Tags = uniqueSortedStrings(doc.Tags)
	}
	return nil
}

func loadCandidateCitations(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT wiki_path, source_path FROM document_citations ORDER BY wiki_path, source_path`)
	if err != nil {
		return fmt.Errorf("read candidate citations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var wikiPath, sourcePath string
		if err := rows.Scan(&wikiPath, &sourcePath); err != nil {
			return fmt.Errorf("scan candidate citation: %w", err)
		}
		if doc := docs[wikiPath]; doc != nil {
			doc.Sources = append(doc.Sources, sourcePath)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate citations: %w", err)
	}
	for _, doc := range docs {
		doc.Sources = uniqueSortedStrings(doc.Sources)
	}
	return nil
}

func loadCandidateLinks(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT from_path, to_path FROM document_links WHERE kind = 'wiki' ORDER BY from_path, to_path`)
	if err != nil {
		return fmt.Errorf("read candidate wiki links: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fromPath, toPath string
		if err := rows.Scan(&fromPath, &toPath); err != nil {
			return fmt.Errorf("scan candidate wiki link: %w", err)
		}
		fromDoc := docs[fromPath]
		toDoc := docs[toPath]
		if fromDoc == nil || toDoc == nil || fromDoc.Kind != KindWiki || toDoc.Kind != KindWiki || fromPath == toPath {
			continue
		}
		fromDoc.Outgoing[toPath] = struct{}{}
		toDoc.Incoming[fromPath] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate wiki links: %w", err)
	}
	return nil
}

func buildCandidateTerms(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	builders := make(map[string]*strings.Builder, len(docs))
	byID := make(map[string]*candidateDocument, len(docs))
	for _, doc := range docs {
		var b strings.Builder
		writeCandidateTermText(&b, doc.Path)
		writeCandidateTermText(&b, doc.Title)
		writeCandidateTermText(&b, doc.Summary)
		writeCandidateTermText(&b, strings.Join(doc.Tags, " "))
		builders[doc.ID] = &b
		byID[doc.ID] = doc
	}

	rows, err := db.QueryContext(ctx, `SELECT document_id, COALESCE(heading, ''), body FROM sections ORDER BY document_id, ordinal`)
	if err != nil {
		return fmt.Errorf("read candidate section text: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var documentID, heading, body string
		if err := rows.Scan(&documentID, &heading, &body); err != nil {
			return fmt.Errorf("scan candidate section text: %w", err)
		}
		b := builders[documentID]
		if b == nil {
			continue
		}
		writeCandidateTermText(b, heading)
		writeCandidateTermText(b, body)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate section text: %w", err)
	}

	for id, b := range builders {
		byID[id].Terms = candidateTermCounts(b.String())
	}
	return nil
}

func writeCandidateTermText(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(text)
}

func candidateDocumentsByKind(docsByPath map[string]*candidateDocument, kind string) []*candidateDocument {
	docs := make([]*candidateDocument, 0, len(docsByPath))
	for _, doc := range docsByPath {
		if doc.Kind == kind {
			docs = append(docs, doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs
}

func candidateTermCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, term := range strings.Fields(normalizeFTSText(text)) {
		if isCandidateStopword(term) {
			continue
		}
		counts[term]++
	}
	return counts
}

func isCandidateStopword(term string) bool {
	if len(term) < 3 {
		return true
	}
	if searchStopwords[term] || recommendationStopwords[term] || candidateStopwords[term] {
		return true
	}
	return false
}

func candidateTermDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for term := range doc.Terms {
			out[term]++
		}
	}
	return out
}

func candidateTagDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for _, tag := range doc.Tags {
			out[tag]++
		}
	}
	return out
}

func candidateSourceDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for _, source := range doc.Sources {
			out[source]++
		}
	}
	return out
}

var candidateStopwords = map[string]bool{
	"anchor": true, "anchors": true, "available": true, "body": true, "combined": true,
	"compact": true, "doc": true, "docs": true, "file": true, "files": true,
	"generated": true, "generic": true, "index": true, "markdown": true, "may": true,
	"must": true, "page": true, "pages": true, "section": true, "sections": true,
	"source": true, "sources": true, "wiki": true,
}

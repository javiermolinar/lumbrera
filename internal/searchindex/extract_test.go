package searchindex

import (
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func TestExtractMarkdownRecordsWiki(t *testing.T) {
	id := "doc_0123456789abcdef0123456789abcdef"
	meta := frontmatter.NewWithID(
		id,
		KindWiki,
		"Tempo limits",
		"Tempo ingestion and retention limits.",
		[]string{"tempo", "limits"},
		[]string{"sources/raw-b.md", "sources/raw-a.md"},
		[]string{"wiki/related.md"},
	)
	meta.Lumbrera.ModifiedDate = "2026-05-06"
	content, err := frontmatter.Attach(meta, "# Tempo limits\n\nIntro mentions tempo.\n\n## Ingestion\n\nIngestion body.\n")
	if err != nil {
		t.Fatalf("attach frontmatter: %v", err)
	}

	doc, sections, err := ExtractMarkdownRecords("wiki/tempo.md", []byte(content))
	if err != nil {
		t.Fatalf("extract wiki records: %v", err)
	}

	if doc.ID != id || doc.Path != "wiki/tempo.md" || doc.Kind != KindWiki {
		t.Fatalf("unexpected wiki document identity: %#v", doc)
	}
	if doc.Title != "Tempo limits" || doc.Summary != "Tempo ingestion and retention limits." {
		t.Fatalf("unexpected wiki title/summary: %#v", doc)
	}
	if doc.TagsJSON != `["limits","tempo"]` || doc.TagsText != "limits tempo" {
		t.Fatalf("unexpected tags fields: json=%q text=%q", doc.TagsJSON, doc.TagsText)
	}
	if doc.SourcesJSON != `["sources/raw-a.md","sources/raw-b.md"]` || doc.SourcesText != "sources/raw-a.md sources/raw-b.md" {
		t.Fatalf("unexpected sources fields: json=%q text=%q", doc.SourcesJSON, doc.SourcesText)
	}
	if doc.LinksJSON != `["wiki/related.md"]` || doc.LinksText != "wiki/related.md" {
		t.Fatalf("unexpected links fields: json=%q text=%q", doc.LinksJSON, doc.LinksText)
	}
	if doc.ModifiedDate != "2026-05-06" {
		t.Fatalf("modified date = %q, want 2026-05-06", doc.ModifiedDate)
	}
	if doc.Hash != contentHash([]byte(content)) || doc.SizeBytes != int64(len(content)) {
		t.Fatalf("unexpected hash/size: %#v", doc)
	}

	wantSections := []Section{
		{DocumentID: id, Ordinal: 1, Heading: "Tempo limits", Anchor: "tempo-limits", Level: 1, Body: "Intro mentions tempo."},
		{DocumentID: id, Ordinal: 2, Heading: "Ingestion", Anchor: "ingestion", Level: 2, Body: "Ingestion body."},
	}
	assertSections(t, sections, wantSections)
}

func TestExtractMarkdownRecordsWithFactsWikiRelationships(t *testing.T) {
	id := "doc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	meta := frontmatter.NewWithID(
		id,
		KindWiki,
		"Related tempo",
		"Related tempo summary.",
		[]string{"tempo", "related"},
		[]string{"sources/raw.md"},
		[]string{"wiki/other.md"},
	)
	meta.Lumbrera.ModifiedDate = "2026-05-06"
	content, err := frontmatter.Attach(meta, "# Related tempo\n\nSee [Other](./other.md). Inline evidence [source: ../sources/raw.md#evidence].\n")
	if err != nil {
		t.Fatalf("attach frontmatter: %v", err)
	}

	_, _, links, citations, tags, err := ExtractMarkdownRecordsWithFacts("wiki/related-tempo.md", []byte(content))
	if err != nil {
		t.Fatalf("extract wiki records with facts: %v", err)
	}

	if len(links) != 1 || links[0].FromDocumentID != id || links[0].FromPath != "wiki/related-tempo.md" || links[0].ToPath != "wiki/other.md" || links[0].Kind != KindWiki {
		t.Fatalf("unexpected links: %#v", links)
	}
	wantCitations := []DocumentCitation{
		{DocumentID: id, WikiPath: "wiki/related-tempo.md", SourcePath: "sources/raw.md", CitationText: "sources/raw.md", CitationKind: "frontmatter_source"},
		{DocumentID: id, WikiPath: "wiki/related-tempo.md", SourcePath: "sources/raw.md", SourceAnchor: "evidence", CitationText: "sources/raw.md#evidence", CitationKind: "inline_source"},
	}
	if len(citations) != len(wantCitations) {
		t.Fatalf("citation count = %d, want %d: %#v", len(citations), len(wantCitations), citations)
	}
	for i := range wantCitations {
		if citations[i] != wantCitations[i] {
			t.Fatalf("citation[%d] = %#v, want %#v; all=%#v", i, citations[i], wantCitations[i], citations)
		}
	}
	wantTags := []DocumentTag{{DocumentID: id, Path: "wiki/related-tempo.md", Tag: "related"}, {DocumentID: id, Path: "wiki/related-tempo.md", Tag: "tempo"}}
	if len(tags) != len(wantTags) {
		t.Fatalf("tag count = %d, want %d: %#v", len(tags), len(wantTags), tags)
	}
	for i := range wantTags {
		if tags[i] != wantTags[i] {
			t.Fatalf("tag[%d] = %#v, want %#v; all=%#v", i, tags[i], wantTags[i], tags)
		}
	}
}

func TestExtractMarkdownRecordsSource(t *testing.T) {
	content := []byte("# Raw Source\n\nRaw body with an unresolved [local link](../missing.md).\n\n## Evidence\n\nEvidence body.\n")

	doc, sections, err := ExtractMarkdownRecords("sources/raw-source.md", content)
	if err != nil {
		t.Fatalf("extract source records: %v", err)
	}

	wantID := sourceDocumentID("sources/raw-source.md")
	if doc.ID != wantID || doc.Path != "sources/raw-source.md" || doc.Kind != KindSource {
		t.Fatalf("unexpected source document identity: %#v", doc)
	}
	if !strings.HasPrefix(doc.ID, "source_") {
		t.Fatalf("source id %q should use source_ prefix", doc.ID)
	}
	if doc.Title != "Raw Source" {
		t.Fatalf("source title = %q, want Raw Source", doc.Title)
	}
	if doc.Summary != "" || doc.TagsJSON != "[]" || doc.SourcesJSON != "[]" || doc.LinksJSON != "[]" {
		t.Fatalf("unexpected source metadata defaults: %#v", doc)
	}
	if doc.TagsText != "" || doc.SourcesText != "" || doc.LinksText != "" {
		t.Fatalf("unexpected source searchable metadata text: %#v", doc)
	}
	if doc.Hash != contentHash(content) || doc.SizeBytes != int64(len(content)) {
		t.Fatalf("unexpected hash/size: %#v", doc)
	}

	wantSections := []Section{
		{DocumentID: wantID, Ordinal: 1, Heading: "Raw Source", Anchor: "raw-source", Level: 1, Body: "Raw body with an unresolved [local link](../missing.md)."},
		{DocumentID: wantID, Ordinal: 2, Heading: "Evidence", Anchor: "evidence", Level: 2, Body: "Evidence body."},
	}
	assertSections(t, sections, wantSections)
}

func TestExtractMarkdownRecordsSourceTitleFallbacks(t *testing.T) {
	doc, _, err := ExtractMarkdownRecords("sources/heading-only.md", []byte("Intro.\n\n## Evidence Heading\n\nEvidence.\n"))
	if err != nil {
		t.Fatalf("extract source with first non-H1 heading: %v", err)
	}
	if doc.Title != "Evidence Heading" {
		t.Fatalf("source title = %q, want Evidence Heading", doc.Title)
	}

	doc, _, err = ExtractMarkdownRecords("sources/plain-text_source.md", []byte("Plain source body.\n"))
	if err != nil {
		t.Fatalf("extract source without headings: %v", err)
	}
	if doc.Title != "Plain Text Source" {
		t.Fatalf("source title = %q, want Plain Text Source", doc.Title)
	}
}

func TestExtractMarkdownRecordsRejectsInvalidWiki(t *testing.T) {
	if _, _, err := ExtractMarkdownRecords("wiki/missing.md", []byte("# Missing\n")); err == nil {
		t.Fatal("extract wiki without frontmatter succeeded, want error")
	}

	meta := frontmatter.NewWithID("doc_0123456789abcdef0123456789abcdef", KindSource, "Wrong", "", nil, nil, nil)
	content, err := frontmatter.Attach(meta, "# Wrong\n")
	if err != nil {
		t.Fatalf("attach source-kind frontmatter: %v", err)
	}
	if _, _, err := ExtractMarkdownRecords("wiki/wrong-kind.md", []byte(content)); err == nil {
		t.Fatal("extract wiki with source kind succeeded, want error")
	}
}

func TestExtractMarkdownRecordsSourceIDIsPathDerived(t *testing.T) {
	docA, _, err := ExtractMarkdownRecords("sources/path-derived.md", []byte("First body.\n"))
	if err != nil {
		t.Fatalf("extract source A: %v", err)
	}
	docB, _, err := ExtractMarkdownRecords("sources/path-derived.md", []byte("Second body.\n"))
	if err != nil {
		t.Fatalf("extract source B: %v", err)
	}
	if docA.ID != docB.ID {
		t.Fatalf("source ID changed with content: %q vs %q", docA.ID, docB.ID)
	}
	if docA.Hash == docB.Hash {
		t.Fatalf("source content hashes should differ for different content: %q", docA.Hash)
	}
}

func assertSections(t *testing.T, got, want []Section) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("section count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("section %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

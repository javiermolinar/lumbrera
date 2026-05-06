package searchindex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/textutil"
)

// ExtractMarkdownRecords converts one canonical Markdown file into normalized
// document and section records for RebuildRecords.
func ExtractMarkdownRecords(relPath string, content []byte) (Document, []Section, error) {
	doc, sections, _, _, _, err := ExtractMarkdownRecordsWithFacts(relPath, content)
	return doc, sections, err
}

// ExtractMarkdownRecordsWithFacts converts one canonical Markdown file into
// normalized document, section, and relationship fact records.
func ExtractMarkdownRecordsWithFacts(relPath string, content []byte) (Document, []Section, []DocumentLink, []DocumentCitation, []DocumentTag, error) {
	normalizedPath, kind, err := pathpolicy.NormalizeTargetPath(relPath)
	if err != nil {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("normalize indexed path: %w", err)
	}

	switch kind {
	case KindWiki:
		return extractWikiRecords(normalizedPath, content)
	case KindSource:
		doc, sections, err := extractSourceRecords(normalizedPath, content)
		return doc, sections, nil, nil, nil, err
	default:
		return Document{}, nil, nil, nil, nil, fmt.Errorf("unsupported indexed path kind %q", kind)
	}
}

func extractWikiRecords(relPath string, content []byte) (Document, []Section, []DocumentLink, []DocumentCitation, []DocumentTag, error) {
	meta, body, hasFrontmatter, err := frontmatter.Split(content)
	if err != nil {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !hasFrontmatter {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if meta.Lumbrera.Kind != KindWiki {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, KindWiki)
	}

	sections, err := markdownSections(meta.Lumbrera.ID, body)
	if err != nil {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("split %s into sections: %w", relPath, err)
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("analyze %s relationships: %w", relPath, err)
	}
	modifiedDate := strings.TrimSpace(meta.Lumbrera.ModifiedDate)
	if modifiedDate == "" {
		return Document{}, nil, nil, nil, nil, fmt.Errorf("%s is missing generated lumbrera.modified_date; run lumbrera index --rebuild --brain <path> to repair older pages", relPath)
	}
	doc := Document{
		ID:           meta.Lumbrera.ID,
		Path:         relPath,
		Kind:         KindWiki,
		Title:        strings.TrimSpace(meta.Title),
		Summary:      strings.TrimSpace(meta.Summary),
		TagsJSON:     jsonStringArray(meta.Tags),
		SourcesJSON:  jsonStringArray(meta.Lumbrera.Sources),
		LinksJSON:    jsonStringArray(meta.Lumbrera.Links),
		TagsText:     textList(meta.Tags),
		SourcesText:  textList(meta.Lumbrera.Sources),
		LinksText:    textList(meta.Lumbrera.Links),
		ModifiedDate: modifiedDate,
		Hash:         contentHash(content),
		SizeBytes:    int64(len(content)),
	}
	links, citations, tags := wikiRelationshipFacts(doc, meta, analysis)
	return doc, sections, links, citations, tags, nil
}

func extractSourceRecords(relPath string, content []byte) (Document, []Section, error) {
	body := string(content)
	sections, err := markdownSections(sourceDocumentID(relPath), body)
	if err != nil {
		return Document{}, nil, fmt.Errorf("split %s into sections: %w", relPath, err)
	}
	doc := Document{
		ID:          sourceDocumentID(relPath),
		Path:        relPath,
		Kind:        KindSource,
		Title:       sourceTitle(relPath, sections),
		Summary:     "",
		TagsJSON:    "[]",
		SourcesJSON: "[]",
		LinksJSON:   "[]",
		TagsText:    "",
		SourcesText: "",
		LinksText:   "",
		Hash:        contentHash(content),
		SizeBytes:   int64(len(content)),
	}
	return doc, sections, nil
}

func markdownSections(documentID, body string) ([]Section, error) {
	parsed, err := md.SplitSections(body)
	if err != nil {
		return nil, err
	}
	sections := make([]Section, 0, len(parsed))
	for _, section := range parsed {
		sections = append(sections, Section{
			DocumentID: documentID,
			Ordinal:    section.Ordinal,
			Heading:    section.Heading,
			Anchor:     section.Anchor,
			Level:      section.Level,
			Body:       section.Body,
		})
	}
	return sections, nil
}

func sourceTitle(relPath string, sections []Section) string {
	for _, section := range sections {
		if section.Level == 1 && strings.TrimSpace(section.Heading) != "" {
			return strings.TrimSpace(section.Heading)
		}
	}
	for _, section := range sections {
		if strings.TrimSpace(section.Heading) != "" {
			return strings.TrimSpace(section.Heading)
		}
	}
	return titleForPath(relPath)
}

func sourceDocumentID(relPath string) string {
	sum := sha256.Sum256([]byte(relPath))
	return "source_" + hex.EncodeToString(sum[:16])
}

func wikiRelationshipFacts(doc Document, meta frontmatter.Document, analysis md.Analysis) ([]DocumentLink, []DocumentCitation, []DocumentTag) {
	links := make([]DocumentLink, 0, len(analysis.LinkReferences))
	for _, ref := range analysis.LinkReferences {
		if ref.Path == "" || ref.Path == doc.Path {
			continue
		}
		links = append(links, DocumentLink{
			FromDocumentID: doc.ID,
			FromPath:       doc.Path,
			ToPath:         ref.Path,
			ToAnchor:       ref.Anchor,
			Kind:           kindForLinkedPath(ref.Path),
		})
	}

	citations := make([]DocumentCitation, 0, len(meta.Lumbrera.Sources)+len(analysis.SourceCitations))
	for _, source := range uniqueSortedStrings(meta.Lumbrera.Sources) {
		citations = append(citations, DocumentCitation{
			DocumentID:   doc.ID,
			WikiPath:     doc.Path,
			SourcePath:   source,
			CitationText: source,
			CitationKind: "frontmatter_source",
		})
	}
	for _, ref := range analysis.SourceCitations {
		citations = append(citations, DocumentCitation{
			DocumentID:   doc.ID,
			WikiPath:     doc.Path,
			SourcePath:   ref.Path,
			SourceAnchor: ref.Anchor,
			CitationText: ref.String(),
			CitationKind: "inline_source",
		})
	}

	tags := make([]DocumentTag, 0, len(meta.Tags))
	for _, tag := range uniqueSortedStrings(meta.Tags) {
		tags = append(tags, DocumentTag{DocumentID: doc.ID, Path: doc.Path, Tag: tag})
	}
	return links, citations, tags
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func jsonStringArray(values []string) string {
	encoded, err := json.Marshal(uniqueSortedStrings(values))
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func textList(values []string) string {
	return strings.Join(uniqueSortedStrings(values), " ")
}

func titleForPath(relPath string) string {
	return textutil.TitleForPath(relPath)
}

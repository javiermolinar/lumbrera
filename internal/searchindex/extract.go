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
	normalizedPath, kind, err := pathpolicy.NormalizeTargetPath(relPath)
	if err != nil {
		return Document{}, nil, fmt.Errorf("normalize indexed path: %w", err)
	}

	switch kind {
	case KindWiki:
		return extractWikiRecords(normalizedPath, content)
	case KindSource:
		return extractSourceRecords(normalizedPath, content)
	default:
		return Document{}, nil, fmt.Errorf("unsupported indexed path kind %q", kind)
	}
}

func extractWikiRecords(relPath string, content []byte) (Document, []Section, error) {
	meta, body, hasFrontmatter, err := frontmatter.Split(content)
	if err != nil {
		return Document{}, nil, fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !hasFrontmatter {
		return Document{}, nil, fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if meta.Lumbrera.Kind != KindWiki {
		return Document{}, nil, fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, KindWiki)
	}

	sections, err := markdownSections(meta.Lumbrera.ID, body)
	if err != nil {
		return Document{}, nil, fmt.Errorf("split %s into sections: %w", relPath, err)
	}
	doc := Document{
		ID:          meta.Lumbrera.ID,
		Path:        relPath,
		Kind:        KindWiki,
		Title:       strings.TrimSpace(meta.Title),
		Summary:     strings.TrimSpace(meta.Summary),
		TagsJSON:    jsonStringArray(meta.Tags),
		SourcesJSON: jsonStringArray(meta.Lumbrera.Sources),
		LinksJSON:   jsonStringArray(meta.Lumbrera.Links),
		TagsText:    textList(meta.Tags),
		SourcesText: textList(meta.Lumbrera.Sources),
		LinksText:   textList(meta.Lumbrera.Links),
		Hash:        contentHash(content),
		SizeBytes:   int64(len(content)),
	}
	return doc, sections, nil
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

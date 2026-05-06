package searchindex

import (
	"fmt"
	"sort"
	"strings"
)

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

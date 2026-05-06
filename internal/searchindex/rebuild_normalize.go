package searchindex

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

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

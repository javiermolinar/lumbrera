package searchindex

import (
	"fmt"
)

const (
	KindWiki   = "wiki"
	KindSource = "source"

	TierCanonical = "canonical"
	TierDesign    = "design"
	TierReference = "reference"
)

// Document is the normalized file-level projection inserted into the derived
// search index.

type Document struct {
	ID           string
	Path         string
	Kind         string
	Tier         string
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

func SectionID(documentID string, ordinal int) string {
	return fmt.Sprintf("%s#section-%04d", documentID, ordinal)
}

package searchindex

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

func pageConnectivityCandidates(docs []*candidateDocument, termDF map[string]int, kind string) []Candidate {
	var candidates []Candidate
	for _, doc := range docs {
		incoming := len(doc.Incoming)
		outgoing := len(doc.Outgoing)
		switch {
		case incoming == 0 && outgoing == 0:
			score := 0.52
			candidate := Candidate{
				Type:              CandidateTypeOrphanPage,
				Confidence:        confidenceForScore(score),
				Score:             score,
				Pages:             []string{doc.Path},
				Reasons:           []CandidateReason{{Code: ReasonOrphanPage, Value: "incoming=0,outgoing=0"}},
				SuggestedQueries:  singleDocumentSuggestedQueries(doc, termDF, len(docs)),
				ReviewInstruction: reviewInstruction(CandidateTypeOrphanPage),
			}
			candidates = append(candidates, candidate)
		case (kind == CandidateKindAll || kind == CandidateKindOrphans) && (incoming == 0 || outgoing == 0):
			score := 0.34
			candidate := Candidate{
				Type:              CandidateTypeUnderlinkedPage,
				Confidence:        confidenceForScore(score),
				Score:             score,
				Pages:             []string{doc.Path},
				Reasons:           []CandidateReason{{Code: ReasonUnderlinkedPage, Value: fmt.Sprintf("incoming=%d,outgoing=%d", incoming, outgoing)}},
				SuggestedQueries:  singleDocumentSuggestedQueries(doc, termDF, len(docs)),
				ReviewInstruction: reviewInstruction(CandidateTypeUnderlinkedPage),
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func stubPageCandidates(docs []*candidateDocument) []Candidate {
	var candidates []Candidate
	for _, doc := range docs {
		if doc.BodyLines >= stubPageMaxBodyLines {
			continue
		}
		score := 0.32
		if doc.BodyLines <= 5 {
			score = 0.42
		}
		candidates = append(candidates, Candidate{
			Type:              CandidateTypeStubPage,
			Confidence:        confidenceForScore(score),
			Score:             score,
			Pages:             []string{doc.Path},
			Reasons:           []CandidateReason{{Code: ReasonStubPage, Value: fmt.Sprintf("body_lines=%d", doc.BodyLines)}},
			SuggestedQueries:  singleDocumentSuggestedQueries(doc, nil, 0),
			ReviewInstruction: reviewInstruction(CandidateTypeStubPage),
		})
	}
	return candidates
}

func tagAnomalyCandidates(docs []*candidateDocument, tagDF map[string]int) []Candidate {
	totalDocs := len(docs)
	if totalDocs < tagAnomalyMinWikiPages {
		return nil
	}
	broadThreshold := int(tagAnomalyBroadRatio * float64(totalDocs))

	seen := map[string]struct{}{}
	var candidates []Candidate
	for _, doc := range docs {
		for _, tag := range doc.Tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}

			df := tagDF[tag]
			var reason CandidateReason
			var score float64
			switch {
			case df == 1:
				reason = CandidateReason{Code: ReasonTagTooSpecific, Value: fmt.Sprintf("%s,pages=%d", tag, df)}
				score = 0.28
			case df > broadThreshold:
				reason = CandidateReason{Code: ReasonTagTooBroad, Value: fmt.Sprintf("%s,pages=%d,total=%d", tag, df, totalDocs)}
				score = 0.38
			default:
				continue
			}

			pages := tagAnomalyPages(docs, tag, df)
			candidates = append(candidates, Candidate{
				Type:              CandidateTypeTagAnomaly,
				Confidence:        confidenceForScore(score),
				Score:             score,
				Pages:             pages,
				Reasons:           []CandidateReason{reason},
				SuggestedQueries:  []string{tag},
				ReviewInstruction: reviewInstruction(CandidateTypeTagAnomaly),
			})
		}
	}
	return candidates
}

// tagAnomalyPages returns affected pages for a tag anomaly candidate.
// For singleton tags, returns the single page. For broad tags, returns up to 3
// example pages to keep the output compact.
func tagAnomalyPages(docs []*candidateDocument, tag string, df int) []string {
	limit := df
	if limit > 3 {
		limit = 3
	}
	pages := make([]string, 0, limit)
	for _, doc := range docs {
		if stringSliceContainsExact(doc.Tags, tag) {
			pages = append(pages, doc.Path)
			if len(pages) == limit {
				break
			}
		}
	}
	return pages
}

type sourceSectionInfo struct {
	anchor  string
	heading string
}

func sourceCoverageGapCandidates(ctx context.Context, db *sql.DB, sourceDocs []*candidateDocument) ([]Candidate, error) {
	// Load all H2/H3 source sections with anchors.
	sectionsBySource := map[string][]sourceSectionInfo{}
	secRows, err := db.QueryContext(ctx, `
		SELECT s.path, s.anchor, COALESCE(s.heading, '')
		FROM sections s
		JOIN documents d ON d.id = s.document_id
		WHERE d.kind = 'source'
		AND s.level IN (2, 3)
		AND s.anchor IS NOT NULL AND s.anchor != ''
		ORDER BY s.path, s.ordinal`)
	if err != nil {
		return nil, fmt.Errorf("read source sections for coverage gap: %w", err)
	}
	defer secRows.Close()
	for secRows.Next() {
		var path, anchor, heading string
		if err := secRows.Scan(&path, &anchor, &heading); err != nil {
			return nil, fmt.Errorf("scan source section: %w", err)
		}
		sectionsBySource[path] = append(sectionsBySource[path], sourceSectionInfo{anchor: anchor, heading: heading})
	}
	if err := secRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source sections: %w", err)
	}

	// Load all citation facts to determine which sources are cited and which
	// specific anchors are covered.
	citedSources := map[string]struct{}{}
	citedAnchors := map[string]map[string]struct{}{}
	citRows, err := db.QueryContext(ctx, `SELECT DISTINCT source_path, source_anchor FROM document_citations`)
	if err != nil {
		return nil, fmt.Errorf("read cited source anchors: %w", err)
	}
	defer citRows.Close()
	for citRows.Next() {
		var path, anchor string
		if err := citRows.Scan(&path, &anchor); err != nil {
			return nil, fmt.Errorf("scan cited anchor: %w", err)
		}
		citedSources[path] = struct{}{}
		if anchor != "" {
			if citedAnchors[path] == nil {
				citedAnchors[path] = map[string]struct{}{}
			}
			citedAnchors[path][anchor] = struct{}{}
		}
	}
	if err := citRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cited anchors: %w", err)
	}

	var candidates []Candidate
	for _, source := range sourceDocs {
		sections := sectionsBySource[source.Path]
		if len(sections) == 0 {
			continue
		}
		// Skip entirely uncited sources — those are handled by uncited_source.
		if _, ok := citedSources[source.Path]; !ok {
			continue
		}

		cited := citedAnchors[source.Path]
		var uncited []sourceSectionInfo
		for _, section := range sections {
			if _, ok := cited[section.anchor]; !ok {
				uncited = append(uncited, section)
			}
		}
		if len(uncited) == 0 {
			continue
		}

		ratio := float64(len(uncited)) / float64(len(sections))
		score := clampCandidateScore(0.35 + 0.20*ratio)

		reasons := make([]CandidateReason, 0, minInt(len(uncited), sourceCoverageGapMaxReasons)+1)
		for i, section := range uncited {
			if i >= sourceCoverageGapMaxReasons {
				break
			}
			value := section.anchor
			if section.heading != "" {
				value = section.heading
			}
			reasons = append(reasons, CandidateReason{Code: ReasonUncitedSection, Value: value})
		}
		if len(uncited) > sourceCoverageGapMaxReasons {
			reasons = append(reasons, CandidateReason{
				Code:  ReasonUncitedSection,
				Value: fmt.Sprintf("...and %d more uncited sections", len(uncited)-sourceCoverageGapMaxReasons),
			})
		}

		candidates = append(candidates, Candidate{
			Type:              CandidateTypeSourceCoverageGap,
			Confidence:        confidenceForScore(score),
			Score:             score,
			Sources:           []string{source.Path},
			Reasons:           reasons,
			SuggestedQueries:  singleDocumentSuggestedQueries(source, nil, 0),
			ReviewInstruction: reviewInstruction(CandidateTypeSourceCoverageGap),
		})
	}
	return candidates, nil
}

func sourceCoverageCandidates(sourceDocs []*candidateDocument, docsByPath map[string]*candidateDocument) []Candidate {
	var candidates []Candidate
	for _, source := range sourceDocs {
		cited := false
		for _, doc := range docsByPath {
			if doc.Kind != KindWiki {
				continue
			}
			if stringSliceContainsExact(doc.Sources, source.Path) {
				cited = true
				break
			}
		}
		if cited {
			continue
		}
		score := 0.48
		candidates = append(candidates, Candidate{
			Type:              CandidateTypeUncitedSource,
			Confidence:        confidenceForScore(score),
			Score:             score,
			Sources:           []string{source.Path},
			Reasons:           []CandidateReason{{Code: ReasonUncitedSource}},
			SuggestedQueries:  singleDocumentSuggestedQueries(source, nil, 0),
			ReviewInstruction: reviewInstruction(CandidateTypeUncitedSource),
		})
	}
	return candidates
}

func filterCandidates(candidates []Candidate, opts CandidateOptions) []Candidate {
	out := candidates[:0]
	for _, candidate := range candidates {
		if !candidateMatchesKind(candidate, opts.Kind) {
			continue
		}
		if !candidateMatchesPathPrefix(candidate, opts.PathPrefix) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func candidateMatchesKind(candidate Candidate, kind string) bool {
	switch kind {
	case CandidateKindAll:
		return true
	case CandidateKindDuplicates:
		return candidate.Type == CandidateTypePossibleDuplicate
	case CandidateKindLinks:
		return candidate.Type == CandidateTypeMissingLink || candidateHasReason(candidate, ReasonNotLinked)
	case CandidateKindSources:
		return candidate.Type == CandidateTypeUncitedSource || candidate.Type == CandidateTypeSourceCoverageGap
	case CandidateKindStubs:
		return candidate.Type == CandidateTypeStubPage
	case CandidateKindTags:
		return candidate.Type == CandidateTypeTagAnomaly
	case CandidateKindOrphans:
		return candidate.Type == CandidateTypeOrphanPage || candidate.Type == CandidateTypeUnderlinkedPage
	default:
		return false
	}
}

func candidateMatchesPathPrefix(candidate Candidate, prefix string) bool {
	if prefix == "" {
		return true
	}
	for _, page := range candidate.Pages {
		if strings.HasPrefix(page, prefix) {
			return true
		}
	}
	for _, source := range candidate.Sources {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	return false
}

// diversifyCandidates allocates slots across candidate categories so that
// --kind all returns a mixed review queue instead of being dominated by
// whichever type produces the most high-scoring candidates.
func diversifyCandidates(candidates []Candidate, limit int) []Candidate {
	type catDef struct {
		types []string
		base  int // slots for baseTotal=10
	}
	defs := []catDef{
		{types: []string{CandidateTypePossibleDuplicate, CandidateTypeMissingLink}, base: 3},
		{types: []string{CandidateTypeUncitedSource, CandidateTypeSourceCoverageGap}, base: 3},
		{types: []string{CandidateTypeOrphanPage, CandidateTypeUnderlinkedPage}, base: 2},
		{types: []string{CandidateTypeStubPage}, base: 1},
		{types: []string{CandidateTypeTagAnomaly}, base: 1},
	}
	const baseTotal = 10

	// Scale quotas proportionally to the requested limit.
	quotas := make([]int, len(defs))
	allocated := 0
	for i, d := range defs {
		q := d.base * limit / baseTotal
		if q < 1 {
			q = 1
		}
		quotas[i] = q
		allocated += q
	}
	// Trim excess when limit is small (e.g. limit=3 → 5 categories each want 1).
	for allocated > limit {
		trimmed := false
		for i := len(quotas) - 1; i >= 0 && allocated > limit; i-- {
			if quotas[i] > 0 {
				quotas[i]--
				allocated--
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
	}

	// Select up to quota per category from the pre-sorted input.
	selected := make(map[int]struct{}, limit)
	result := make([]Candidate, 0, limit)
	for ci, d := range defs {
		count := 0
		for i, c := range candidates {
			if _, ok := selected[i]; ok {
				continue
			}
			if !candidateTypeInSlice(c.Type, d.types) {
				continue
			}
			selected[i] = struct{}{}
			result = append(result, c)
			count++
			if count >= quotas[ci] {
				break
			}
		}
	}

	// Fill remaining slots with highest-scoring unselected candidates.
	for i := range candidates {
		if len(result) >= limit {
			break
		}
		if _, ok := selected[i]; ok {
			continue
		}
		result = append(result, candidates[i])
	}

	sortCandidates(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func candidateTypeInSlice(candidateType string, types []string) bool {
	for _, t := range types {
		if candidateType == t {
			return true
		}
	}
	return false
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Type != candidates[j].Type {
			return candidates[i].Type < candidates[j].Type
		}
		leftKey := candidateSortKey(candidates[i])
		rightKey := candidateSortKey(candidates[j])
		return leftKey < rightKey
	})
}

func candidateSortKey(candidate Candidate) string {
	parts := append([]string{}, candidate.Pages...)
	parts = append(parts, candidate.Sources...)
	return strings.Join(parts, "\x00")
}

func candidateHasReason(candidate Candidate, code string) bool {
	for _, reason := range candidate.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func stringSliceContainsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func reviewInstruction(candidateType string) string {
	switch candidateType {
	case CandidateTypePossibleDuplicate:
		return "Read both pages and cited sources before deciding whether to merge, cross-link, or leave separate."
	case CandidateTypeMissingLink:
		return "Read both pages before deciding whether a cross-link is warranted."
	case CandidateTypeOrphanPage:
		return "Search for related pages before deciding whether to link, merge, or leave standalone."
	case CandidateTypeUnderlinkedPage:
		return "Search for related pages before deciding whether additional inbound or outbound links are warranted."
	case CandidateTypeUncitedSource:
		return "Search for concepts in this source and decide whether to create or update wiki coverage, or leave it uncited."
	case CandidateTypeSourceCoverageGap:
		return "Review uncited source sections and decide whether to add wiki coverage, inline citations, or leave them uncovered."
	case CandidateTypeStubPage:
		return "Review whether this page should be expanded with more content or merged into a parent page."
	case CandidateTypeTagAnomaly:
		return "Review whether this tag adds search value. Singleton tags may need removal or broader application; broad tags may need splitting or removal."
	default:
		return "Review deterministic evidence before deciding whether any knowledge-base mutation is warranted."
	}
}

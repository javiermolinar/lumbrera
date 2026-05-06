package searchindex

import (
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
		return candidate.Type == CandidateTypeUncitedSource
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
	default:
		return "Review deterministic evidence before deciding whether any knowledge-base mutation is warranted."
	}
}

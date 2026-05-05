package searchindex

import "sort"

func rerankCandidates(results []SearchResult, queryTerms []string) []SearchResult {
	if len(results) == 0 {
		return []SearchResult{}
	}
	out := append([]SearchResult(nil), results...)
	productTerms := detectedProductTerms(out, queryTerms)
	if len(productTerms) == 0 {
		return out
	}
	productSet := map[string]struct{}{}
	for _, term := range productTerms {
		productSet[term] = struct{}{}
	}
	for i := range out {
		alignment := resultEntityAlignment(out[i], productSet)
		switch alignment {
		case entityAlignmentMatch:
			out[i].Score -= entityMatchBoost
		case entityAlignmentMismatch:
			out[i].Score += entityMismatchPenalty
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return betterResult(out[i], out[j])
	})
	return out
}

func searchCandidateLimit(limit int) int {
	candidateLimit := limit * 5
	if candidateLimit < searchCandidateLimitMinimum {
		candidateLimit = searchCandidateLimitMinimum
	}
	if candidateLimit > searchCandidateLimitMaximum {
		candidateLimit = searchCandidateLimitMaximum
	}
	return candidateLimit
}

func selectSearchResults(candidates []SearchResult, limit int, recommendedSections []RecommendedSection) []SearchResult {
	if len(candidates) == 0 || limit <= 0 {
		return []SearchResult{}
	}
	countsByDocument := map[string]int{}
	seenSections := map[string]struct{}{}
	out := make([]SearchResult, 0, minInt(limit, len(candidates)))
	bySectionID := make(map[string]SearchResult, len(candidates))
	for _, result := range candidates {
		bySectionID[result.SectionID] = result
	}
	for _, section := range recommendedSections {
		result, ok := bySectionID[section.SectionID]
		if !ok {
			continue
		}
		out = appendSearchResult(out, countsByDocument, seenSections, result)
		if len(out) == limit {
			return out
		}
	}
	for _, result := range candidates {
		out = appendSearchResult(out, countsByDocument, seenSections, result)
		if len(out) == limit {
			break
		}
	}
	return out
}

func appendSearchResult(out []SearchResult, countsByDocument map[string]int, seenSections map[string]struct{}, result SearchResult) []SearchResult {
	if _, ok := seenSections[result.SectionID]; ok {
		return out
	}
	key := result.DocumentID
	if key == "" {
		key = result.Path
	}
	if countsByDocument[key] >= maxSectionsPerDocument {
		return out
	}
	countsByDocument[key]++
	seenSections[result.SectionID] = struct{}{}
	return append(out, result)
}

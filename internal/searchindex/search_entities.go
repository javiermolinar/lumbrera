package searchindex

func detectedProductTerms(results []SearchResult, queryTerms []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(queryTerms))
	for _, term := range queryTerms {
		if isRecommendationStopword(term) {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		for _, result := range results {
			if resultHasEntityTerm(result, term) {
				seen[term] = struct{}{}
				out = append(out, term)
				break
			}
		}
	}
	return out
}

type entityAlignment int

const (
	entityAlignmentNeutral entityAlignment = iota
	entityAlignmentMatch
	entityAlignmentMismatch
)

func resultEntityAlignment(result SearchResult, productSet map[string]struct{}) entityAlignment {
	for term := range productSet {
		if resultHasEntityTerm(result, term) {
			return entityAlignmentMatch
		}
	}
	if entity := resultPrimaryEntity(result); entity != "" {
		if _, ok := productSet[entity]; !ok {
			return entityAlignmentMismatch
		}
	}
	return entityAlignmentNeutral
}

func resultPrimaryEntity(result SearchResult) string {
	pathToken := firstPathToken(result.Path)
	if pathToken == "" || isRecommendationStopword(pathToken) {
		return ""
	}
	if stringSliceContains(result.Tags, pathToken) || result.Kind == KindSource {
		return pathToken
	}
	return ""
}

func resultHasEntityTerm(result SearchResult, term string) bool {
	return firstPathToken(result.Path) == term || firstTextToken(result.Title) == term
}

func bestPathForTerm(results []SearchResult, kind, term string) (string, bool) {
	return bestPathForTermInContext(results, kind, term, "")
}

func bestPathForTermInContext(results []SearchResult, kind, term, product string) (string, bool) {
	if product != "" {
		if path, ok := bestPathForTermFiltered(results, kind, term, product); ok {
			return path, true
		}
	}
	return bestPathForTermFiltered(results, kind, term, "")
}

func bestPathForTermFiltered(results []SearchResult, kind, term, product string) (string, bool) {
	var best SearchResult
	bestQuality := 0
	for _, result := range results {
		if result.Kind != kind {
			continue
		}
		if product != "" && !resultHasEntityTerm(result, product) {
			continue
		}
		quality := resultTermQuality(result, term)
		if quality == 0 {
			continue
		}
		if bestQuality == 0 || quality > bestQuality || (quality == bestQuality && betterResult(result, best)) {
			best = result
			bestQuality = quality
		}
	}
	if bestQuality == 0 {
		return "", false
	}
	return best.Path, true
}

func resultTermQuality(result SearchResult, term string) int {
	switch {
	case pathTokensContain(result.Path, term):
		return 4
	case textTokensContain(result.Title, term):
		return 4
	case stringSliceContains(result.Tags, term):
		return 3
	case textTokensContain(result.Heading, term):
		return 2
	default:
		return 0
	}
}

func betterResult(left, right SearchResult) bool {
	if left.Score != right.Score {
		return left.Score < right.Score
	}
	if left.LexicalScore != right.LexicalScore {
		return left.LexicalScore < right.LexicalScore
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	return left.SectionID < right.SectionID
}

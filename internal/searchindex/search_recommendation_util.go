package searchindex

import (
	"path"
	"strings"
)

func dedupeBestPaths(results []SearchResult, kind string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(results))
	for _, result := range results {
		if result.Kind != kind {
			continue
		}
		if _, ok := seen[result.Path]; ok {
			continue
		}
		seen[result.Path] = struct{}{}
		out = append(out, result.Path)
	}
	return out
}

func appendRecommendedPath(out []string, seen map[string]struct{}, path string) []string {
	if len(out) == maxRecommendedReadOrder || path == "" {
		return out
	}
	if _, ok := seen[path]; ok {
		return out
	}
	seen[path] = struct{}{}
	return append(out, path)
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func pathTokensContain(value, term string) bool {
	base := path.Base(value)
	base = strings.TrimSuffix(base, path.Ext(base))
	return textTokensContain(base, term)
}

func firstPathToken(value string) string {
	base := path.Base(value)
	base = strings.TrimSuffix(base, path.Ext(base))
	return firstTextToken(base)
}

func firstTextToken(value string) string {
	tokens := strings.Fields(normalizeFTSText(value))
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func textTokensContain(value, term string) bool {
	for _, token := range strings.Fields(normalizeFTSText(value)) {
		if token == term {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, term string) bool {
	for _, value := range values {
		if normalizeFTSText(value) == term {
			return true
		}
	}
	return false
}

func isRecommendationStopword(term string) bool {
	return searchStopwords[term] || recommendationStopwords[term] || len(term) < 3
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var recommendationStopwords = map[string]bool{
	"about": true, "answer": true, "better": true, "compare": true,
	"comparison": true, "difference": true, "differences": true, "explain": true,
	"find": true, "information": true, "read": true, "relationship": true,
	"search": true, "understand": true, "versus": true, "vs": true,
}

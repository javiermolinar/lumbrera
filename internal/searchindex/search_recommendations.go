package searchindex

import (
	"fmt"
	"path"
	"strings"
)

func defaultAgentInstructions() AgentInstructions {
	return AgentInstructions{
		ReadFirst: "recommended_sections",
		DoNot:     []string{"scan_repo"},
		Fallback:  "ask_for_clearer_terms_or_use_index_tags_only",
	}
}

func coverageForRecommendations(results []SearchResult, sections []RecommendedSection, queryTerms []string) SearchCoverage {
	productTerms := detectedProductTerms(results, queryTerms)
	coverage := SearchCoverage{
		Entities: map[string]bool{},
		Missing:  []string{},
	}
	if len(productTerms) == 0 {
		return coverage
	}
	bySectionID := make(map[string]SearchResult, len(results))
	for _, result := range results {
		bySectionID[result.SectionID] = result
	}
	for _, term := range productTerms {
		covered := false
		for _, section := range sections {
			result, ok := bySectionID[section.SectionID]
			if !ok {
				continue
			}
			if resultHasEntityTerm(result, term) {
				covered = true
				break
			}
		}
		coverage.Entities[term] = covered
		if !covered {
			coverage.Missing = append(coverage.Missing, term)
		}
	}
	return coverage
}

func recommendations(results []SearchResult, opts SearchOptions, queryTerms []string) ([]string, []RecommendedSection, string) {
	if len(results) == 0 {
		return []string{}, []RecommendedSection{}, "No indexed content matched. Ask for clearer terms or use INDEX.md/tags.md only if fallback navigation is required; do not scan the repo."
	}
	if opts.Kind == KindSource || !hasKind(results, KindWiki) {
		paths := balancedBestPaths(results, KindSource, queryTerms)
		return paths, recommendedSectionsForPaths(results, paths, KindSource, queryTerms), "Read these source sections/files directly. Do not scan the repo unless they are insufficient."
	}
	paths := balancedBestPaths(results, KindWiki, queryTerms)
	return paths, recommendedSectionsForPaths(results, paths, KindWiki, queryTerms), "Read recommended_sections from the top wiki pages first. Do not scan the repo unless those are insufficient."
}

func hasKind(results []SearchResult, kind string) bool {
	for _, result := range results {
		if result.Kind == kind {
			return true
		}
	}
	return false
}

func balancedBestPaths(results []SearchResult, kind string, queryTerms []string) []string {
	base := dedupeBestPaths(results, kind)
	if len(base) == 0 {
		return []string{}
	}
	if kind != KindWiki {
		return limitStrings(base, maxRecommendedReadOrder)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, maxRecommendedReadOrder)
	productTerms := detectedProductTerms(results, queryTerms)
	productSet := map[string]struct{}{}
	for _, term := range productTerms {
		productSet[term] = struct{}{}
	}

	// When a query names multiple products, recommend one strong wiki page for
	// each product before falling back to global lexical order. With one product,
	// treat it as context and balance on the remaining topic terms instead.
	if len(productTerms) > 1 {
		for _, term := range productTerms {
			if path, ok := bestPathForTerm(results, kind, term); ok {
				out = appendRecommendedPath(out, seen, path)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	contextProduct := ""
	if len(productTerms) == 1 {
		contextProduct = productTerms[0]
	}
	for _, term := range queryTerms {
		if isRecommendationStopword(term) {
			continue
		}
		if _, isProduct := productSet[term]; isProduct {
			continue
		}
		if path, ok := bestPathForTermInContext(results, kind, term, contextProduct); ok {
			out = appendRecommendedPath(out, seen, path)
			if len(out) == maxRecommendedReadOrder {
				return out
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, path := range base {
		out = appendRecommendedPath(out, seen, path)
		if len(out) == maxRecommendedReadOrder {
			break
		}
	}
	return out
}

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

func recommendedSectionsForPaths(results []SearchResult, paths []string, kind string, queryTerms []string) []RecommendedSection {
	if len(paths) == 0 {
		return []RecommendedSection{}
	}
	productTerms := detectedProductTerms(results, queryTerms)
	sections := make([]RecommendedSection, 0, len(paths))
	for _, path := range paths {
		var best SearchResult
		bestQuality := -1
		for _, result := range results {
			if result.Kind != kind || result.Path != path {
				continue
			}
			quality := recommendedSectionQuality(result, queryTerms, productTerms)
			if bestQuality < 0 || quality > bestQuality || (quality == bestQuality && betterResult(result, best)) {
				best = result
				bestQuality = quality
			}
		}
		if bestQuality < 0 {
			continue
		}
		sections = append(sections, RecommendedSection{
			SectionID: best.SectionID,
			Target:    resultTarget(best),
			Path:      best.Path,
			Anchor:    best.Anchor,
			Kind:      best.Kind,
			Title:     best.Title,
			Heading:   best.Heading,
			Reason:    recommendedSectionReason(best, productTerms),
		})
	}
	return sections
}

func recommendedSectionQuality(result SearchResult, queryTerms []string, productTerms []string) int {
	productSet := map[string]struct{}{}
	for _, term := range productTerms {
		productSet[term] = struct{}{}
	}
	lastTopicTerm := ""
	for _, term := range queryTerms {
		if isRecommendationStopword(term) {
			continue
		}
		if _, isProduct := productSet[term]; isProduct {
			continue
		}
		lastTopicTerm = term
	}
	quality := 0
	for _, term := range queryTerms {
		if isRecommendationStopword(term) {
			continue
		}
		if _, isProduct := productSet[term]; isProduct {
			continue
		}
		switch {
		case textTokensContain(result.Heading, term):
			quality += 12
			if term == lastTopicTerm {
				quality += 20
			}
		case pathTokensContain(result.Path, term) || textTokensContain(result.Title, term):
			quality += 4
		case strings.Contains(normalizeFTSText(result.Snippet), term):
			quality += 1
		}
	}
	return quality
}

func recommendedSectionReason(result SearchResult, productTerms []string) string {
	label := result.Heading
	if label == "" {
		label = result.Title
	}
	for _, term := range productTerms {
		if resultHasEntityTerm(result, term) {
			return fmt.Sprintf("Covers %s: %s", displayTerm(term), label)
		}
	}
	return fmt.Sprintf("Best matching %s section: %s", result.Kind, label)
}

func displayTerm(term string) string {
	if term == "" {
		return ""
	}
	return strings.ToUpper(term[:1]) + term[1:]
}

func resultTarget(result SearchResult) string {
	if result.Anchor == "" {
		return result.Path
	}
	return result.Path + "#" + result.Anchor
}

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

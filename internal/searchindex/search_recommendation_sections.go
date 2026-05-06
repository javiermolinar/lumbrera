package searchindex

import (
	"fmt"
	"strings"
)

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

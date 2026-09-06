package searchindex

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
		return []string{}, []RecommendedSection{}, "No indexed content matched. Ask for clearer terms or use INDEX.md, NOTES.md, and tags.md only if fallback navigation is required; do not scan the repo."
	}
	kind := preferredRecommendationKind(results, opts.Kind)
	paths := balancedBestPaths(results, kind, queryTerms)
	sections := recommendedSectionsForPaths(results, paths, kind, queryTerms)
	switch kind {
	case KindWiki:
		return paths, sections, "Read recommended_sections from the top wiki pages first. Do not scan the repo unless those are insufficient."
	case KindNote:
		return paths, sections, "Read these note sections first. Do not fall back to raw sources or scan the repo unless they are insufficient."
	default:
		return paths, sections, "Read these source sections/files directly. Do not scan the repo unless they are insufficient."
	}
}

func preferredRecommendationKind(results []SearchResult, requested string) string {
	if requested != "" && requested != KindAll {
		return requested
	}
	for _, kind := range []string{KindWiki, KindNote, KindSource} {
		if hasKind(results, kind) {
			return kind
		}
	}
	return KindSource
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

package searchindex

import (
	"path"
	"sort"
	"strings"
)

func intersectSortedStrings(left, right []string) []string {
	leftSet := map[string]struct{}{}
	for _, value := range left {
		if value != "" {
			leftSet[value] = struct{}{}
		}
	}
	out := make([]string, 0, minInt(len(left), len(right)))
	seen := map[string]struct{}{}
	for _, value := range right {
		if value == "" {
			continue
		}
		if _, ok := leftSet[value]; !ok {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func pairSuggestedQueries(left, right *candidateDocument, sharedTags, sharedSources, lexicalTerms []string, termDF map[string]int, totalDocs int) []string {
	var queries []string
	first := append([]string{}, sharedTags...)
	first = append(first, lexicalTerms...)
	first = append(first, pathTerms(left.Path)...)
	first = append(first, pathTerms(right.Path)...)
	first = cleanCandidateQueryTerms(first, 7)
	if len(first) > 0 {
		queries = append(queries, strings.Join(first, " "))
	}
	if len(sharedSources) > 0 {
		sourceTerms := append(pathTerms(sharedSources[0]), sharedTags...)
		sourceTerms = append(sourceTerms, lexicalTerms...)
		sourceTerms = cleanCandidateQueryTerms(sourceTerms, 7)
		if len(sourceTerms) > 0 {
			queries = append(queries, strings.Join(sourceTerms, " "))
		}
	}
	if len(queries) == 0 {
		terms := append(queryTermsForDocument(left, termDF, totalDocs, 4), queryTermsForDocument(right, termDF, totalDocs, 4)...)
		terms = cleanCandidateQueryTerms(terms, 7)
		if len(terms) > 0 {
			queries = append(queries, strings.Join(terms, " "))
		}
	}
	return uniqueSortedByInput(queries)
}

func singleDocumentSuggestedQueries(doc *candidateDocument, termDF map[string]int, totalDocs int) []string {
	terms := queryTermsForDocument(doc, termDF, totalDocs, 7)
	if len(terms) == 0 {
		terms = pathTerms(doc.Path)
	}
	if len(terms) == 0 {
		return []string{}
	}
	return []string{strings.Join(terms, " ")}
}

func queryTermsForDocument(doc *candidateDocument, termDF map[string]int, totalDocs, limit int) []string {
	terms := make([]string, 0, limit)
	terms = append(terms, strings.Fields(normalizeFTSText(doc.Title))...)
	terms = append(terms, doc.Tags...)
	terms = append(terms, pathTerms(doc.Path)...)
	if len(terms) < limit && termDF != nil && totalDocs > 0 {
		terms = append(terms, topDocumentTerms(doc, termDF, totalDocs, limit-len(terms))...)
	}
	return cleanCandidateQueryTerms(terms, limit)
}

func topDocumentTerms(doc *candidateDocument, termDF map[string]int, totalDocs, limit int) []string {
	if limit <= 0 {
		return []string{}
	}
	contributions := make([]overlapContribution, 0, len(doc.Terms))
	for term, count := range doc.Terms {
		if isCandidateStopword(term) {
			continue
		}
		contributions = append(contributions, overlapContribution{Term: term, Contribution: candidateTermWeight(count, termDF[term], totalDocs)})
	}
	sort.Slice(contributions, func(i, j int) bool {
		if contributions[i].Contribution != contributions[j].Contribution {
			return contributions[i].Contribution > contributions[j].Contribution
		}
		return contributions[i].Term < contributions[j].Term
	})
	out := make([]string, 0, minInt(limit, len(contributions)))
	for _, contribution := range contributions {
		out = append(out, contribution.Term)
		if len(out) == limit {
			break
		}
	}
	return out
}

func pathTerms(value string) []string {
	base := path.Base(value)
	base = strings.TrimSuffix(base, path.Ext(base))
	return strings.Fields(normalizeFTSText(base))
}

func cleanCandidateQueryTerms(input []string, limit int) []string {
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, term := range input {
		for _, normalized := range strings.Fields(normalizeFTSText(term)) {
			if isCandidateStopword(normalized) {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
			if len(out) == limit {
				return out
			}
		}
	}
	return out
}

func uniqueSortedByInput(values []string) []string {
	out := values[:0]
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

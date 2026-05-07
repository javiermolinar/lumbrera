package searchindex

import (
	"context"
	"database/sql"
	"errors"
)

const (
	KindAll                     = "all"
	DefaultSearchLimit          = 5
	MaxSearchLimit              = 20
	QueryModeAND                = "and"
	QueryModeORFallback         = "or_fallback"
	wikiScoreBoost              = 0.15
	maxSectionsPerDocument      = 3
	maxRecommendedReadOrder     = 3
	entityMismatchPenalty       = 15.0
	entityMatchBoost            = 0.25
	searchCandidateLimitMinimum = 40
	searchCandidateLimitMaximum = 100
)

type SearchOptions struct {
	Limit      int
	Kind       string
	PathPrefix string
	Tags       []string
	Sources    []string
	Tiers      []string
}

type SearchResponse struct {
	Query                string
	QueryMode            string
	RecommendedSections  []RecommendedSection
	AgentInstructions    AgentInstructions
	Coverage             SearchCoverage
	Results              []SearchResult
	RecommendedReadOrder []string
	StopRule             string
}

type AgentInstructions struct {
	ReadFirst string
	DoNot     []string
	Fallback  string
}

type SearchCoverage struct {
	Entities map[string]bool
	Missing  []string
}

type RecommendedSection struct {
	SectionID string
	Target    string
	Path      string
	Anchor    string
	Kind      string
	Title     string
	Heading   string
	Reason    string
}

type SearchResult struct {
	DocumentID   string
	SectionID    string
	Path         string
	Anchor       string
	Kind         string
	Tier         string
	Title        string
	Heading      string
	Summary      string
	Tags         []string
	Sources      []string
	Links        []string
	Snippet      string
	Score        float64
	LexicalScore float64
}

type sanitizedQuery struct {
	atoms []string
	terms []string
}

func Search(ctx context.Context, db *sql.DB, query string, opts SearchOptions) (SearchResponse, error) {
	if db == nil {
		return SearchResponse{}, errors.New("search index query: nil database")
	}

	sanitized, err := sanitizeQuery(query)
	if err != nil {
		return SearchResponse{}, err
	}
	opts, err = normalizeSearchOptions(opts)
	if err != nil {
		return SearchResponse{}, err
	}

	results, err := runSearch(ctx, db, sanitized.matchAND(), opts)
	if err != nil {
		return SearchResponse{}, err
	}
	mode := QueryModeAND
	if shouldRunORFallback(results, sanitized, opts) {
		results, err = runSearch(ctx, db, sanitized.matchOR(), opts)
		if err != nil {
			return SearchResponse{}, err
		}
		mode = QueryModeORFallback
	}

	candidates := rerankCandidates(results, sanitized.terms)
	recommendedReadOrder, recommendedSections, stopRule := recommendations(candidates, opts, sanitized.terms)
	results = selectSearchResults(candidates, opts.Limit, recommendedSections)
	return SearchResponse{
		Query:                query,
		QueryMode:            mode,
		RecommendedSections:  recommendedSections,
		AgentInstructions:    defaultAgentInstructions(),
		Coverage:             coverageForRecommendations(candidates, recommendedSections, sanitized.terms),
		Results:              results,
		RecommendedReadOrder: recommendedReadOrder,
		StopRule:             stopRule,
	}, nil
}

func shouldRunORFallback(results []SearchResult, query sanitizedQuery, opts SearchOptions) bool {
	if len(query.atoms) <= 1 {
		return false
	}
	if len(results) == 0 {
		return true
	}
	if opts.Kind == KindAll && !hasKind(results, KindWiki) {
		return true
	}
	if opts.Kind == KindAll || opts.Kind == KindWiki {
		return missingWikiTopicCoverage(results, query.terms)
	}
	return false
}

func missingWikiTopicCoverage(results []SearchResult, queryTerms []string) bool {
	if !hasKind(results, KindWiki) {
		return false
	}
	productTerms := detectedProductTerms(results, queryTerms)
	productSet := map[string]struct{}{}
	for _, term := range productTerms {
		productSet[term] = struct{}{}
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
		if _, ok := bestPathForTermInContext(results, KindWiki, term, contextProduct); !ok {
			return true
		}
	}
	return false
}

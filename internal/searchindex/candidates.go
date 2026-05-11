package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	DefaultCandidateLimit = 10
	MaxCandidateLimit     = 50

	CandidateKindAll        = "all"
	CandidateKindDuplicates = "duplicates"
	CandidateKindLinks      = "links"
	CandidateKindSources    = "sources"
	CandidateKindOrphans    = "orphans"
	CandidateKindStubs      = "stubs"
	CandidateKindTags       = "tags"

	CandidateTypePossibleDuplicate = "possible_duplicate"
	CandidateTypeMissingLink       = "missing_link"
	CandidateTypeOrphanPage        = "orphan_page"
	CandidateTypeUnderlinkedPage   = "underlinked_page"
	CandidateTypeUncitedSource     = "uncited_source"
	CandidateTypeStubPage          = "stub_page"
	CandidateTypeTagAnomaly        = "tag_anomaly"
	CandidateTypeSourceCoverageGap = "source_coverage_gap"

	CandidateConfidenceHigh   = "high"
	CandidateConfidenceMedium = "medium"
	CandidateConfidenceLow    = "low"

	ReasonSharedTag         = "shared_tag"
	ReasonSharedSource      = "shared_source"
	ReasonNotLinked         = "not_linked"
	ReasonLexicalOverlap    = "lexical_overlap"
	ReasonOrphanPage        = "orphan_page"
	ReasonUnderlinkedPage   = "underlinked_page"
	ReasonUncitedSource     = "uncited_source"
	ReasonOlderRelevantPage = "older_relevant_page"
	ReasonStubPage          = "stub_page"
	ReasonTagTooSpecific    = "tag_too_specific"
	ReasonTagTooBroad       = "tag_too_broad"
	ReasonUncitedSection    = "uncited_section"

	candidateLexicalReasonThreshold = 0.08
	candidateLexicalPairThreshold   = 0.10
	candidateMinimumPairScore       = 0.18
	candidateOlderRelevanceTopPairs = 100
	staleRiskMinimumDays            = 30

	stubPageMaxBodyLines         = 15
	tagAnomalyMinWikiPages       = 3
	tagAnomalyBroadRatio         = 0.40
	sourceCoverageGapMaxReasons  = 5
)

// CandidateOptions controls deterministic health/consolidation candidate
// generation from the disposable SQLite search index.
type CandidateOptions struct {
	Limit      int
	Kind       string
	PathPrefix string
}

// CandidateResponse is the deterministic review packet consumed by the future
// health command and LLM-facing health skill. Candidates are not diagnoses;
// they only identify where to review next.
type CandidateResponse struct {
	Candidates []Candidate
	StopRule   string
}

// Candidate is a deterministic single-page/source or page-pair review item.
type Candidate struct {
	Type              string
	Confidence        string
	Score             float64
	Pages             []string
	Sources           []string
	Reasons           []CandidateReason
	SuggestedQueries  []string
	ReviewInstruction string
}

// CandidateReason explains one deterministic signal used to rank a candidate.
type CandidateReason struct {
	Code  string
	Value string
}

type candidateDocument struct {
	ID           string
	Path         string
	Kind         string
	Title        string
	Summary      string
	ModifiedDate string
	Tags         []string
	Sources      []string
	Outgoing     map[string]struct{}
	Incoming     map[string]struct{}
	Terms        map[string]int
	BodyLines    int
}

type lexicalOverlapResult struct {
	Score float64
	Terms []string
}

type overlapContribution struct {
	Term         string
	Contribution float64
}

// HealthCandidates returns deterministic candidates for LLM health and
// consolidation review. It intentionally avoids semantic conclusions.
func HealthCandidates(ctx context.Context, db *sql.DB, opts CandidateOptions) (CandidateResponse, error) {
	if db == nil {
		return CandidateResponse{}, errors.New("health candidate query: nil database")
	}

	normalized, err := normalizeCandidateOptions(opts)
	if err != nil {
		return CandidateResponse{}, err
	}
	docsByPath, err := loadCandidateDocuments(ctx, db)
	if err != nil {
		return CandidateResponse{}, err
	}
	if err := loadCandidateFacts(ctx, db, docsByPath); err != nil {
		return CandidateResponse{}, err
	}
	if err := buildCandidateTerms(ctx, db, docsByPath); err != nil {
		return CandidateResponse{}, err
	}

	wikiDocs := candidateDocumentsByKind(docsByPath, KindWiki)
	sourceDocs := candidateDocumentsByKind(docsByPath, KindSource)
	termDF := candidateTermDocumentFrequency(wikiDocs)
	tagDF := candidateTagDocumentFrequency(wikiDocs)
	sourceDF := candidateSourceDocumentFrequency(wikiDocs)

	var candidates []Candidate
	if normalized.Kind == CandidateKindAll || normalized.Kind == CandidateKindDuplicates || normalized.Kind == CandidateKindLinks {
		pairCandidates, err := pagePairCandidates(ctx, db, wikiDocs, termDF, tagDF, sourceDF)
		if err != nil {
			return CandidateResponse{}, err
		}
		candidates = append(candidates, pairCandidates...)
	}
	if normalized.Kind == CandidateKindAll || normalized.Kind == CandidateKindOrphans {
		candidates = append(candidates, pageConnectivityCandidates(wikiDocs, termDF, normalized.Kind)...)
	}
	if normalized.Kind == CandidateKindAll || normalized.Kind == CandidateKindSources {
		candidates = append(candidates, sourceCoverageCandidates(sourceDocs, docsByPath)...)
		gapCandidates, err := sourceCoverageGapCandidates(ctx, db, sourceDocs)
		if err != nil {
			return CandidateResponse{}, err
		}
		candidates = append(candidates, gapCandidates...)
	}
	if normalized.Kind == CandidateKindAll || normalized.Kind == CandidateKindStubs {
		candidates = append(candidates, stubPageCandidates(wikiDocs)...)
	}
	if normalized.Kind == CandidateKindAll || normalized.Kind == CandidateKindTags {
		candidates = append(candidates, tagAnomalyCandidates(wikiDocs, tagDF)...)
	}

	candidates = filterCandidates(candidates, normalized)
	sortCandidates(candidates)
	if normalized.Kind == CandidateKindAll {
		candidates = diversifyCandidates(candidates, normalized.Limit)
	} else if len(candidates) > normalized.Limit {
		candidates = append([]Candidate(nil), candidates[:normalized.Limit]...)
	}
	return CandidateResponse{
		Candidates: candidates,
		StopRule:   "Review top candidates first. Do not scan the repo unless candidates are insufficient.",
	}, nil
}

func normalizeCandidateOptions(opts CandidateOptions) (CandidateOptions, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultCandidateLimit
	}
	if opts.Limit > MaxCandidateLimit {
		opts.Limit = MaxCandidateLimit
	}
	if opts.Kind == "" {
		opts.Kind = CandidateKindAll
	}
	switch opts.Kind {
	case CandidateKindAll, CandidateKindDuplicates, CandidateKindLinks, CandidateKindSources, CandidateKindOrphans, CandidateKindStubs, CandidateKindTags:
	default:
		return CandidateOptions{}, fmt.Errorf("invalid candidate kind %q", opts.Kind)
	}
	prefix, err := normalizePathPrefix(opts.PathPrefix)
	if err != nil {
		return CandidateOptions{}, err
	}
	opts.PathPrefix = prefix
	return opts, nil
}

package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	DefaultCandidateLimit = 10
	MaxCandidateLimit     = 50

	CandidateKindAll        = "all"
	CandidateKindDuplicates = "duplicates"
	CandidateKindLinks      = "links"
	CandidateKindSources    = "sources"
	CandidateKindOrphans    = "orphans"

	CandidateTypePossibleDuplicate = "possible_duplicate"
	CandidateTypeMissingLink       = "missing_link"
	CandidateTypeOrphanPage        = "orphan_page"
	CandidateTypeUnderlinkedPage   = "underlinked_page"
	CandidateTypeUncitedSource     = "uncited_source"

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

	candidateLexicalReasonThreshold = 0.08
	candidateLexicalPairThreshold   = 0.10
	candidateMinimumPairScore       = 0.18
	staleRiskMinimumDays            = 30
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
	}

	candidates = filterCandidates(candidates, normalized)
	sortCandidates(candidates)
	if len(candidates) > normalized.Limit {
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
	case CandidateKindAll, CandidateKindDuplicates, CandidateKindLinks, CandidateKindSources, CandidateKindOrphans:
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

func loadCandidateDocuments(ctx context.Context, db *sql.DB) (map[string]*candidateDocument, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, path, kind, title, summary, modified_date FROM documents ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("read candidate documents: %w", err)
	}
	defer rows.Close()

	docs := map[string]*candidateDocument{}
	for rows.Next() {
		doc := &candidateDocument{
			Outgoing: map[string]struct{}{},
			Incoming: map[string]struct{}{},
		}
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.Kind, &doc.Title, &doc.Summary, &doc.ModifiedDate); err != nil {
			return nil, fmt.Errorf("scan candidate document: %w", err)
		}
		docs[doc.Path] = doc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate documents: %w", err)
	}
	return docs, nil
}

func loadCandidateFacts(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	if err := loadCandidateTags(ctx, db, docs); err != nil {
		return err
	}
	if err := loadCandidateCitations(ctx, db, docs); err != nil {
		return err
	}
	if err := loadCandidateLinks(ctx, db, docs); err != nil {
		return err
	}
	return nil
}

func loadCandidateTags(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT path, tag FROM document_tags ORDER BY path, tag`)
	if err != nil {
		return fmt.Errorf("read candidate document tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var docPath, tag string
		if err := rows.Scan(&docPath, &tag); err != nil {
			return fmt.Errorf("scan candidate document tag: %w", err)
		}
		if doc := docs[docPath]; doc != nil {
			doc.Tags = append(doc.Tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate document tags: %w", err)
	}
	for _, doc := range docs {
		doc.Tags = uniqueSortedStrings(doc.Tags)
	}
	return nil
}

func loadCandidateCitations(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT wiki_path, source_path FROM document_citations ORDER BY wiki_path, source_path`)
	if err != nil {
		return fmt.Errorf("read candidate citations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var wikiPath, sourcePath string
		if err := rows.Scan(&wikiPath, &sourcePath); err != nil {
			return fmt.Errorf("scan candidate citation: %w", err)
		}
		if doc := docs[wikiPath]; doc != nil {
			doc.Sources = append(doc.Sources, sourcePath)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate citations: %w", err)
	}
	for _, doc := range docs {
		doc.Sources = uniqueSortedStrings(doc.Sources)
	}
	return nil
}

func loadCandidateLinks(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT from_path, to_path FROM document_links WHERE kind = 'wiki' ORDER BY from_path, to_path`)
	if err != nil {
		return fmt.Errorf("read candidate wiki links: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fromPath, toPath string
		if err := rows.Scan(&fromPath, &toPath); err != nil {
			return fmt.Errorf("scan candidate wiki link: %w", err)
		}
		fromDoc := docs[fromPath]
		toDoc := docs[toPath]
		if fromDoc == nil || toDoc == nil || fromDoc.Kind != KindWiki || toDoc.Kind != KindWiki || fromPath == toPath {
			continue
		}
		fromDoc.Outgoing[toPath] = struct{}{}
		toDoc.Incoming[fromPath] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate wiki links: %w", err)
	}
	return nil
}

func buildCandidateTerms(ctx context.Context, db *sql.DB, docs map[string]*candidateDocument) error {
	builders := make(map[string]*strings.Builder, len(docs))
	byID := make(map[string]*candidateDocument, len(docs))
	for _, doc := range docs {
		var b strings.Builder
		writeCandidateTermText(&b, doc.Path)
		writeCandidateTermText(&b, doc.Title)
		writeCandidateTermText(&b, doc.Summary)
		writeCandidateTermText(&b, strings.Join(doc.Tags, " "))
		builders[doc.ID] = &b
		byID[doc.ID] = doc
	}

	rows, err := db.QueryContext(ctx, `SELECT document_id, COALESCE(heading, ''), body FROM sections ORDER BY document_id, ordinal`)
	if err != nil {
		return fmt.Errorf("read candidate section text: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var documentID, heading, body string
		if err := rows.Scan(&documentID, &heading, &body); err != nil {
			return fmt.Errorf("scan candidate section text: %w", err)
		}
		b := builders[documentID]
		if b == nil {
			continue
		}
		writeCandidateTermText(b, heading)
		writeCandidateTermText(b, body)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate candidate section text: %w", err)
	}

	for id, b := range builders {
		byID[id].Terms = candidateTermCounts(b.String())
	}
	return nil
}

func writeCandidateTermText(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(text)
}

func candidateDocumentsByKind(docsByPath map[string]*candidateDocument, kind string) []*candidateDocument {
	docs := make([]*candidateDocument, 0, len(docsByPath))
	for _, doc := range docsByPath {
		if doc.Kind == kind {
			docs = append(docs, doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs
}

func candidateTermCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, term := range strings.Fields(normalizeFTSText(text)) {
		if isCandidateStopword(term) {
			continue
		}
		counts[term]++
	}
	return counts
}

func isCandidateStopword(term string) bool {
	if len(term) < 3 {
		return true
	}
	if searchStopwords[term] || recommendationStopwords[term] || candidateStopwords[term] {
		return true
	}
	return false
}

func candidateTermDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for term := range doc.Terms {
			out[term]++
		}
	}
	return out
}

func candidateTagDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for _, tag := range doc.Tags {
			out[tag]++
		}
	}
	return out
}

func candidateSourceDocumentFrequency(docs []*candidateDocument) map[string]int {
	out := map[string]int{}
	for _, doc := range docs {
		for _, source := range doc.Sources {
			out[source]++
		}
	}
	return out
}

func pagePairCandidates(ctx context.Context, db *sql.DB, docs []*candidateDocument, termDF, tagDF, sourceDF map[string]int) ([]Candidate, error) {
	var candidates []Candidate
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			candidate, ok, err := pagePairCandidate(ctx, db, docs[i], docs[j], len(docs), termDF, tagDF, sourceDF)
			if err != nil {
				return nil, err
			}
			if ok {
				candidates = append(candidates, candidate)
			}
		}
	}
	return candidates, nil
}

func pagePairCandidate(ctx context.Context, db *sql.DB, left, right *candidateDocument, totalWikiDocs int, termDF, tagDF, sourceDF map[string]int) (Candidate, bool, error) {
	sharedTags := intersectSortedStrings(left.Tags, right.Tags)
	sharedSources := intersectSortedStrings(left.Sources, right.Sources)
	lexical := lexicalOverlap(left.Terms, right.Terms, termDF, totalWikiDocs)
	linked := documentsLinked(left, right)

	tagScore := sharedTagScore(sharedTags, tagDF, totalWikiDocs)
	sourceScore := sharedSourceScore(sharedSources, sourceDF, totalWikiDocs)
	lexicalScore := math.Min(0.30, lexical.Score*1.5)
	baseScore := tagScore + sourceScore + lexicalScore
	sameEntity := candidateSamePrimaryEntity(left, right)

	hasStrongSignal := len(sharedSources) > 0 || len(sharedTags) >= 2 || lexical.Score >= candidateLexicalPairThreshold || (len(sharedTags) > 0 && lexical.Score >= candidateLexicalReasonThreshold)
	if !hasStrongSignal || baseScore < candidateMinimumPairScore {
		return Candidate{}, false, nil
	}

	reasons := make([]CandidateReason, 0, len(sharedTags)+len(sharedSources)+3)
	for _, tag := range sharedTags {
		reasons = append(reasons, CandidateReason{Code: ReasonSharedTag, Value: tag})
	}
	for _, source := range sharedSources {
		reasons = append(reasons, CandidateReason{Code: ReasonSharedSource, Value: source})
	}
	if lexical.Score >= candidateLexicalReasonThreshold && len(lexical.Terms) > 0 {
		reasons = append(reasons, CandidateReason{Code: ReasonLexicalOverlap, Value: strings.Join(lexical.Terms, ",")})
	}

	score := baseScore
	if sameEntity {
		score += 0.06
	} else if len(sharedSources) == 0 {
		// Cross-entity pairs without shared source evidence are often useful as
		// analogies, but are weaker link/consolidation candidates.
		score -= 0.25
	}
	if !linked {
		score += 0.12
		reasons = append(reasons, CandidateReason{Code: ReasonNotLinked})
	} else {
		score *= 0.75
	}
	if reason, ok, err := olderRelevantPageReason(ctx, db, left, right, hasStrongSignal); err != nil {
		return Candidate{}, false, err
	} else if ok {
		score += 0.10
		reasons = append(reasons, reason)
	}

	candidateType, keep := pagePairCandidateType(linked, sameEntity, sharedTags, sharedSources, tagDF, sourceDF, lexical.Score)
	if !keep {
		return Candidate{}, false, nil
	}
	score = calibrateCandidateScore(candidateType, score)
	candidate := Candidate{
		Type:              candidateType,
		Confidence:        confidenceForCandidate(candidateType, score),
		Score:             score,
		Pages:             []string{left.Path, right.Path},
		Sources:           sharedSources,
		Reasons:           reasons,
		SuggestedQueries:  pairSuggestedQueries(left, right, sharedTags, sharedSources, lexical.Terms, termDF, totalWikiDocs),
		ReviewInstruction: reviewInstruction(candidateType),
	}
	return candidate, true, nil
}

func documentsLinked(left, right *candidateDocument) bool {
	if _, ok := left.Outgoing[right.Path]; ok {
		return true
	}
	if _, ok := right.Outgoing[left.Path]; ok {
		return true
	}
	return false
}

func candidateSamePrimaryEntity(left, right *candidateDocument) bool {
	leftEntity := firstPathToken(left.Path)
	rightEntity := firstPathToken(right.Path)
	return leftEntity != "" && leftEntity == rightEntity
}

func pagePairCandidateType(linked, sameEntity bool, sharedTags, sharedSources []string, tagDF, sourceDF map[string]int, lexicalScore float64) (string, bool) {
	rareSignals := rareSharedSignalCount(sharedTags, sharedSources, tagDF, sourceDF)
	strongDuplicateSignal := sameEntity && rareSignals >= 2 && lexicalScore >= 0.22
	if strongDuplicateSignal {
		return CandidateTypePossibleDuplicate, true
	}
	if linked {
		// Already-linked related pages are usually not actionable unless the
		// deterministic evidence is strong enough to suggest consolidation.
		return "", false
	}
	return CandidateTypeMissingLink, true
}

func rareSharedSignalCount(sharedTags, sharedSources []string, tagDF, sourceDF map[string]int) int {
	count := 0
	for _, tag := range sharedTags {
		if tagDF[tag] <= 3 {
			count++
		}
	}
	for _, source := range sharedSources {
		if sourceDF[source] <= 3 {
			count++
		}
	}
	return count
}

func sharedTagScore(tags []string, df map[string]int, totalDocs int) float64 {
	var score float64
	for _, tag := range tags {
		freq := df[tag]
		if freq <= 0 {
			freq = 1
		}
		rarity := 1 - float64(freq-1)/math.Max(1, float64(totalDocs-1))
		score += 0.08 + 0.10*rarity
	}
	return math.Min(0.42, score)
}

func sharedSourceScore(sources []string, df map[string]int, totalDocs int) float64 {
	var score float64
	for _, source := range sources {
		freq := df[source]
		if freq <= 0 {
			freq = 1
		}
		rarity := 1 - float64(freq-1)/math.Max(1, float64(totalDocs-1))
		// A shared one-off source is a strong relationship fact. A broad compact
		// source cited by many pages is weaker and should not dominate ranking.
		score += 0.05 + 0.20*rarity*rarity
	}
	return math.Min(0.42, score)
}

func lexicalOverlap(left, right map[string]int, df map[string]int, totalDocs int) lexicalOverlapResult {
	if len(left) == 0 || len(right) == 0 {
		return lexicalOverlapResult{}
	}
	seen := map[string]struct{}{}
	for term := range left {
		seen[term] = struct{}{}
	}
	for term := range right {
		seen[term] = struct{}{}
	}

	var intersection, union float64
	var contributions []overlapContribution
	for term := range seen {
		leftWeight := candidateTermWeight(left[term], df[term], totalDocs)
		rightWeight := candidateTermWeight(right[term], df[term], totalDocs)
		if leftWeight == 0 && rightWeight == 0 {
			continue
		}
		shared := math.Min(leftWeight, rightWeight)
		intersection += shared
		union += math.Max(leftWeight, rightWeight)
		if shared > 0 {
			contributions = append(contributions, overlapContribution{Term: term, Contribution: shared})
		}
	}
	if union == 0 {
		return lexicalOverlapResult{}
	}
	sort.Slice(contributions, func(i, j int) bool {
		if contributions[i].Contribution != contributions[j].Contribution {
			return contributions[i].Contribution > contributions[j].Contribution
		}
		return contributions[i].Term < contributions[j].Term
	})
	terms := make([]string, 0, minInt(5, len(contributions)))
	for _, contribution := range contributions {
		terms = append(terms, contribution.Term)
		if len(terms) == 5 {
			break
		}
	}
	return lexicalOverlapResult{Score: intersection / union, Terms: terms}
}

func candidateTermWeight(count, df, totalDocs int) float64 {
	if count <= 0 {
		return 0
	}
	if df <= 0 {
		df = 1
	}
	idf := math.Log(float64(totalDocs+1)/float64(df+1)) + 1
	return (1 + math.Log(float64(count))) * idf
}

func olderRelevantPageReason(ctx context.Context, db *sql.DB, left, right *candidateDocument, alreadyRelated bool) (CandidateReason, bool, error) {
	if !alreadyRelated {
		return CandidateReason{}, false, nil
	}
	leftDate, leftOK := parseCandidateModifiedDate(left.ModifiedDate)
	rightDate, rightOK := parseCandidateModifiedDate(right.ModifiedDate)
	if !leftOK || !rightOK || leftDate.Equal(rightDate) {
		return CandidateReason{}, false, nil
	}
	older, newer := left, right
	olderDate, newerDate := leftDate, rightDate
	if rightDate.Before(leftDate) {
		older, newer = right, left
		olderDate, newerDate = rightDate, leftDate
	}
	if newerDate.Sub(olderDate) < staleRiskMinimumDays*24*time.Hour {
		return CandidateReason{}, false, nil
	}
	relevant, err := candidateBM25Relevant(ctx, db, older.Path, newer)
	if err != nil {
		return CandidateReason{}, false, err
	}
	if !relevant {
		return CandidateReason{}, false, nil
	}
	return CandidateReason{Code: ReasonOlderRelevantPage, Value: older.Path}, true, nil
}

func parseCandidateModifiedDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(modifiedDateLayout, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func candidateBM25Relevant(ctx context.Context, db *sql.DB, targetPath string, queryDoc *candidateDocument) (bool, error) {
	terms := queryTermsForDocument(queryDoc, nil, 0, 5)
	if len(terms) == 0 {
		return false, nil
	}
	sanitized, err := sanitizeQuery(strings.Join(terms, " "))
	if err != nil {
		return false, nil
	}
	var score float64
	err = db.QueryRowContext(ctx, `SELECT bm25(sections_fts, 5.0, 3.0, 4.0, 2.0, 2.0, 1.5, 3.0, 1.0) AS score
		FROM sections_fts
		JOIN sections s ON s.rowid = sections_fts.rowid
		WHERE sections_fts MATCH ? AND s.path = ?
		ORDER BY score ASC
		LIMIT 1`, sanitized.matchOR(), targetPath).Scan(&score)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check older page relevance for %s: %w", targetPath, err)
	}
	return true, nil
}

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

func calibrateCandidateScore(candidateType string, score float64) float64 {
	if candidateType == CandidateTypeMissingLink {
		if score < 0 {
			score = 0
		}
		// Missing-link scores should read as review priority, not certainty
		// that a link must be added. Preserve ordering while keeping the range
		// below duplicate/consolidation-level confidence.
		return clampCandidateScore(0.40 + 0.35*(1-math.Exp(-score)))
	}
	return clampCandidateScore(score)
}

func confidenceForCandidate(candidateType string, score float64) string {
	confidence := confidenceForScore(score)
	if candidateType == CandidateTypeMissingLink && confidence == CandidateConfidenceHigh {
		return CandidateConfidenceMedium
	}
	return confidence
}

func confidenceForScore(score float64) string {
	switch {
	case score >= 0.75:
		return CandidateConfidenceHigh
	case score >= 0.45:
		return CandidateConfidenceMedium
	default:
		return CandidateConfidenceLow
	}
}

func clampCandidateScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return math.Round(score*1000) / 1000
}

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

var candidateStopwords = map[string]bool{
	"anchor": true, "anchors": true, "available": true, "body": true, "combined": true,
	"compact": true, "doc": true, "docs": true, "file": true, "files": true,
	"generated": true, "generic": true, "index": true, "markdown": true, "may": true,
	"must": true, "page": true, "pages": true, "section": true, "sections": true,
	"source": true, "sources": true, "wiki": true,
}

package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type candidatePairKey struct {
	left  int
	right int
}

func pagePairCandidates(ctx context.Context, db *sql.DB, docs []*candidateDocument, termDF, tagDF, sourceDF map[string]int) ([]Candidate, error) {
	pairs := candidatePairKeys(docs, termDF)
	candidates := make([]Candidate, 0, len(pairs))
	for _, pair := range pairs {
		candidate, ok, err := pagePairCandidate(ctx, db, docs[pair.left], docs[pair.right], len(docs), termDF, tagDF, sourceDF)
		if err != nil {
			return nil, err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	sortCandidates(candidates)
	return enrichOlderRelevantPairCandidates(ctx, db, candidates, candidateDocsByPath(docs))
}

func candidatePairKeys(docs []*candidateDocument, termDF map[string]int) []candidatePairKey {
	seen := map[candidatePairKey]struct{}{}
	addPairs := func(groups map[string][]int) {
		for _, indexes := range groups {
			for i := 0; i < len(indexes); i++ {
				for j := i + 1; j < len(indexes); j++ {
					key := candidatePairKey{left: indexes[i], right: indexes[j]}
					seen[key] = struct{}{}
				}
			}
		}
	}

	byTag := map[string][]int{}
	bySource := map[string][]int{}
	byTerm := map[string][]int{}
	for i, doc := range docs {
		for _, tag := range doc.Tags {
			byTag[tag] = append(byTag[tag], i)
		}
		for _, source := range doc.Sources {
			bySource[source] = append(bySource[source], i)
		}
		for _, term := range candidatePairTerms(doc, termDF, len(docs)) {
			byTerm[term] = append(byTerm[term], i)
		}
	}
	addPairs(byTag)
	addPairs(bySource)
	addPairs(byTerm)

	pairs := make([]candidatePairKey, 0, len(seen))
	for pair := range seen {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		leftI, rightI := docs[pairs[i].left], docs[pairs[i].right]
		leftJ, rightJ := docs[pairs[j].left], docs[pairs[j].right]
		if leftI.Path != leftJ.Path {
			return leftI.Path < leftJ.Path
		}
		return rightI.Path < rightJ.Path
	})
	return pairs
}

func candidatePairTerms(doc *candidateDocument, termDF map[string]int, totalDocs int) []string {
	terms := topDocumentTerms(doc, termDF, totalDocs, 12)
	out := terms[:0]
	for _, term := range terms {
		if candidatePairTermAllowed(termDF[term], totalDocs) {
			out = append(out, term)
		}
	}
	return out
}

func candidatePairTermAllowed(df, totalDocs int) bool {
	if df <= 1 {
		return false
	}
	if df <= 8 {
		return true
	}
	return totalDocs > 0 && df*3 <= totalDocs
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

func candidateDocsByPath(docs []*candidateDocument) map[string]*candidateDocument {
	out := make(map[string]*candidateDocument, len(docs))
	for _, doc := range docs {
		out[doc.Path] = doc
	}
	return out
}

func enrichOlderRelevantPairCandidates(ctx context.Context, db *sql.DB, candidates []Candidate, docsByPath map[string]*candidateDocument) ([]Candidate, error) {
	limit := minInt(len(candidates), candidateOlderRelevanceTopPairs)
	for i := 0; i < limit; i++ {
		candidate := &candidates[i]
		if len(candidate.Pages) != 2 || candidateHasReason(*candidate, ReasonOlderRelevantPage) {
			continue
		}
		left := docsByPath[candidate.Pages[0]]
		right := docsByPath[candidate.Pages[1]]
		if left == nil || right == nil {
			continue
		}
		reason, ok, err := olderRelevantPageReason(ctx, db, left, right, true)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		candidate.Score = clampCandidateScore(candidate.Score + 0.10)
		candidate.Confidence = confidenceForCandidate(candidate.Type, candidate.Score)
		candidate.Reasons = append(candidate.Reasons, reason)
	}
	return candidates, nil
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

package searchindex

import (
	"math"
	"sort"
)

func sharedTagScore(tags []string, df map[string]int, totalDocs int) float64 {
	var score float64
	for _, tag := range tags {
		freq := df[tag]
		if freq <= 0 {
			freq = 1
		}
		rarity := 1 - float64(freq-1)/math.Max(1, float64(totalDocs-1))
		// Quadratic rarity: saturated tags (>50% of pages) contribute near-zero.
		// Rare tags still contribute up to 0.18 per tag (0.02 + 0.16).
		score += 0.02 + 0.16*rarity*rarity
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

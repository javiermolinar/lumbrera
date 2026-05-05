package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
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

func runSearch(ctx context.Context, db *sql.DB, match string, opts SearchOptions) ([]SearchResult, error) {
	where := []string{"sections_fts MATCH ?"}
	args := []any{match}
	if opts.Kind != KindAll {
		where = append(where, "s.kind = ?")
		args = append(args, opts.Kind)
	}
	if opts.PathPrefix != "" {
		where = append(where, `s.path LIKE ? ESCAPE '\'`)
		args = append(args, escapeLikePrefix(opts.PathPrefix)+"%")
	}
	args = append(args, searchCandidateLimit(opts.Limit))

	query := fmt.Sprintf(`WITH matches AS (
	SELECT
		s.document_id,
		s.id AS section_id,
		s.path,
		s.anchor,
		s.kind,
		s.title,
		s.heading,
		s.summary,
		s.tags_json,
		s.sources_json,
		s.links_json,
		s.ordinal,
		snippet(sections_fts, -1, '<<', '>>', '...', 16) AS snippet,
		bm25(sections_fts, 5.0, 3.0, 4.0, 2.0, 2.0, 1.5, 3.0, 1.0) AS lexical_score,
		CASE lower(COALESCE(s.heading, ''))
			WHEN 'sources' THEN 4.0
			WHEN 'related pages' THEN 4.0
			WHEN 'status' THEN 3.5
			WHEN 'table of contents' THEN 3.5
			WHEN 'contents' THEN 3.5
			WHEN 'navigation' THEN 3.5
			ELSE 0
		END AS heading_penalty
	FROM sections_fts
	JOIN sections s ON s.rowid = sections_fts.rowid
	WHERE %s
)
SELECT
	document_id,
	section_id,
	path,
	COALESCE(anchor, ''),
	kind,
	title,
	COALESCE(heading, ''),
	summary,
	tags_json,
	sources_json,
	links_json,
	snippet,
	lexical_score - CASE kind WHEN 'wiki' THEN abs(lexical_score) * %.2f ELSE 0 END + heading_penalty AS score,
	lexical_score
FROM matches
ORDER BY
	score ASC,
	lexical_score ASC,
	CASE kind WHEN 'wiki' THEN 0 ELSE 1 END,
	path ASC,
	ordinal ASC,
	section_id ASC
LIMIT ?`, strings.Join(where, " AND "), wikiScoreBoost)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search index query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var tagsJSON, sourcesJSON, linksJSON string
		if err := rows.Scan(
			&result.DocumentID,
			&result.SectionID,
			&result.Path,
			&result.Anchor,
			&result.Kind,
			&result.Title,
			&result.Heading,
			&result.Summary,
			&tagsJSON,
			&sourcesJSON,
			&linksJSON,
			&result.Snippet,
			&result.Score,
			&result.LexicalScore,
		); err != nil {
			return nil, fmt.Errorf("scan search index result: %w", err)
		}
		var err error
		if result.Tags, err = decodeStringArray(tagsJSON); err != nil {
			return nil, fmt.Errorf("decode tags for %s: %w", result.SectionID, err)
		}
		if result.Sources, err = decodeStringArray(sourcesJSON); err != nil {
			return nil, fmt.Errorf("decode sources for %s: %w", result.SectionID, err)
		}
		if result.Links, err = decodeStringArray(linksJSON); err != nil {
			return nil, fmt.Errorf("decode links for %s: %w", result.SectionID, err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search index results: %w", err)
	}
	return results, nil
}

func sanitizeQuery(query string) (sanitizedQuery, error) {
	parts := parseSearchParts(query)
	atoms := make([]string, 0, len(parts))
	terms := make([]string, 0, len(parts))
	seenTerms := map[string]struct{}{}
	for _, part := range parts {
		var text string
		if part.quoted {
			text = normalizeFTSText(part.text)
		} else {
			text = normalizeFTSText(part.text)
			if text != "" && !strings.Contains(text, " ") && searchStopwords[text] {
				continue
			}
		}
		if text == "" {
			continue
		}
		atoms = append(atoms, quoteFTSAtom(text))
		for _, term := range strings.Fields(text) {
			if searchStopwords[term] {
				continue
			}
			if _, ok := seenTerms[term]; ok {
				continue
			}
			seenTerms[term] = struct{}{}
			terms = append(terms, term)
		}
	}
	if len(atoms) == 0 {
		return sanitizedQuery{}, errors.New("search query has no searchable terms")
	}
	return sanitizedQuery{atoms: atoms, terms: terms}, nil
}

type searchPart struct {
	text   string
	quoted bool
}

func parseSearchParts(query string) []searchPart {
	var parts []searchPart
	var b strings.Builder
	inQuote := false
	for _, r := range query {
		switch {
		case r == '"':
			if inQuote {
				parts = appendPart(parts, b.String(), true)
				b.Reset()
				inQuote = false
				continue
			}
			parts = appendPart(parts, b.String(), false)
			b.Reset()
			inQuote = true
		case !inQuote && unicode.IsSpace(r):
			parts = appendPart(parts, b.String(), false)
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	parts = appendPart(parts, b.String(), inQuote)
	return parts
}

func appendPart(parts []searchPart, text string, quoted bool) []searchPart {
	text = strings.TrimSpace(text)
	if text == "" {
		return parts
	}
	return append(parts, searchPart{text: text, quoted: quoted})
}

func normalizeFTSText(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if b.Len() > 0 && !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func quoteFTSAtom(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

func (q sanitizedQuery) matchAND() string {
	return strings.Join(q.atoms, " AND ")
}

func (q sanitizedQuery) matchOR() string {
	return strings.Join(q.atoms, " OR ")
}

func normalizeSearchOptions(opts SearchOptions) (SearchOptions, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultSearchLimit
	}
	if opts.Limit > MaxSearchLimit {
		opts.Limit = MaxSearchLimit
	}
	if opts.Kind == "" {
		opts.Kind = KindAll
	}
	if opts.Kind != KindAll && opts.Kind != KindWiki && opts.Kind != KindSource {
		return SearchOptions{}, fmt.Errorf("invalid search kind %q", opts.Kind)
	}
	prefix, err := normalizePathPrefix(opts.PathPrefix)
	if err != nil {
		return SearchOptions{}, err
	}
	opts.PathPrefix = prefix
	return opts, nil
}

func normalizePathPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(filepath.ToSlash(prefix))
	if prefix == "" {
		return "", nil
	}
	if filepath.IsAbs(prefix) || path.IsAbs(prefix) {
		return "", fmt.Errorf("search path prefix %q must be repo-relative", prefix)
	}
	if strings.HasPrefix(prefix, "./") {
		prefix = strings.TrimPrefix(prefix, "./")
	}
	if hasParentSegment(prefix) {
		return "", fmt.Errorf("search path prefix %q must not contain ..", prefix)
	}
	clean := path.Clean(prefix)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

func hasParentSegment(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func escapeLikePrefix(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '%', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func decodeStringArray(value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []string{}, nil
	}
	return out, nil
}

func rerankCandidates(results []SearchResult, queryTerms []string) []SearchResult {
	if len(results) == 0 {
		return []SearchResult{}
	}
	out := append([]SearchResult(nil), results...)
	productTerms := detectedProductTerms(out, queryTerms)
	if len(productTerms) == 0 {
		return out
	}
	productSet := map[string]struct{}{}
	for _, term := range productTerms {
		productSet[term] = struct{}{}
	}
	for i := range out {
		alignment := resultEntityAlignment(out[i], productSet)
		switch alignment {
		case entityAlignmentMatch:
			out[i].Score -= entityMatchBoost
		case entityAlignmentMismatch:
			out[i].Score += entityMismatchPenalty
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return betterResult(out[i], out[j])
	})
	return out
}

func searchCandidateLimit(limit int) int {
	candidateLimit := limit * 5
	if candidateLimit < searchCandidateLimitMinimum {
		candidateLimit = searchCandidateLimitMinimum
	}
	if candidateLimit > searchCandidateLimitMaximum {
		candidateLimit = searchCandidateLimitMaximum
	}
	return candidateLimit
}

func selectSearchResults(candidates []SearchResult, limit int, recommendedSections []RecommendedSection) []SearchResult {
	if len(candidates) == 0 || limit <= 0 {
		return []SearchResult{}
	}
	countsByDocument := map[string]int{}
	seenSections := map[string]struct{}{}
	out := make([]SearchResult, 0, minInt(limit, len(candidates)))
	bySectionID := make(map[string]SearchResult, len(candidates))
	for _, result := range candidates {
		bySectionID[result.SectionID] = result
	}
	for _, section := range recommendedSections {
		result, ok := bySectionID[section.SectionID]
		if !ok {
			continue
		}
		out = appendSearchResult(out, countsByDocument, seenSections, result)
		if len(out) == limit {
			return out
		}
	}
	for _, result := range candidates {
		out = appendSearchResult(out, countsByDocument, seenSections, result)
		if len(out) == limit {
			break
		}
	}
	return out
}

func appendSearchResult(out []SearchResult, countsByDocument map[string]int, seenSections map[string]struct{}, result SearchResult) []SearchResult {
	if _, ok := seenSections[result.SectionID]; ok {
		return out
	}
	key := result.DocumentID
	if key == "" {
		key = result.Path
	}
	if countsByDocument[key] >= maxSectionsPerDocument {
		return out
	}
	countsByDocument[key]++
	seenSections[result.SectionID] = struct{}{}
	return append(out, result)
}

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

func resultHasProductTerm(result SearchResult, term string) bool {
	if resultHasEntityTerm(result, term) || stringSliceContains(result.Tags, term) {
		return true
	}
	return false
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

var searchStopwords = map[string]bool{
	"a": true, "about": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "between": true, "by": true, "can": true,
	"compare": true, "comparison": true, "difference": true, "differences": true,
	"do": true, "does": true, "explain": true, "find": true, "for": true,
	"from": true, "how": true, "i": true, "in": true, "information": true,
	"is": true, "it": true, "of": true, "on": true, "or": true, "read": true,
	"relationship": true, "should": true, "that": true, "the": true, "this": true,
	"to": true, "understand": true, "versus": true, "vs": true, "was": true,
	"what": true, "when": true, "where": true, "which": true, "who": true,
	"why": true, "with": true,
}

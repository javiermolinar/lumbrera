package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	KindAll             = "all"
	DefaultSearchLimit  = 5
	MaxSearchLimit      = 20
	QueryModeAND        = "and"
	QueryModeORFallback = "or_fallback"
	wikiScoreBoost      = 0.15
)

type SearchOptions struct {
	Limit      int
	Kind       string
	PathPrefix string
}

type SearchResponse struct {
	Query                string
	QueryMode            string
	Results              []SearchResult
	RecommendedReadOrder []string
	StopRule             string
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
	if len(results) == 0 && len(sanitized.atoms) > 1 {
		results, err = runSearch(ctx, db, sanitized.matchOR(), opts)
		if err != nil {
			return SearchResponse{}, err
		}
		mode = QueryModeORFallback
	}

	recommendedReadOrder, stopRule := recommendedReadOrder(results, opts)
	return SearchResponse{
		Query:                query,
		QueryMode:            mode,
		Results:              results,
		RecommendedReadOrder: recommendedReadOrder,
		StopRule:             stopRule,
	}, nil
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
	args = append(args, opts.Limit)

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
			WHEN 'sources' THEN 2.0
			WHEN 'related pages' THEN 1.5
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
	}
	if len(atoms) == 0 {
		return sanitizedQuery{}, errors.New("search query has no searchable terms")
	}
	return sanitizedQuery{atoms: atoms}, nil
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

func recommendedReadOrder(results []SearchResult, opts SearchOptions) ([]string, string) {
	if len(results) == 0 {
		return []string{}, "No search results. Run one refined search with different terms before broader exploration."
	}
	if opts.Kind == KindSource || !hasKind(results, KindWiki) {
		return dedupeBestPaths(results, KindSource), "Read these source results directly. Do not scan the repo unless they are insufficient."
	}
	return dedupeBestPaths(results, KindWiki), "Read the top 3 wiki pages first. Do not scan the repo unless those are insufficient."
}

func hasKind(results []SearchResult, kind string) bool {
	for _, result := range results {
		if result.Kind == kind {
			return true
		}
	}
	return false
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

var searchStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "can": true, "do": true, "does": true, "for": true,
	"from": true, "how": true, "i": true, "in": true, "is": true, "it": true,
	"of": true, "on": true, "or": true, "should": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "what": true, "when": true, "where": true,
	"which": true, "who": true, "why": true, "with": true,
}

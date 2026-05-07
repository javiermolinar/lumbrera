package searchindex

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

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
	for _, tag := range opts.Tags {
		where = append(where, `EXISTS (SELECT 1 FROM document_tags dt WHERE dt.document_id = s.document_id AND dt.tag = ?)`)
		args = append(args, tag)
	}
	for _, source := range opts.Sources {
		where = append(where, `EXISTS (SELECT 1 FROM document_citations dc WHERE dc.document_id = s.document_id AND dc.source_path = ?)`)
		args = append(args, source)
	}
	if len(opts.Tiers) > 0 {
		placeholders := make([]string, len(opts.Tiers))
		for i, tier := range opts.Tiers {
			placeholders[i] = "?"
			args = append(args, tier)
		}
		where = append(where, fmt.Sprintf(`d.tier IN (%s)`, strings.Join(placeholders, ", ")))
	}
	args = append(args, searchCandidateLimit(opts.Limit))

	query := fmt.Sprintf(`WITH matches AS (
	SELECT
		s.document_id,
		s.id AS section_id,
		s.path,
		s.anchor,
		s.kind,
		d.tier,
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
		END AS heading_penalty,
		CASE d.tier
			WHEN 'design' THEN 0.45
			WHEN 'reference' THEN 0.60
			ELSE 0.00
		END AS tier_penalty
	FROM sections_fts
	JOIN sections s ON s.rowid = sections_fts.rowid
	JOIN documents d ON d.id = s.document_id
	WHERE %s
)
SELECT
	document_id,
	section_id,
	path,
	COALESCE(anchor, ''),
	kind,
	tier,
	title,
	COALESCE(heading, ''),
	summary,
	tags_json,
	sources_json,
	links_json,
	snippet,
	(lexical_score - CASE kind WHEN 'wiki' THEN abs(lexical_score) * %.2f ELSE 0 END + heading_penalty) + (abs(lexical_score) * tier_penalty) AS score,
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
			&result.Tier,
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

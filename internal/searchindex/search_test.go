package searchindex

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestSearchANDQueryAndWikiBoost(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "tempo downscale", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if response.QueryMode != QueryModeAND {
		t.Fatalf("query mode = %q, want %q", response.QueryMode, QueryModeAND)
	}
	if len(response.Results) < 2 {
		t.Fatalf("result count = %d, want at least 2: %#v", len(response.Results), response.Results)
	}
	if response.Results[0].Path != "wiki/tempo-downscale.md" || response.Results[0].Kind != KindWiki {
		t.Fatalf("top result = %#v, want wiki downscale page", response.Results[0])
	}
	if response.Results[0].Score >= response.Results[0].LexicalScore {
		t.Fatalf("wiki boost not applied: score=%v lexical=%v", response.Results[0].Score, response.Results[0].LexicalScore)
	}
	if !strings.Contains(response.Results[0].Snippet, "<<") {
		t.Fatalf("snippet has no highlight markers: %q", response.Results[0].Snippet)
	}
	assertStringSlicesEqual(t, response.Results[0].Tags, []string{"tempo"}, "tags")
	assertStringSlicesEqual(t, response.Results[0].Sources, []string{"sources/tempo.md"}, "sources")
	assertStringSlicesEqual(t, response.RecommendedReadOrder, []string{"wiki/tempo-downscale.md", "wiki/tempo-related.md"}, "recommended read order")
	if response.StopRule != "Read the top 3 wiki pages first. Do not scan the repo unless those are insufficient." {
		t.Fatalf("stop rule = %q", response.StopRule)
	}
}

func TestSearchORFallback(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "tempo nonexistent", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if response.QueryMode != QueryModeORFallback {
		t.Fatalf("query mode = %q, want %q", response.QueryMode, QueryModeORFallback)
	}
	if len(response.Results) == 0 {
		t.Fatal("OR fallback returned no results")
	}
	if response.Results[0].Kind != KindWiki || !strings.HasPrefix(response.Results[0].Path, "wiki/tempo") {
		t.Fatalf("top fallback result = %#v, want tempo wiki result", response.Results[0])
	}
}

func TestSearchStopwordsAndBooleanWordsAreSafe(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "what is the tempo AND downscale?", SearchOptions{})
	if err != nil {
		t.Fatalf("search with boolean-looking input: %v", err)
	}
	if len(response.Results) == 0 {
		t.Fatal("search with stopwords and boolean-looking input returned no results")
	}

	if _, err := Search(context.Background(), db, "what is the", SearchOptions{}); err == nil {
		t.Fatal("stopword-only search succeeded, want no searchable terms error")
	}
}

func TestSearchQuotedPhrase(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, `"tenant limits"`, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search quoted phrase: %v", err)
	}
	if response.QueryMode != QueryModeAND {
		t.Fatalf("query mode = %q, want AND", response.QueryMode)
	}
	if len(response.Results) == 0 || response.Results[0].Path != "wiki/mimir-limits.md" {
		t.Fatalf("quoted phrase results = %#v, want mimir limits top", response.Results)
	}

	response, err = Search(context.Background(), db, `"limits tenant"`, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search reversed quoted phrase: %v", err)
	}
	if len(response.Results) != 0 {
		t.Fatalf("reversed exact phrase returned results: %#v", response.Results)
	}
}

func TestSearchFiltersAndLimit(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "tempo downscale", SearchOptions{Kind: KindSource, Limit: 10})
	if err != nil {
		t.Fatalf("source-filtered search: %v", err)
	}
	if len(response.Results) == 0 {
		t.Fatal("source-filtered search returned no results")
	}
	for _, result := range response.Results {
		if result.Kind != KindSource {
			t.Fatalf("source-filtered result has kind %q: %#v", result.Kind, result)
		}
	}

	response, err = Search(context.Background(), db, "tempo downscale", SearchOptions{PathPrefix: "wiki/", Limit: 10})
	if err != nil {
		t.Fatalf("path-filtered search: %v", err)
	}
	if len(response.Results) == 0 {
		t.Fatal("path-filtered search returned no results")
	}
	for _, result := range response.Results {
		if !strings.HasPrefix(result.Path, "wiki/") {
			t.Fatalf("path-filtered result path = %q", result.Path)
		}
	}

	response, err = Search(context.Background(), db, "tempo", SearchOptions{Limit: 100})
	if err != nil {
		t.Fatalf("capped limit search: %v", err)
	}
	if len(response.Results) > MaxSearchLimit {
		t.Fatalf("result count = %d, want capped at %d", len(response.Results), MaxSearchLimit)
	}
}

func TestSearchPenalizesGeneratedNavigationSections(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{{
		ID:          "doc_generated_sections",
		Path:        "wiki/generated-sections.md",
		Kind:        KindWiki,
		Title:       "Generated sections",
		Summary:     "generatedunique summary",
		TagsJSON:    `["generated"]`,
		SourcesJSON: `[]`,
		LinksJSON:   `[]`,
		TagsText:    "generated",
		Hash:        "hash-generated",
		SizeBytes:   100,
	}}
	sections := []Section{
		{DocumentID: "doc_generated_sections", Ordinal: 1, Heading: "Generated sections", Anchor: "generated-sections", Level: 1, Body: "body"},
		{DocumentID: "doc_generated_sections", Ordinal: 2, Heading: "Sources", Anchor: "sources", Level: 2, Body: "body"},
		{DocumentID: "doc_generated_sections", Ordinal: 3, Heading: "Related pages", Anchor: "related-pages", Level: 2, Body: "body"},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "fixture"}); err != nil {
		t.Fatalf("rebuild generated sections fixture: %v", err)
	}

	response, err := Search(context.Background(), db, "generatedunique", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("search generated sections fixture: %v", err)
	}
	if len(response.Results) != 3 {
		t.Fatalf("result count = %d, want 3: %#v", len(response.Results), response.Results)
	}
	if response.Results[0].Heading == "Sources" || response.Results[0].Heading == "Related pages" {
		t.Fatalf("generated navigation section ranked first: %#v", response.Results)
	}
}

func TestSearchRecommendedReadOrderForSourceOnlyResults(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "tempo downscale", SearchOptions{Kind: KindSource, Limit: 10})
	if err != nil {
		t.Fatalf("source search: %v", err)
	}
	assertStringSlicesEqual(t, response.RecommendedReadOrder, []string{"sources/tempo.md"}, "source recommended read order")
	if response.StopRule != "Read these source results directly. Do not scan the repo unless they are insufficient." {
		t.Fatalf("source stop rule = %q", response.StopRule)
	}
}

func TestSearchRecommendedReadOrderForNoResults(t *testing.T) {
	db := searchFixtureDB(t)

	response, err := Search(context.Background(), db, "absentterm", SearchOptions{})
	if err != nil {
		t.Fatalf("no-result search: %v", err)
	}
	assertStringSlicesEqual(t, response.RecommendedReadOrder, []string{}, "empty recommended read order")
	if response.StopRule != "No search results. Run one refined search with different terms before broader exploration." {
		t.Fatalf("empty stop rule = %q", response.StopRule)
	}
}

func TestSearchRejectsInvalidOptions(t *testing.T) {
	db := searchFixtureDB(t)

	if _, err := Search(context.Background(), db, "tempo", SearchOptions{Kind: "note"}); err == nil {
		t.Fatal("invalid kind search succeeded, want error")
	}
	if _, err := Search(context.Background(), db, "tempo", SearchOptions{PathPrefix: "../wiki"}); err == nil {
		t.Fatal("invalid path prefix search succeeded, want error")
	}
	if _, err := Search(context.Background(), db, "tempo", SearchOptions{PathPrefix: "wiki/../sources"}); err == nil {
		t.Fatal("path prefix with parent segment succeeded, want error")
	}
}

func TestSanitizeQuery(t *testing.T) {
	query, err := sanitizeQuery(`what is "per tenant" AND sources/azure_migration.md?`)
	if err != nil {
		t.Fatalf("sanitize query: %v", err)
	}
	wantAND := `"per tenant" AND "sources azure_migration md"`
	if query.matchAND() != wantAND {
		t.Fatalf("AND query = %q, want %q", query.matchAND(), wantAND)
	}
	wantOR := `"per tenant" OR "sources azure_migration md"`
	if query.matchOR() != wantOR {
		t.Fatalf("OR query = %q, want %q", query.matchOR(), wantOR)
	}
}

func searchFixtureDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	docs := []Document{
		{
			ID:          "doc_tempo_downscale",
			Path:        "wiki/tempo-downscale.md",
			Kind:        KindWiki,
			Title:       "Tempo downscale",
			Summary:     "Tempo downscale runbook.",
			TagsJSON:    `["tempo"]`,
			SourcesJSON: `["sources/tempo.md"]`,
			LinksJSON:   `["wiki/mimir-limits.md"]`,
			TagsText:    "tempo",
			SourcesText: "sources/tempo.md",
			LinksText:   "wiki/mimir-limits.md",
			Hash:        "hash-wiki-tempo",
			SizeBytes:   100,
		},
		{
			ID:          "doc_source_tempo",
			Path:        "sources/tempo.md",
			Kind:        KindSource,
			Title:       "Tempo source",
			Summary:     "",
			TagsJSON:    `[]`,
			SourcesJSON: `[]`,
			LinksJSON:   `[]`,
			Hash:        "hash-source-tempo",
			SizeBytes:   100,
		},
		{
			ID:          "doc_tempo_related",
			Path:        "wiki/tempo-related.md",
			Kind:        KindWiki,
			Title:       "Tempo related",
			Summary:     "Related tempo downscale notes.",
			TagsJSON:    `["tempo"]`,
			SourcesJSON: `[]`,
			LinksJSON:   `[]`,
			TagsText:    "tempo",
			Hash:        "hash-wiki-tempo-related",
			SizeBytes:   100,
		},
		{
			ID:          "doc_mimir_limits",
			Path:        "wiki/mimir-limits.md",
			Kind:        KindWiki,
			Title:       "Mimir tenant limits",
			Summary:     "Tenant limits for Mimir.",
			TagsJSON:    `["mimir","limits"]`,
			SourcesJSON: `[]`,
			LinksJSON:   `[]`,
			TagsText:    "limits mimir",
			Hash:        "hash-wiki-mimir",
			SizeBytes:   100,
		},
	}
	sections := []Section{
		{DocumentID: "doc_tempo_downscale", Ordinal: 1, Heading: "Tempo downscale", Anchor: "tempo-downscale", Level: 1, Body: "Tempo downscale procedure for ingesters."},
		{DocumentID: "doc_tempo_downscale", Ordinal: 2, Heading: "Rollback", Anchor: "rollback", Level: 2, Body: "Rollback after downscale."},
		{DocumentID: "doc_source_tempo", Ordinal: 1, Heading: "Tempo source", Anchor: "tempo-source", Level: 1, Body: "Tempo downscale raw evidence."},
		{DocumentID: "doc_tempo_related", Ordinal: 1, Heading: "Tempo related", Anchor: "tempo-related", Level: 1, Body: "Tempo downscale related notes."},
		{DocumentID: "doc_mimir_limits", Ordinal: 1, Heading: "Tenant limits", Anchor: "tenant-limits", Level: 1, Body: "Mimir tenant limits are configured per tenant."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "fixture"}); err != nil {
		t.Fatalf("rebuild search fixture: %v", err)
	}
	return db
}

func assertStringSlicesEqual(t *testing.T, got, want []string, name string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d: %#v", name, len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q; all=%#v", name, i, got[i], want[i], got)
		}
	}
}

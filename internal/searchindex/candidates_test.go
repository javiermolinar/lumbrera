package searchindex

import (
	"context"
	"testing"
)

func TestHealthCandidatesRanksPairBySharedFactsAndMissingLink(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateWikiDoc("doc_tempo_retention", "wiki/tempo-retention.md", "Tempo retention", "Tempo retention compaction guidance.", `["tempo","retention"]`, `["sources/tempo-raw.md"]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_tempo_retention_notes", "wiki/tempo-retention-notes.md", "Tempo retention notes", "Retention compaction notes for Tempo blocks.", `["tempo","retention"]`, `["sources/tempo-raw.md"]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_loki_cache", "wiki/loki-cache.md", "Loki cache", "Loki cache operations.", `["loki"]`, `[]`, `[]`, "2026-05-06"),
		candidateSourceDoc("source_tempo_raw", "sources/tempo-raw.md", "Tempo raw"),
	}
	sections := []Section{
		{DocumentID: "doc_tempo_retention", Ordinal: 1, Heading: "Tempo retention", Anchor: "tempo-retention", Level: 1, Body: "Tempo retention compaction keeps trace blocks in object storage."},
		{DocumentID: "doc_tempo_retention_notes", Ordinal: 1, Heading: "Tempo retention notes", Anchor: "tempo-retention-notes", Level: 1, Body: "Tempo retention compaction notes cover trace blocks and object storage."},
		{DocumentID: "doc_loki_cache", Ordinal: 1, Heading: "Loki cache", Anchor: "loki-cache", Level: 1, Body: "Loki cache operations are unrelated."},
		{DocumentID: "source_tempo_raw", Ordinal: 1, Heading: "Tempo raw", Anchor: "tempo-raw", Level: 1, Body: "Raw Tempo retention evidence."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-pair"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindDuplicates, Limit: 3})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) == 0 {
		t.Fatal("no duplicate candidates returned")
	}
	candidate := response.Candidates[0]
	if candidate.Type != CandidateTypePossibleDuplicate {
		t.Fatalf("candidate type = %q, want %q: %#v", candidate.Type, CandidateTypePossibleDuplicate, candidate)
	}
	assertCandidatePages(t, candidate, []string{"wiki/tempo-retention-notes.md", "wiki/tempo-retention.md"})
	assertCandidateReason(t, candidate, ReasonSharedTag, "retention")
	assertCandidateReason(t, candidate, ReasonSharedSource, "sources/tempo-raw.md")
	assertCandidateReason(t, candidate, ReasonNotLinked, "")
	assertCandidateReasonCode(t, candidate, ReasonLexicalOverlap)
	if len(candidate.SuggestedQueries) == 0 {
		t.Fatalf("candidate has no suggested queries: %#v", candidate)
	}
	if response.StopRule == "" {
		t.Fatal("response stop rule is empty")
	}
}

func TestHealthCandidatesFindsOrphanPagesAndUncitedSources(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateWikiDoc("doc_hub", "wiki/hub.md", "Hub", "Hub page.", `["hub"]`, `["sources/used.md"]`, `["wiki/leaf.md"]`, "2026-05-06"),
		candidateWikiDoc("doc_leaf", "wiki/leaf.md", "Leaf", "Leaf page.", `["leaf"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_orphan", "wiki/orphan.md", "Orphan", "Standalone orphan page.", `["orphan"]`, `[]`, `[]`, "2026-05-06"),
		candidateSourceDoc("source_used", "sources/used.md", "Used source"),
		candidateSourceDoc("source_unused", "sources/unused.md", "Unused source"),
	}
	sections := []Section{
		{DocumentID: "doc_hub", Ordinal: 1, Heading: "Hub", Anchor: "hub", Level: 1, Body: "Hub links to the leaf."},
		{DocumentID: "doc_leaf", Ordinal: 1, Heading: "Leaf", Anchor: "leaf", Level: 1, Body: "Leaf detail."},
		{DocumentID: "doc_orphan", Ordinal: 1, Heading: "Orphan", Anchor: "orphan", Level: 1, Body: "Standalone orphan detail."},
		{DocumentID: "source_used", Ordinal: 1, Heading: "Used source", Anchor: "used-source", Level: 1, Body: "Used source detail."},
		{DocumentID: "source_unused", Ordinal: 1, Heading: "Unused source", Anchor: "unused-source", Level: 1, Body: "Unused source detail."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-single"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	orphans, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindOrphans, Limit: 5})
	if err != nil {
		t.Fatalf("orphan candidates: %v", err)
	}
	if len(orphans.Candidates) == 0 {
		t.Fatal("no orphan candidates returned")
	}
	if orphans.Candidates[0].Type != CandidateTypeOrphanPage {
		t.Fatalf("top orphan candidate = %#v, want orphan page", orphans.Candidates[0])
	}
	assertCandidatePages(t, orphans.Candidates[0], []string{"wiki/orphan.md"})
	assertCandidateReasonCode(t, orphans.Candidates[0], ReasonOrphanPage)

	sources, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindSources, Limit: 5})
	if err != nil {
		t.Fatalf("source candidates: %v", err)
	}
	if len(sources.Candidates) != 1 {
		t.Fatalf("source candidate count = %d, want 1: %#v", len(sources.Candidates), sources.Candidates)
	}
	if sources.Candidates[0].Type != CandidateTypeUncitedSource {
		t.Fatalf("source candidate = %#v, want uncited source", sources.Candidates[0])
	}
	assertCandidateSources(t, sources.Candidates[0], []string{"sources/unused.md"})
	assertCandidateReasonCode(t, sources.Candidates[0], ReasonUncitedSource)
}

func TestHealthCandidatesAddsOlderRelevantPageReason(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateWikiDoc("doc_retention_old", "wiki/retention-old.md", "Tempo retention", "Older retention guidance.", `["tempo","retention"]`, `["sources/tempo-raw.md"]`, `[]`, "2026-01-01"),
		candidateWikiDoc("doc_retention_new", "wiki/retention-new.md", "Tempo retention update", "Newer retention guidance.", `["tempo","retention"]`, `["sources/tempo-raw.md"]`, `[]`, "2026-05-06"),
		candidateSourceDoc("source_tempo_raw", "sources/tempo-raw.md", "Tempo raw"),
	}
	sections := []Section{
		{DocumentID: "doc_retention_old", Ordinal: 1, Heading: "Tempo retention", Anchor: "tempo-retention", Level: 1, Body: "Tempo retention compaction guidance for trace blocks."},
		{DocumentID: "doc_retention_new", Ordinal: 1, Heading: "Tempo retention update", Anchor: "tempo-retention-update", Level: 1, Body: "Tempo retention update for compaction and trace blocks."},
		{DocumentID: "source_tempo_raw", Ordinal: 1, Heading: "Tempo raw", Anchor: "tempo-raw", Level: 1, Body: "Raw Tempo retention evidence."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-stale"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindDuplicates, Limit: 1})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(response.Candidates), response.Candidates)
	}
	assertCandidateReason(t, response.Candidates[0], ReasonOlderRelevantPage, "wiki/retention-old.md")
}

func TestHealthCandidatesFiltersByPathPrefix(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateWikiDoc("doc_tempo_a", "wiki/tempo/a.md", "Tempo A", "Tempo shared.", `["tempo","shared"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_tempo_b", "wiki/tempo/b.md", "Tempo B", "Tempo shared.", `["tempo","shared"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_mimir_a", "wiki/mimir/a.md", "Mimir A", "Mimir shared.", `["mimir","shared"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_mimir_b", "wiki/mimir/b.md", "Mimir B", "Mimir shared.", `["mimir","shared"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "doc_tempo_a", Ordinal: 1, Heading: "Tempo A", Anchor: "tempo-a", Level: 1, Body: "Tempo shared detail."},
		{DocumentID: "doc_tempo_b", Ordinal: 1, Heading: "Tempo B", Anchor: "tempo-b", Level: 1, Body: "Tempo shared detail."},
		{DocumentID: "doc_mimir_a", Ordinal: 1, Heading: "Mimir A", Anchor: "mimir-a", Level: 1, Body: "Mimir shared detail."},
		{DocumentID: "doc_mimir_b", Ordinal: 1, Heading: "Mimir B", Anchor: "mimir-b", Level: 1, Body: "Mimir shared detail."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-prefix"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindAll, PathPrefix: "wiki/tempo", Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) == 0 {
		t.Fatal("no path-filtered candidates returned")
	}
	for _, candidate := range response.Candidates {
		matched := false
		for _, page := range candidate.Pages {
			if page == "wiki/tempo/a.md" || page == "wiki/tempo/b.md" {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("candidate outside path prefix returned: %#v", candidate)
		}
	}
}

func candidateWikiDoc(id, pathValue, title, summary, tagsJSON, sourcesJSON, linksJSON, modifiedDate string) Document {
	return Document{
		ID:           id,
		Path:         pathValue,
		Kind:         KindWiki,
		Title:        title,
		Summary:      summary,
		TagsJSON:     tagsJSON,
		SourcesJSON:  sourcesJSON,
		LinksJSON:    linksJSON,
		ModifiedDate: modifiedDate,
		Hash:         "hash-" + id,
		SizeBytes:    100,
	}
}

func candidateSourceDoc(id, pathValue, title string) Document {
	return Document{
		ID:        id,
		Path:      pathValue,
		Kind:      KindSource,
		Title:     title,
		Hash:      "hash-" + id,
		SizeBytes: 100,
	}
}

func assertCandidatePages(t *testing.T, candidate Candidate, want []string) {
	t.Helper()
	if len(candidate.Pages) != len(want) {
		t.Fatalf("candidate pages length = %d, want %d: %#v", len(candidate.Pages), len(want), candidate)
	}
	for i := range want {
		if candidate.Pages[i] != want[i] {
			t.Fatalf("candidate page[%d] = %q, want %q: %#v", i, candidate.Pages[i], want[i], candidate)
		}
	}
}

func assertCandidateSources(t *testing.T, candidate Candidate, want []string) {
	t.Helper()
	if len(candidate.Sources) != len(want) {
		t.Fatalf("candidate sources length = %d, want %d: %#v", len(candidate.Sources), len(want), candidate)
	}
	for i := range want {
		if candidate.Sources[i] != want[i] {
			t.Fatalf("candidate source[%d] = %q, want %q: %#v", i, candidate.Sources[i], want[i], candidate)
		}
	}
}

func assertCandidateReasonCode(t *testing.T, candidate Candidate, code string) {
	t.Helper()
	for _, reason := range candidate.Reasons {
		if reason.Code == code {
			return
		}
	}
	t.Fatalf("candidate missing reason code %q: %#v", code, candidate)
}

func assertCandidateReason(t *testing.T, candidate Candidate, code, value string) {
	t.Helper()
	for _, reason := range candidate.Reasons {
		if reason.Code == code && reason.Value == value {
			return
		}
	}
	t.Fatalf("candidate missing reason %q=%q: %#v", code, value, candidate)
}

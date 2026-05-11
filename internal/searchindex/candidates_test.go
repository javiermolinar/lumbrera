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

func TestHealthCandidatesFindsLexicalOnlyMissingLinks(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateWikiDoc("doc_alpha_one", "wiki/alpha-one.md", "Alpha one", "Lexical hydra flux notes.", `["one"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_alpha_two", "wiki/alpha-two.md", "Alpha two", "Lexical hydra flux guidance.", `["two"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "doc_alpha_one", Ordinal: 1, Heading: "Alpha one", Anchor: "alpha-one", Level: 1, Body: "Lexical hydra flux overlap for missing link review."},
		{DocumentID: "doc_alpha_two", Ordinal: 1, Heading: "Alpha two", Anchor: "alpha-two", Level: 1, Body: "Lexical hydra flux overlap for related page review."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-lexical"}); err != nil {
		t.Fatalf("rebuild lexical candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindLinks, Limit: 3})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(response.Candidates), response.Candidates)
	}
	candidate := response.Candidates[0]
	if candidate.Type != CandidateTypeMissingLink {
		t.Fatalf("candidate type = %q, want missing link: %#v", candidate.Type, candidate)
	}
	assertCandidateReasonCode(t, candidate, ReasonLexicalOverlap)
	assertCandidateReason(t, candidate, ReasonNotLinked, "")
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

func TestHealthCandidatesFindsStubPages(t *testing.T) {
	db := openTestDB(t)
	// stub: 1 body line; non-stub: 20 body lines
	docs := []Document{
		candidateWikiDoc("doc_stub", "wiki/stub.md", "Stub page", "Tiny page.", `["stub"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_full", "wiki/full.md", "Full page", "Full page.", `["full"]`, `[]`, `[]`, "2026-05-06"),
	}
	var fullBody string
	for i := 0; i < 20; i++ {
		if i > 0 {
			fullBody += "\n"
		}
		fullBody += "Full content line for a non-stub page."
	}
	sections := []Section{
		{DocumentID: "doc_stub", Ordinal: 1, Heading: "Stub page", Anchor: "stub-page", Level: 1, Body: "Tiny body."},
		{DocumentID: "doc_full", Ordinal: 1, Heading: "Full page", Anchor: "full-page", Level: 1, Body: fullBody},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-stubs"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindStubs, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(response.Candidates), response.Candidates)
	}
	candidate := response.Candidates[0]
	if candidate.Type != CandidateTypeStubPage {
		t.Fatalf("candidate type = %q, want %q", candidate.Type, CandidateTypeStubPage)
	}
	assertCandidatePages(t, candidate, []string{"wiki/stub.md"})
	assertCandidateReasonCode(t, candidate, ReasonStubPage)
}

func TestHealthCandidatesFindsStubPagesMultipleSections(t *testing.T) {
	db := openTestDB(t)
	// Page with 3 sections of 4 lines each = 12 body lines → still a stub (< 15)
	docs := []Document{
		candidateWikiDoc("doc_multi", "wiki/multi.md", "Multi section", "Multi.", `["multi"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "doc_multi", Ordinal: 1, Heading: "Section one", Anchor: "s1", Level: 1, Body: "Line one.\nLine two.\nLine three.\nLine four."},
		{DocumentID: "doc_multi", Ordinal: 2, Heading: "Section two", Anchor: "s2", Level: 2, Body: "Line one.\nLine two.\nLine three.\nLine four."},
		{DocumentID: "doc_multi", Ordinal: 3, Heading: "Section three", Anchor: "s3", Level: 2, Body: "Line one.\nLine two.\nLine three.\nLine four."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-stubs-multi"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindStubs, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(response.Candidates), response.Candidates)
	}
	if response.Candidates[0].Type != CandidateTypeStubPage {
		t.Fatalf("candidate type = %q, want %q", response.Candidates[0].Type, CandidateTypeStubPage)
	}
}

func TestHealthCandidatesFindsTagAnomalies(t *testing.T) {
	db := openTestDB(t)
	// 5 wiki pages. "singleton" used by 1 page, "broad" used by 4/5 (80% > 40%), "normal" used by 2.
	docs := []Document{
		candidateWikiDoc("doc_a", "wiki/a.md", "Page A", "A.", `["broad","normal"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_b", "wiki/b.md", "Page B", "B.", `["broad","normal"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_c", "wiki/c.md", "Page C", "C.", `["broad"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_d", "wiki/d.md", "Page D", "D.", `["broad"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_e", "wiki/e.md", "Page E", "E.", `["singleton"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "doc_a", Ordinal: 1, Heading: "A", Anchor: "a", Level: 1, Body: "Page A content."},
		{DocumentID: "doc_b", Ordinal: 1, Heading: "B", Anchor: "b", Level: 1, Body: "Page B content."},
		{DocumentID: "doc_c", Ordinal: 1, Heading: "C", Anchor: "c", Level: 1, Body: "Page C content."},
		{DocumentID: "doc_d", Ordinal: 1, Heading: "D", Anchor: "d", Level: 1, Body: "Page D content."},
		{DocumentID: "doc_e", Ordinal: 1, Heading: "E", Anchor: "e", Level: 1, Body: "Page E content."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-tags"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindTags, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(response.Candidates), response.Candidates)
	}
	// Broad tag (score 0.38) should rank above singleton (score 0.28).
	broad := response.Candidates[0]
	if broad.Type != CandidateTypeTagAnomaly {
		t.Fatalf("broad candidate type = %q, want %q", broad.Type, CandidateTypeTagAnomaly)
	}
	assertCandidateReasonCode(t, broad, ReasonTagTooBroad)

	singleton := response.Candidates[1]
	if singleton.Type != CandidateTypeTagAnomaly {
		t.Fatalf("singleton candidate type = %q, want %q", singleton.Type, CandidateTypeTagAnomaly)
	}
	assertCandidateReasonCode(t, singleton, ReasonTagTooSpecific)
	assertCandidatePages(t, singleton, []string{"wiki/e.md"})
}

func TestHealthCandidatesFindsSourceCoverageGaps(t *testing.T) {
	db := openTestDB(t)
	docs := []Document{
		candidateSourceDoc("source_a", "sources/a.md", "Source A"),
		candidateWikiDoc("doc_w1", "wiki/w1.md", "Wiki one", "W1.", `["tag"]`, `["sources/a.md"]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		// Source has 3 H2 sections.
		{DocumentID: "source_a", Ordinal: 1, Heading: "Section one", Anchor: "section-one", Level: 2, Body: "Content one."},
		{DocumentID: "source_a", Ordinal: 2, Heading: "Section two", Anchor: "section-two", Level: 2, Body: "Content two."},
		{DocumentID: "source_a", Ordinal: 3, Heading: "Section three", Anchor: "section-three", Level: 2, Body: "Content three."},
		{DocumentID: "doc_w1", Ordinal: 1, Heading: "Wiki one", Anchor: "wiki-one", Level: 1, Body: "Cites section one."},
	}
	// Only section-one is cited inline; section-two and section-three are gaps.
	citations := []DocumentCitation{
		{DocumentID: "doc_w1", WikiPath: "wiki/w1.md", SourcePath: "sources/a.md", CitationKind: "frontmatter_source", CitationText: "sources/a.md"},
		{DocumentID: "doc_w1", WikiPath: "wiki/w1.md", SourcePath: "sources/a.md", SourceAnchor: "section-one", CitationKind: "inline_source", CitationText: "Source A \u00a7 Section one", SectionID: "doc_w1#section-0001"},
	}
	tags := []DocumentTag{
		{DocumentID: "doc_w1", Path: "wiki/w1.md", Tag: "tag"},
	}
	if err := RebuildRecordsWithFacts(context.Background(), db, docs, sections, nil, citations, tags, map[string]string{"manifest_hash": "candidate-gap"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindSources, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	// uncited_source should NOT fire (source is cited). source_coverage_gap should fire.
	var gapCandidates []Candidate
	for _, c := range response.Candidates {
		if c.Type == CandidateTypeSourceCoverageGap {
			gapCandidates = append(gapCandidates, c)
		}
		if c.Type == CandidateTypeUncitedSource {
			t.Fatalf("source_a should not be flagged as uncited: %#v", c)
		}
	}
	if len(gapCandidates) != 1 {
		t.Fatalf("source_coverage_gap count = %d, want 1: %#v", len(gapCandidates), response.Candidates)
	}
	candidate := gapCandidates[0]
	assertCandidateSources(t, candidate, []string{"sources/a.md"})
	// Should have 2 uncited sections: section-two and section-three.
	uncitedCount := 0
	for _, reason := range candidate.Reasons {
		if reason.Code == ReasonUncitedSection {
			uncitedCount++
		}
	}
	if uncitedCount != 2 {
		t.Fatalf("uncited_section reason count = %d, want 2: %#v", uncitedCount, candidate.Reasons)
	}
	assertCandidateReason(t, candidate, ReasonUncitedSection, "Section two")
	assertCandidateReason(t, candidate, ReasonUncitedSection, "Section three")
}

func TestHealthCandidatesSourceCoverageGapSkipsUncitedSources(t *testing.T) {
	db := openTestDB(t)
	// Source with sections but no citations at all — should only trigger uncited_source, not coverage_gap.
	docs := []Document{
		candidateSourceDoc("source_b", "sources/b.md", "Source B"),
		candidateWikiDoc("doc_w2", "wiki/w2.md", "Wiki two", "W2.", `["tag"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "source_b", Ordinal: 1, Heading: "Heading B1", Anchor: "heading-b1", Level: 2, Body: "B1 content."},
		{DocumentID: "source_b", Ordinal: 2, Heading: "Heading B2", Anchor: "heading-b2", Level: 2, Body: "B2 content."},
		{DocumentID: "doc_w2", Ordinal: 1, Heading: "Wiki two", Anchor: "wiki-two", Level: 1, Body: "Unrelated."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-gap-skip"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindSources, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	for _, c := range response.Candidates {
		if c.Type == CandidateTypeSourceCoverageGap {
			t.Fatalf("uncited source should not produce coverage_gap: %#v", c)
		}
	}
	// Should have uncited_source instead.
	found := false
	for _, c := range response.Candidates {
		if c.Type == CandidateTypeUncitedSource {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected uncited_source candidate for sources/b.md: %#v", response.Candidates)
	}
}

func TestHealthCandidatesTagAnomalySkipsSmallBrains(t *testing.T) {
	db := openTestDB(t)
	// Only 2 wiki pages — below the minimum threshold of 3.
	docs := []Document{
		candidateWikiDoc("doc_x", "wiki/x.md", "X", "X.", `["only"]`, `[]`, `[]`, "2026-05-06"),
		candidateWikiDoc("doc_y", "wiki/y.md", "Y", "Y.", `["other"]`, `[]`, `[]`, "2026-05-06"),
	}
	sections := []Section{
		{DocumentID: "doc_x", Ordinal: 1, Heading: "X", Anchor: "x", Level: 1, Body: "X content."},
		{DocumentID: "doc_y", Ordinal: 1, Heading: "Y", Anchor: "y", Level: 1, Body: "Y content."},
	}
	if err := RebuildRecords(context.Background(), db, docs, sections, map[string]string{"manifest_hash": "candidate-tags-small"}); err != nil {
		t.Fatalf("rebuild candidate fixture: %v", err)
	}

	response, err := HealthCandidates(context.Background(), db, CandidateOptions{Kind: CandidateKindTags, Limit: 10})
	if err != nil {
		t.Fatalf("health candidates: %v", err)
	}
	if len(response.Candidates) != 0 {
		t.Fatalf("expected no tag anomaly candidates for small brain, got %d: %#v", len(response.Candidates), response.Candidates)
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

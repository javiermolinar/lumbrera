# Lumbrera health and consolidation candidate plan

## Context

Lumbrera now has deterministic SQLite/FTS5 search for wiki and source Markdown. That helps agents find relevant context, but it does not solve semantic drift: duplicated pages, fragmented concepts, stale claims, missing cross-links, and source coverage gaps require LLM reasoning.

The desired direction is a deterministic first pass that returns likely drift/consolidation review candidates, followed by LLM review. The CLI should not claim that pages are semantically drifted; it should rank where to look and explain deterministic reasons.

## Goals

- Provide a deterministic way to return likely health/consolidation candidates for LLM review.
- Keep the CLI minimal and avoid turning Lumbrera into a large semantic-analysis app.
- Add factual relationship data to the derived SQLite index so candidate generation is explainable.
- Let the LLM perform follow-up `lumbrera search` queries, read pages/sources, and decide whether to merge, link, split, update, skip, or investigate further.
- Preserve the existing mutation boundary: all changes still go through `lumbrera write`, followed by `lumbrera verify`.
- Use existing write/delete primitives for consolidation; do not add a first-cut move command.

## Constraints

- Do not present deterministic candidates as proven semantic drift.
- Do not add `validation_issues` tables for this workflow; verification errors are separate from consolidation signals.
- Prefer fact tables over error tables.
- Keep SQLite as a disposable derived cache; Markdown, generated metadata, and ops logs remain canonical.
- Avoid broad per-page LLM scans as the primary workflow; the deterministic candidate pass should reduce the review set.
- Do not add a dedicated move command in the first cut; use create/update plus delete where needed.

## Implementation status

Done:

- Search schema version bumped to v2.
- Added `document_links`, `document_citations`, and `document_tags` fact tables plus indexes.
- Added `modified_date` to existing `documents` and `sections` tables.
- Populated relationship fact tables during deterministic index rebuild.
- Added generated `lumbrera.modified_date` support:
  - `lumbrera write` sets it on wiki create/update/append;
  - `lumbrera index --rebuild` repairs older wiki pages missing it before indexing;
  - wiki documents entering the search database must have a valid `YYYY-MM-DD` modified date.
- Tested with `~/grafana/brain`: rebuilt schema v2 index, repaired 23 wiki pages, and populated 28 links, 46 citations, and 81 tags.
- Implemented the internal candidate generator in `internal/searchindex`.
- Added the read-only `lumbrera health` command with JSON and compact human output.
- Added health command and candidate generator tests; `go test ./...` passes.
- Tuned first-cut ranking against `~/grafana/brain` after reading top candidate pages:
  - broad related-page pairs now classify as `missing_link` instead of overclaiming `possible_duplicate`;
  - missing-link scores are calibrated as review priority, not certainty;
  - already-linked weak pairs are suppressed;
  - broad shared sources are downweighted;
  - cross-product analogy pairs without shared source evidence are downranked;
  - suggested query stopwords were tightened.

Remaining:

- Formal JSON/human output contract fixtures once the command surface stabilizes.
- Formal JSON/human output contract fixtures once the command surface stabilizes.
- Further candidate ranking tuning only as real brain review exposes noisy signals.

## Proposed plan

1. Extend the search index with relationship fact tables: ✅ done
   - `document_links` for exact wiki/source link graph facts.
   - `document_citations` for file-level and inline source citation facts.
   - `document_tags` for exact tag facts.

2. Update existing search index tables for freshness metadata: ✅ done
   - add `modified_date TEXT NOT NULL` to `documents` for wiki pages;
   - denormalize `modified_date TEXT NOT NULL` into `sections` so search/health output can use it without extra joins;
   - use `""` for source documents because source files do not have Lumbrera wiki freshness frontmatter;
   - include `modified_date` in deterministic row dumps, schema tests, manifest/staleness expectations, and JSON output only where the public contract needs it.

3. Populate relationship tables during the existing deterministic rebuild: ✅ done
   - extract links, citations, and tags from the same canonical Markdown/frontmatter parsing used today;
   - insert rows in deterministic order;
   - include these table outputs in schema/version expectations.

4. Add generated wiki freshness metadata: ✅ done
   - add required generated `lumbrera.modified_date` frontmatter for wiki pages, formatted as `YYYY-MM-DD`;
   - set it during `lumbrera write` create/update/append operations;
   - do not expose it as an LLM-authored field; agents still provide only body, title, summary, tags, and sources;
   - do not update it during index rebuilds, search, health checks, or verification-only reads;
   - add migration/repair behavior for older wiki pages missing `modified_date` before they can be indexed into the DB;
   - require a valid `modified_date` for every wiki document row that enters the search/health database;
   - use this as page freshness metadata, not as a proof of staleness.

5. Add an internal candidate generator in `internal/searchindex`: ✅ done
   - first cut returns single-page and page-pair candidates only;
   - rank page pairs by shared tags, shared cited sources, lexical overlap, and missing links;
   - rank single-page/source candidates for orphan/underlinked pages and source coverage gaps;
   - add stale-risk boosts when an older page is also BM25-relevant to newer or related pages/sources;
   - return reason codes such as `shared_tag`, `shared_source`, `not_linked`, `lexical_overlap`, `orphan_page`, `uncited_source`, or `older_relevant_page`;
   - produce suggested follow-up queries for the LLM.

6. Add a narrow read-only public command for candidate output: ✅ done
   - command: `lumbrera health`;
   - purpose: return deterministic health/consolidation review candidates;
   - non-purpose: do not diagnose semantic drift, do not mutate files, and do not replace LLM review;
   - default scope: whole brain;
   - supported scope flags: `--path <prefix>` for an area and optional positional `wiki/page.md` for one page neighborhood;
   - supported candidate kinds: `--kind all|duplicates|links|sources|orphans`;
   - support `--limit <n>` and `--json`.

7. Define candidate output contracts: partially done
   - JSON returns candidate type, score, confidence bucket, pages/sources involved, deterministic reason codes, suggested follow-up queries, and review instructions;
   - human output is compact and explains deterministic reasons, not conclusions;
   - uses `candidates` terminology, not `drifted_pages`;
   - remaining: add formal contract fixtures if the public surface changes.

8. Update the generated health skill: ✅ done
   - renamed generated skill from `lumbrera-lint` to `lumbrera-health`;
   - added a Goal section explaining that `lumbrera health` narrows review to deterministic candidates, not conclusions;
   - consumes candidate packets and starts with `lumbrera health --json`;
   - reads only top ranked candidates first;
   - runs follow-up `lumbrera search` queries when candidate evidence is insufficient;
   - classifies outcomes as duplicate, overlap, missing cross-link, stale-risk, missing concept/source gap, or no action;
   - includes a missing-link triage section to avoid treating every medium-confidence candidate as a todo;
   - includes a duplicate/consolidation helper for canonical-page, overlap, stale-risk, and no-action decisions;
   - applies mutations only with explicit user approval and `lumbrera write`;
   - prefers updating the canonical page plus rewriting duplicate pages into short stubs that link to the canonical page;
   - uses `lumbrera write --delete` only when content is fully covered elsewhere and verification confirms no broken links.

9. Add tests: partially done
   - deterministic table population for links, citations, and tags;
   - candidate ranking fixtures for shared tag/source plus missing link;
   - orphan/underlinked page candidates;
   - uncited source candidates;
   - read-only health command JSON/human smoke coverage;
   - remaining: formal output contract fixtures if the public surface changes.

## Command UX

Examples:

```sh
lumbrera health --json
lumbrera health --path wiki/tempo/ --json
lumbrera health wiki/tempo/downscale.md --json
lumbrera health --kind duplicates --json
lumbrera health --kind links --limit 20 --json
```

Candidate JSON shape:

```json
{
  "candidates": [
    {
      "type": "possible_duplicate",
      "confidence": "medium",
      "score": 0.82,
      "pages": ["wiki/a.md", "wiki/b.md"],
      "sources": ["sources/foo.md"],
      "reasons": [
        {"code": "shared_tag", "value": "tempo"},
        {"code": "shared_source", "value": "sources/foo.md"},
        {"code": "not_linked"}
      ],
      "suggested_queries": [
        "tempo downscale retention",
        "sources/foo.md tempo"
      ],
      "review_instruction": "Read both pages and cited sources before deciding whether to merge, cross-link, or leave separate."
    }
  ],
  "stop_rule": "Review top candidates first. Do not scan the repo unless candidates are insufficient."
}
```

Human output shape:

```text
1. possible_duplicate medium score=0.82
   pages: wiki/a.md, wiki/b.md
   reasons: shared_tag=tempo, shared_source=sources/foo.md, not_linked
   next: search "tempo downscale retention"; read both pages and cited sources
```

## Freshness model

`modified_date` is mandatory generated wiki metadata for database-backed search/health features. It makes old-but-relevant pages easier to surface for LLM review. Age alone should not create a candidate. It should only boost candidates that already have deterministic relevance signals such as lexical overlap, shared tags, shared sources, or missing links.

Source freshness remains separate. If source freshness becomes important, derive it later from source path conventions, explicit source metadata, or `.brain/ops.log` indexing.

## Consolidation mutation model

The first cut should use existing write primitives:

- create or update canonical pages with `lumbrera write wiki/<path>.md ...`;
- update inbound/contextual links with normal `lumbrera write` operations;
- delete only with existing `lumbrera write wiki/<path>.md --delete --reason "..."`;
- avoid a dedicated `move` command until there is evidence that identity-preserving renames are needed.

For semantic consolidation, prefer canonical-page updates plus duplicate-page stubs before deletion. A true move would need multi-file atomicity, document ID preservation, and link rewriting, so it is deferred.

## Iterative health workflow

The first cut intentionally avoids cluster detection. The LLM should use an iterative health loop:

1. Run `lumbrera health --json`.
2. Review the top single-page or page-pair candidate.
3. Run follow-up `lumbrera search` queries if needed.
4. Read the candidate pages and cited sources.
5. Decide whether to merge/update, add cross-links, create a missing concept page, leave unchanged, or request more sources.
6. If mutating, use `lumbrera write`, then `lumbrera verify`.
7. Run `lumbrera health --json` again to get the next candidate.

If false positives become noisy, return top-N candidates and let the LLM skip to the next candidate. Persistent ignore state is deferred.

## Next steps

- Add formal JSON/human output contract fixtures when the command shape stabilizes.
- Consider a later UX split between relation strength and action priority if score semantics remain confusing.

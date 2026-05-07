# Source tiers and search ranking

## Problem

All sources and wiki pages rank equally in search. A design proposal scores the same as canonical product documentation. The LLM cannot structurally distinguish "how Tempo works" from "how Tempo might work someday."

Path conventions (`sources/design/`, `wiki/design/`) and tags (`design`, `draft`) help, but they are advisory. Search ranking is the only signal that affects what the LLM reads first on every query.

## Design

### Closed tier enum

Three tiers. No more until a real content type forces a fourth.

```
canonical   Current product docs, operational guides, runbooks, reference.
design      Proposals, ADRs, specs, alternatives. Not implemented.
reference   Preserved context that rarely surfaces: meeting notes, competition, historical.
```

Default: `canonical`. Unknown path prefixes → `canonical`.

### Tier inference from path

No new CLI flags on `lumbrera write`. No metadata sidecar. The path is the tier.

```
sources/design/       → design
sources/reference/    → reference
wiki/design/          → design
wiki/reference/       → reference
everything else       → canonical
```

Inference function: ~15 lines. One place. Deterministic.

### Search ranking weights

Applied as a multiplier on the FTS5 text score during search SQL.

```
canonical   1.00
design      0.70
reference   0.50
```

Effect: for "querier batching", the canonical querier page ranks above the design proposal. The design proposal is still findable but structurally demoted.

### Tier in search output

Expose `tier` in every search result JSON object:

```json
{
  "path": "wiki/design/adaptive-sharding-proposal.md",
  "tier": "design",
  "title": "Adaptive sharding proposal (draft, Jan 2026)",
  "score": -11.44
}
```

The LLM sees the tier label alongside every result without reading the page.

### Tier filter on search

```sh
lumbrera search "querier batching"                    # all tiers, weighted
lumbrera search "querier batching" --tier canonical    # only canonical
lumbrera search "adaptive sharding" --tier design      # only design
```

Multiple `--tier` flags use OR: `--tier canonical --tier design` returns both.

### Verify rejects unknown tier prefixes

`lumbrera verify` checks that every `sources/<prefix>/` and `wiki/<prefix>/` maps to a known tier. Typos like `sources/desing/` fail verification instead of silently becoming canonical.

Known tier directories: `design/`, `reference/`. Everything else is canonical (no special directory required).

## Implementation

### Files to change

```
internal/searchindex/extract.go     Infer tier from path prefix
internal/searchindex/schema.go      Add tier column to documents table, bump schema version
internal/searchindex/rebuild.go     Store tier during rebuild
internal/searchindex/search_sql.go  Apply tier weight multiplier in ranking
internal/searchindex/search.go      Expose tier in SearchResult
internal/searchcmd/search.go        Add --tier filter flag, include tier in JSON output
internal/verify/verify.go           Reject unknown tier path prefixes
```

### Schema change

Add `tier TEXT NOT NULL DEFAULT 'canonical'` to `documents` table. Bump `CurrentSchemaVersion`. Existing brains auto-rebuild on next search (stale detection).

### Search SQL change

```sql
SELECT ...,
  rank * CASE d.tier
    WHEN 'design' THEN 0.70
    WHEN 'reference' THEN 0.50
    ELSE 1.00
  END AS weighted_rank
FROM sections_fts
JOIN sections ...
JOIN documents d ...
ORDER BY weighted_rank
```

### No migration needed

Schema bump triggers auto-rebuild from canonical Markdown files. Tier is inferred from path at rebuild time. No stored metadata to migrate.

## What this does NOT include

- `--tier` flag on `lumbrera write` — path encodes tier, flag is redundant
- Source metadata sidecar — not needed if path is authoritative
- Lifecycle commands (`set-tier`, `promote`) — move the file to a new path instead
- Automatic staleness detection — separate feature, needs date tracking
- Tier-aware health candidates — later, if health workflow needs it
- Query intent detection ("why" boosts design, "how" boosts canonical) — later

## Adding a 4th tier later

1. Add the string to the tier enum / inference function
2. Assign a ranking weight
3. Add the directory name to verify's known-tier list
4. Schema version stays the same (tier column already exists)
5. `lumbrera index --rebuild` picks it up

One-line code change + rebuild. No migration.

## Success criteria

- `lumbrera search "querier batching"` returns canonical querier page above design proposal
- `lumbrera search "adaptive sharding" --tier design` returns only design content
- `lumbrera verify` rejects `sources/desing/foo.md` (typo)
- All existing tests pass without modification (default tier = canonical)
- No new CLI flags on `lumbrera write`

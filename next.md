# Lumbrera next

## Context

The old `2026-05-02-lumbrera-plan.md` has been renamed to this `next.md`. The current implementation has the local v1 shape mostly in place: `init`, `write`, and `verify`; generated `INDEX.md`, `CHANGELOG.md`, `BRAIN.sum`, and read-only `tags.md`; immutable raw sources; stable generated wiki document IDs; mandatory single-line wiki summary, 1-5 tags, and 400-line max wiki body; generated wiki frontmatter and source sections; link, heading-anchor, and inline source-citation validation; rollback on failed writes; and bundled ingest/query/lint skills.

The next milestone is v2: make Lumbrera easier for LLM agents to query by adding a deterministic derived SQLite lexical search index plus `lumbrera index` and `lumbrera search` commands.

## V2 goal

Build a deterministic local indexing and lexical search tool backed by SQLite FTS5 so agents can quickly find relevant wiki pages, source evidence, headings, links, citations, and related documents before deciding what to read in full. FTS5 is full-text search, not nearest-neighbor or semantic vector search; embeddings stay deferred to v2+.

The index is a derived cache, not source of truth. Markdown files, generated Lumbrera document IDs, generated metadata, `tags.md`, and `.brain/ops.log` remain canonical. The SQLite database must be safe to delete and rebuild, and `lumbrera init` must scaffold `.gitignore` so generated brain repos ignore `.brain/search.sqlite*`. Rebuild equivalence means same canonical inputs, same schema version, same normalized output bytes for `documents` and `sections`, and same ranked result order for identical queries. `tags.md` is itself generated from wiki frontmatter and read-only for agents.

## What is missing for v2

- SQLite-backed derived index stored under `.brain/`, likely `.brain/search.sqlite`.
- Deterministic rebuild-first indexing keyed by stable generated Lumbrera document IDs in wiki frontmatter; `verify` repairs missing IDs in older wiki pages.
- Index schema versioning and rebuild/staleness detection; stale or missing indexes are rebuilt from canonical Markdown.
- Markdown section extraction for both `wiki/` and `sources/`; wiki pages are intentionally capped at 400 body lines so section indexing stays predictable.
- Deterministic section IDs from document ID plus section ordinal, with no stable/public line ranges. Section IDs are rebuild-stable for identical inputs, but not edit-stable when earlier sections change.
- Lexical full-text search over titles, summaries, tags, headings, body text, paths, and source citations.
- Search ranking that prefers wiki synthesis first, uses path/title/summary/tags for document routing, and still exposes raw sources when evidence matters.
- Agent-readable `lumbrera search` command output with citations and stable JSON mode.
- Dedicated `lumbrera index` command for explicit cache rebuilds, status checks, and schema maintenance.
- Query-skill updates so agents use search before broad file reads.
- Tests for indexing, stale rebuilds, search ranking, CLI output, and generated scaffold docs.

## Proposed commands

Decision: expose a dedicated `lumbrera index` command. Indexing is still a derived cache operation, but explicit rebuilds, status checks, and future schema maintenance belong to `index`, not `search`.

```sh
lumbrera index --brain ./brain --status
lumbrera index --brain ./brain --rebuild
lumbrera search "atomic write protocol" --brain ./brain --limit 5
lumbrera search "source immutability" --kind wiki --json
```

Search options to consider:

```text
--brain <path>       target brain directory, default current directory
--limit <n>          max results, default 5, maximum 20
--kind <all|wiki|source>
--path <prefix>      restrict to a repo path prefix
--json               machine-readable output for agents

Deferred exact filters, only if first-cut lexical search is insufficient:
--tag <tag>          restrict wiki documents by generated tag
--source <path>      restrict wiki results to pages citing a source
```

Index options to consider:

```text
--brain <path>       target brain directory, default current directory
--status             report whether the index exists, schema version, and staleness
--rebuild            force a full deterministic rebuild
```

Default behavior: if the index is missing or stale, `search` may try one automatic rebuild by invoking the same indexer used by `lumbrera index`. Explicit repair/debug flows should use `lumbrera index --status` and `lumbrera index --rebuild`. If auto-rebuild cannot write the cache because the filesystem is read-only, locked, or unavailable, `search` should fail clearly and tell the user to run `lumbrera index --rebuild`. Failed rebuilds should exit non-zero and point to `lumbrera verify` if the brain has deterministic integrity problems. The default result limit should stay small so agents do not treat search as an invitation to explore the full repo.

## Proposed index shape

Use SQLite FTS5 for deterministic lexical full-text search through `modernc.org/sqlite`, the CGO-free Go module for `gitlab.com/cznic/sqlite`. A local probe confirmed FTS5 support with `CGO_ENABLED=0`. This is not a nearest-neighbor index.

First cut should prefer full deterministic rebuilds over incremental mutation. Rebuilds scan canonical files in lexicographic order and produce equivalent database contents for the same file tree. The staleness manifest should explicitly hash schema version, parse-affecting rules, normalized indexed paths, file hashes, and generated frontmatter fields that affect indexed output. Incremental sync can be added later only if rebuild cost becomes painful.

Core tables:

```sql
meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)

documents(
  id TEXT PRIMARY KEY,         -- wiki: generated Lumbrera id; source: deterministic path-derived id
  path TEXT UNIQUE NOT NULL,
  kind TEXT NOT NULL,          -- wiki or source
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  tags_json TEXT NOT NULL,     -- deterministic JSON array
  sources_json TEXT NOT NULL,  -- deterministic JSON array
  links_json TEXT NOT NULL,    -- deterministic JSON array
  tags_text TEXT NOT NULL,     -- normalized deterministic search/display text
  sources_text TEXT NOT NULL,  -- normalized deterministic search/display text
  links_text TEXT NOT NULL,    -- normalized deterministic search/display text
  hash TEXT NOT NULL,
  size_bytes INTEGER NOT NULL
)

sections(
  rowid INTEGER PRIMARY KEY,   -- internal FTS rowid, assigned deterministically during full rebuild
  id TEXT UNIQUE NOT NULL,     -- <document-id>#section-0001
  document_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  path TEXT NOT NULL,          -- denormalized from documents for cheap result output
  kind TEXT NOT NULL,          -- denormalized from documents for filtering/ranking
  title TEXT NOT NULL,         -- denormalized from documents for FTS/ranking
  summary TEXT NOT NULL,       -- denormalized from documents for FTS/ranking
  tags_json TEXT NOT NULL,     -- denormalized for JSON output without extra joins
  sources_json TEXT NOT NULL,  -- denormalized for JSON output without extra joins
  links_json TEXT NOT NULL,    -- denormalized for JSON output without extra joins
  tags_text TEXT NOT NULL,     -- denormalized for FTS/ranking
  sources_text TEXT NOT NULL,  -- denormalized for FTS/ranking
  links_text TEXT NOT NULL,    -- denormalized for FTS/ranking
  heading TEXT,
  anchor TEXT,
  level INTEGER,
  body TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id) ON DELETE CASCADE
)

sections_fts USING fts5(
  title,
  path,
  summary,
  tags_text,
  sources_text,
  links_text,
  heading,
  body,
  content='sections',
  content_rowid='rowid'
)
```

Index inputs:

- Scan `wiki/` and `sources/` paths in sorted order.
- `wiki/`: parse generated Lumbrera frontmatter into `documents`, require generated id/title/summary/1-5 tags, normalize tags/sources/links into deterministic JSON plus searchable text, strip frontmatter before body indexing, and split body into denormalized deterministic `sections` with the same heading-anchor algorithm used by verification.
- `sources/`: preserve raw Markdown, derive a stable source document ID from normalized path, split by headings, ignore unresolved links inside sources just like current verification does, and set NOT NULL metadata deterministically: title is first H1, else first heading, else filename stem; summary is `""`; tags/sources/links JSON arrays are `[]`; tags/sources/links text fields are `""`.
- `documents` is the only canonical metadata table inside the derived cache; `sections` is the denormalized FTS/search-result projection table. It already contains tags, sources, links, and document context needed for normal result output; no separate `search_rows`, tag, source, or link tables are needed in the first cut. This denormalization is an intentional query-simplicity tradeoff.
- Do not expose or rely on line ranges. Public locations use document path, heading anchor, section ID, and snippet.
- `sections_fts` is an external-content FTS table over `sections`; rebuilds must explicitly run `INSERT INTO sections_fts(sections_fts) VALUES('rebuild')` after inserting sections.
- `.brain/ops.log`: optional later source for operation history search; not needed for first cut.

## Search result contract

Human output should be compact and cite exact locations:

```text
1. wiki/architecture/write-protocol.md#atomic-write-protocol
   title: Atomic write protocol
   kind: wiki
   match: ...one successful write creates one transaction...
   sources: sources/2026/05/04/raw-discussion.md
```

JSON output should be stable for agents:

```json
{
  "query": "atomic write protocol",
  "results": [
    {
      "id": "doc_...",
      "section_id": "doc_...#section-0003",
      "path": "wiki/architecture/write-protocol.md",
      "anchor": "atomic-write-protocol",
      "kind": "wiki",
      "title": "Atomic write protocol",
      "heading": "Atomic write protocol",
      "score": 1.23,
      "snippet": "...",
      "sources": ["sources/2026/05/04/raw-discussion.md"],
      "links": ["wiki/architecture/provenance.md"]
    }
  ],
  "recommended_read_order": [
    "wiki/architecture/write-protocol.md"
  ],
  "stop_rule": "Read the top 3 wiki pages first. Do not scan the repo unless those are insufficient."
}
```

## Agent search-first workflow

The goal of `lumbrera search` is to stop LLMs from doing broad repo exploration.

Generated query skills should instruct agents to:

1. Run `lumbrera search "<user question>" --json` before reading files.
2. Read only the top 3 wiki pages initially.
3. Do not scan the whole repo, run `find`, run broad `rg`, or read every `INDEX.md` entry unless search fails.
4. Stop reading when the answer is supported by the pages already read.
5. Read preserved sources only for numeric limits, operational/destructive actions, surprising claims, conflicts, uncertainty, or user-requested evidence.
6. If top results are insufficient, run one refined search query before broad exploration.
7. If broad repo exploration is still needed, state why search was insufficient before doing it.

Search output should reinforce this behavior with a small default limit, `recommended_read_order`, and a `stop_rule` field in JSON mode. `recommended_read_order` is deduplicated by document path. When wiki results exist, it is wiki-only and ordered by each wiki document's best-scoring matching section. For `--kind source` or source-only results, it contains source paths and the `stop_rule` should tell the agent to read those sources directly.

Search input should be sanitized natural language, not raw SQLite FTS5 syntax. Preserve quoted phrases after escaping, but treat boolean operators, `NEAR`, parentheses, column filters, and `*` as text unless a future explicit raw/prefix mode is added.

## Implementation plan

1. Add `modernc.org/sqlite` and an FTS5 availability test.
2. Create `internal/searchindex` schema: `meta`, `documents`, `sections`, `sections_fts`, minimal indexes, and schema version.
3. Implement deterministic fixture rebuild before real Markdown parsing; verify repeated rebuilds preserve `documents`, `sections`, rowids, and query order.
4. Add Markdown/frontmatter extraction: wiki frontmatter, body stripping, deterministic heading sections, and path-derived source IDs.
5. Implement real rebuild from sorted `wiki/` and `sources/` paths, insert `documents`/`sections`, explicitly populate external-content FTS with `INSERT INTO sections_fts(sections_fts) VALUES('rebuild')`, and write manifest metadata.
6. Implement staleness/status: manifest hash, schema version, fresh/missing/stale/incompatible states, and non-mutating status.
7. Add `lumbrera index --status` and `lumbrera index --rebuild` with clear failure messages.
8. Implement query API: sanitized natural-language FTS queries, `MATCH`, `snippet`, `bm25`, `--kind`/`--path`, and deterministic sort fallback.
9. Add `lumbrera search` human/JSON output, one-shot auto-rebuild for missing/stale indexes, and clear read-only/locked cache failure behavior.
10. Implement `recommended_read_order` deduped by document path: wiki-only when wiki results exist, source paths for `--kind source` or source-only results.
11. Update generated `AGENTS.md`, `.agents/skills/lumbrera-query/SKILL.md`, README command docs, and init scaffold `.gitignore` so generated brain repos ignore `.brain/search.sqlite*`.
12. Broaden tests for source indexing metadata defaults, returned tags/sources/links, no public line ranges, verification failures, read-only/locked cache failures, query sanitization, FTS external-content population, source-only read order, scaffold `.gitignore`, and ranking fixtures after the end-to-end path works.

## Deferred v2+ ideas

- Trim duplicated `sections` projection fields if rebuild size becomes annoying; first candidates are `title`, `summary`, `tags_json`, `sources_json`, and `links_json`.
- Optional semantic/vector search and embeddings.
- Existing Markdown repository import/adoption.
- Configurable directories, branches, lint policy, and source immutability policy.
- PDF/web/image/voice ingestion adapters that convert to Markdown before `lumbrera write`.
- Semantic lint suggestions for stale claims, duplicates, contradictions, weakly connected pages, and missing sources.
- LLM-assisted conflict reconciliation.
- Desktop/browser UI for browsing, capture, sync status, and review.

## Resolved v2 decisions

- `lumbrera index --rebuild` is the explicit rebuild path; `lumbrera search` may auto-rebuild missing/stale indexes but should not expose a first-cut `--rebuild` flag.
- Auto-rebuild is attempted once; read-only, locked, or unavailable cache failures must produce a clear error pointing to `lumbrera index --rebuild`.
- `lumbrera init` should scaffold `.gitignore` so generated brain repos ignore `.brain/search.sqlite*` because the SQLite database is a derived cache.
- Index all raw Markdown sources under `sources/`, not only sources referenced by wiki pages.
- First cut uses deterministic full rebuilds; incremental sync is deferred until measured rebuild cost justifies it.
- Section IDs are rebuild-stable cache locators, not edit-stable citations.
- Search queries are sanitized natural language by default, not raw SQLite FTS5 syntax.
- Use `modernc.org/sqlite`; release builds should include an FTS5 probe on supported target platforms.

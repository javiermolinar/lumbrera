# Lumbrera SQLite search plan

## Context

Lumbrera v1 has a managed Markdown brain with immutable raw sources, generated wiki frontmatter, stable document IDs, `BRAIN.sum`, compact read-only `tags.md`, generated metadata, deterministic verification, rollback on failed writes, and bundled ingest/query/lint skills.

The v2 milestone is a deterministic local SQLite-backed lexical search index plus two public commands:

- `lumbrera index` for explicit cache creation, rebuilds, status checks, and future schema maintenance.
- `lumbrera search` for compact agent-readable search over wiki synthesis, raw sources, headings, links, citations, and related documents.

The SQLite database is a disposable cache under `.brain/`, likely `.brain/search.sqlite`. Markdown files, generated Lumbrera document IDs, generated metadata, `tags.md`, `BRAIN.sum`, and `.brain/ops.log` remain canonical. The same canonical file tree must rebuild to equivalent database contents. Equivalence means same canonical inputs, same schema version, same normalized output bytes for `documents` and `sections`, and same ranked result order for identical queries. `lumbrera init` must scaffold `.gitignore` so generated brain repos ignore `.brain/search.sqlite*`.

FTS5 is lexical full-text search. It is not nearest-neighbor search, semantic search, or vector search. Embeddings and hybrid semantic retrieval remain deferred to v2+.

## Goals

- Add `lumbrera index` as the public command for index status and rebuilds.
- Add `lumbrera search` as the public command for querying the derived index.
- Use one primary lexical search operation for the agent workflow, followed by reading a small result set.
- Index both `wiki/` and `sources/` Markdown, split into deterministic sections by heading.
- Build `documents` from canonical wiki frontmatter and source metadata.
- Build denormalized `sections` from a deterministic Markdown parser/heading stream.
- Use stable generated Lumbrera document IDs from wiki frontmatter.
- Use deterministic path-derived IDs for sources.
- Use deterministic section IDs derived from document ID plus section ordinal. Section IDs are rebuild-stable for identical inputs, but not edit-stable when sections are inserted, deleted, or reordered earlier in a document.
- Support schema versioning, stale detection, automatic missing/stale rebuilds, and explicit rebuilds.
- Search titles, summaries, tags, source paths, links, headings, and body text.
- Prefer wiki synthesis in ranking while still exposing raw sources when evidence matters.
- Return compact human output and stable JSON output for agents.
- Reinforce a search-first workflow so agents avoid broad repo exploration.

## Non-goals

- No embeddings/vector search in this version.
- No nearest-neighbor/ANN index in this version.
- No incremental sync in the first cut; stale means full deterministic rebuild.
- No stable/public line ranges.
- No exact `--tag` or `--source` filter tables in the first cut.
- No separate graph/link tables in the first cut.
- No MCP server.
- No graph traversal command.
- No `.brain/ops.log` operation-history search in the first cut.
- No configurable indexing policy beyond the initial command flags.

## Proposed commands

```sh
lumbrera index --brain . --status
lumbrera index --brain . --rebuild
lumbrera search "tempo downscale" --brain .
lumbrera search "mimir limits" --kind wiki --json
lumbrera search "sources/azure_migration.md otlp gateway" --json
```

Search options:

```text
--brain <path>       target brain directory, default current directory
--limit <n>          max results, default 5, maximum 20
--kind <all|wiki|source>
--path <prefix>      restrict to a repo path prefix
--json               stable machine-readable output
```

Deferred search options, only if later evidence shows they are needed:

```text
--tag <tag>          exact tag filter
--source <path>      exact source-citation filter
```

Index options:

```text
--brain <path>       target brain directory, default current directory
--status             report whether the index exists, schema version, and staleness
--rebuild            force a full deterministic rebuild
```

Default behavior: `search` may automatically rebuild a missing or stale index through the shared indexer. Explicit repair and debugging should use `lumbrera index --status` and `lumbrera index --rebuild`. Auto-rebuild should be attempted once. If rebuild fails because the cache is unavailable, read-only, locked, or otherwise cannot be written, `search` should return a clear non-zero error telling the user to run `lumbrera index --rebuild`. Failed rebuilds should also point to `lumbrera verify` when deterministic integrity problems are detected.

## Agent search model

The LLM should not perform a sequence of database-style lookups. The intended workflow is:

1. Run one broad lexical search from the user question:

   ```sh
   lumbrera search "<question>" --json
   ```

2. Read the top 3 wiki pages from `recommended_read_order`.
3. Stop if those pages support the answer.
4. If insufficient, run one refined lexical search using better terms from the first results.
5. Only then consider broader exploration, and state why search was insufficient.

Tags, sources, and links are returned in search results as context. They help the agent understand the result and refine a second query if needed. They are not intended to drive multiple extra structured searches in the first cut.

## Determinism rules

- Treat SQLite as a derived cache only.
- Do not store timestamps or host-specific metadata in indexed content.
- Scan input paths in lexicographic order.
- Normalize paths to repository-relative slash paths.
- Normalize/sort tags, source paths, and links before storage.
- Build wiki document rows from generated frontmatter.
- Build source document rows from normalized path, content hash, and parsed title fallback.
- Build section rows from the Markdown parser, not regex line slicing.
- Use deterministic section IDs: `<document-id>#section-0001`, `<document-id>#section-0002`, etc.
- Section IDs are cache locators, not permanent citations. They are stable across rebuilds of the same input, but may change after edits that insert, delete, or reorder earlier sections in the same document.
- Use heading anchors generated by the same algorithm as verification.
- Do not expose line ranges in CLI or JSON output.
- Use deterministic ranking tie-breakers: score, kind boost, path, section ordinal, then section ID as the final fallback.
- Full rebuild of the same canonical files should produce equivalent tables and equivalent search results.
- Equivalent rebuild output means same canonical inputs, same schema version, same normalized output bytes for `documents` and `sections`, and same ranked result order for identical queries.
- `sections.rowid` determinism depends on fully deterministic rebuild order. Tests must enforce that sorted inputs produce identical rowids across rebuilds.

## Indexing model

`lumbrera index --rebuild` should:

1. Locate and validate the Lumbrera brain.
2. Run deterministic integrity checks or reuse existing verification code as appropriate.
3. Remove/recreate `.brain/search.sqlite` or rebuild all tables in a clean transaction.
4. Create schema at the current schema version.
5. Scan `wiki/` and `sources/` paths in sorted order.
6. Parse `wiki/` pages:
   - parse generated Lumbrera frontmatter into `documents`,
   - require generated ID/title/summary/1-5 tags,
   - normalize tags, source paths, and links into deterministic JSON and searchable text,
   - strip frontmatter before body indexing,
   - split body into deterministic heading sections.
7. Parse `sources/` Markdown:
   - derive a stable source document ID from normalized path,
   - preserve raw Markdown body text,
   - split by heading,
   - ignore unresolved links inside sources just like current verification does,
   - set all NOT NULL metadata fields deterministically: title is first H1, else first heading, else filename stem; summary is `""`; `tags_json`, `sources_json`, and `links_json` are `[]`; `tags_text`, `sources_text`, and `links_text` are `""`.
8. Insert `documents` and denormalized `sections` in deterministic order.
9. Populate the external-content FTS table from `sections` explicitly.
10. Store schema version and canonical manifest/staleness metadata.

`lumbrera index --status` should:

1. Locate the brain.
2. Check whether `.brain/search.sqlite` exists.
3. Report schema version and whether the index is fresh, missing, stale, or incompatible.
4. Avoid mutating files.

`lumbrera search` should:

1. Locate the brain and open the index.
2. Rebuild automatically once if the index is missing or stale.
3. If auto-rebuild fails because `.brain/search.sqlite` cannot be created, replaced, locked, or written, exit non-zero with a clear error and tell the user to run `lumbrera index --rebuild`.
4. Run the lexical FTS query with simple filters (`--kind`, `--path`) if provided.
5. Return compact ranked results with snippets, document locations, heading anchors, tags, links, and source citations from the matched section row.

## Staleness rules

For the first cut, stale detection should choose full rebuild rather than incremental updates.

An index is stale when:

- schema version differs from the current code,
- any indexed file path is added, removed, or renamed,
- any indexed file hash differs,
- canonical generated metadata used by the index differs,
- required meta keys are missing or invalid.

The manifest hash should be computed from an explicit canonical manifest containing:

- schema version,
- indexer version or parse-affecting rules version,
- Markdown section/anchor algorithm version,
- normalized sorted indexed path list,
- normalized path plus content hash for each indexed file,
- any generated frontmatter fields that affect indexed `documents` or `sections` output.

Store enough manifest detail, or a readable manifest debug string/hash input, to explain why `index --status` considers an index stale.

Incremental sync can be added later if full rebuild cost becomes painful, but it should preserve the same deterministic output contract.

## Schema sketch

Use `documents` as the only canonical metadata table inside the derived cache. Use `sections` as the denormalized search/result projection table and FTS content table. Keep `documents` for file-level state, staleness checks, grouping, and recommended read order. Do not add `search_rows`, `document_tags`, `document_sources`, or `links` tables in the first cut. Duplicating document metadata into `sections` is an intentional first-cut tradeoff for simple one-join query output, not a general normalization pattern.

```sql
meta(
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

documents(
  id TEXT PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,
  kind TEXT NOT NULL,          -- wiki or source
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  tags_json TEXT NOT NULL,     -- deterministic JSON array
  sources_json TEXT NOT NULL,  -- deterministic JSON array
  links_json TEXT NOT NULL,    -- deterministic JSON array
  tags_text TEXT NOT NULL,     -- normalized text for lexical search/display
  sources_text TEXT NOT NULL,  -- normalized text for lexical search/display
  links_text TEXT NOT NULL,    -- normalized text for lexical search/display
  hash TEXT NOT NULL,
  size_bytes INTEGER NOT NULL
);

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
);

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
);
```

Required `meta` keys:

```text
schema_version
manifest_hash
indexed_paths_hash
```

Useful indexes beyond primary/unique keys:

```sql
CREATE INDEX idx_documents_kind_path ON documents(kind, path);
CREATE INDEX idx_sections_document_ordinal ON sections(document_id, ordinal);
CREATE INDEX idx_sections_kind_path ON sections(kind, path);
CREATE INDEX idx_sections_path_ordinal ON sections(path, ordinal);
```

Avoid adding more B-tree indexes in the first cut. FTS handles text search, and extra section indexes will slow deterministic rebuilds without helping the common `sections_fts JOIN sections` query much.

Avoid `created_at`, `updated_at`, absolute paths, machine names, or other nondeterministic values.

## FTS external-content population

`sections_fts` uses `content='sections'`, so inserting rows into `sections` is not enough to populate the FTS index. Full rebuild must explicitly rebuild the FTS index after all `sections` rows are inserted:

```sql
INSERT INTO sections_fts(sections_fts) VALUES('rebuild');
```

Do not rely on implicit triggers in the first cut. Tests must include a fixture query with a unique term that would return zero rows if the FTS rebuild step were omitted, so this failure mode is observable.

## Query shape

The common search query should only join `sections_fts` to the denormalized `sections` content table. `documents` supports staleness checks, document grouping, and recommended read order; it is not needed for the normal top-N result projection.

```sql
SELECT
  s.path,
  s.id AS section_id,
  s.anchor,
  s.kind,
  s.title,
  s.heading,
  s.tags_json,
  s.sources_json,
  s.links_json,
  snippet(sections_fts, -1, '<<', '>>', '...', 16) AS snippet,
  bm25(sections_fts, 5.0, 3.0, 4.0, 2.0, 2.0, 1.5, 3.0, 1.0) AS score
FROM sections_fts
JOIN sections s ON s.rowid = sections_fts.rowid
WHERE sections_fts MATCH ?
ORDER BY
  score ASC,
  CASE s.kind WHEN 'wiki' THEN 0 ELSE 1 END,
  s.path ASC,
  s.ordinal ASC,
  s.id ASC
LIMIT ?;
```

The BM25 weights correspond to:

```text
title        5.0
path         3.0
summary      4.0
tags_text    2.0
sources_text 2.0
links_text   1.5
heading      3.0
body         1.0
```

## FTS query syntax

Default `lumbrera search` should accept natural language input, not raw SQLite FTS5 syntax.

First-cut query handling:

- Normalize and tokenize unquoted text into safe FTS terms.
- Preserve quoted substrings as exact phrases after escaping internal quotes.
- Treat boolean operators (`AND`, `OR`, `NOT`), `NEAR`, parentheses, column filters, and `*` as user text unless a later explicit raw/prefix mode is added.
- Return a clear error if sanitization leaves no searchable terms.
- Do not pass the user query directly to `MATCH` without sanitization.

This prevents accidental FTS syntax errors and keeps agent-generated queries predictable. Raw FTS syntax or prefix search can be added later behind an explicit flag if needed.

## Ranking

Start with FTS5 BM25 plus deterministic boosts:

- path/title/tag hits > summary hits > heading hits > body hits
- source path and link path hits are searchable text, not separate structured lookups
- wiki results get a small boost over sources
- source results still appear when the query clearly matches raw evidence
- ties break by path, section ordinal, and section ID as the final fallback
- default limit is 5, maximum is 20

Ranking is lexical. It only matches tokens and phrases accepted by the sanitized FTS query builder. It does not infer semantic similarity between unrelated terms.

## Human output contract

```text
1. wiki/architecture/write-protocol.md#atomic-write-protocol
   title: Atomic write protocol
   kind: wiki
   tags: architecture, writes
   match: ...one successful write creates one transaction...
   sources: sources/2026/05/04/raw-discussion.md
```

Do not include line ranges in default output. They are not stable enough for Lumbrera's index contract.

## JSON result contract

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
      "summary": "...",
      "tags": ["architecture", "writes"],
      "score": 1.23,
      "snippet": "...one successful write creates one transaction...",
      "sources": ["sources/2026/05/04/raw-discussion.md"],
      "links": ["wiki/architecture/provenance.md"]
    }
  ],
  "recommended_read_order": ["wiki/architecture/write-protocol.md"],
  "stop_rule": "Read the top 3 wiki pages first. Do not scan the repo unless those are insufficient."
}
```

The JSON contract intentionally omits line ranges. Use `path`, `anchor`, `section_id`, and `snippet` as the location fields.

`recommended_read_order` is deduplicated by document path, not section. When wiki results are present, it should contain one entry per wiki document, ordered by each document's best-scoring matching section after deterministic ranking. This prevents agents from reading multiple sections from the same page before moving to the next relevant page. If the search is `--kind source` or no wiki results are returned, `recommended_read_order` should contain deduplicated source paths ordered by their best-scoring matching section, and `stop_rule` should tell the agent to read those source results directly instead of scanning the repo.

## Agent workflow update

Generated query skill should say:

1. Run `lumbrera search "<question>" --json` before reading files.
2. Read only the top 3 wiki pages initially.
3. Do not scan the whole repo, run broad `find`, run broad `rg`, or read every `INDEX.md` entry unless search fails.
4. Stop once the answer is supported by the pages already read.
5. Read cited sources only for numeric limits, operational/destructive actions, surprising claims, conflicts, uncertainty, or requested evidence.
6. If top results are insufficient, run one refined search query before broad exploration.
7. If broad exploration is still needed, state why search was insufficient before doing it.

## Implementation order

1. Add SQLite dependency and FTS5 probe.
   - Add `modernc.org/sqlite`.
   - Add a small FTS5 availability test.

2. Create `internal/searchindex` schema.
   - Add `meta`, `documents`, `sections`, `sections_fts`, and the minimal B-tree indexes.
   - Add schema version constant and schema creation code.

3. Implement deterministic fixture rebuild.
   - Start with simple in-memory fixture documents/sections before wiring real Markdown parsing.
   - Verify two rebuilds create identical `documents`, `sections`, rowids, and query order.

4. Implement Markdown/frontmatter extraction.
   - Parse wiki frontmatter.
   - Strip frontmatter before body indexing.
   - Parse headings/sections deterministically.
   - Derive source IDs from normalized paths.

5. Implement real rebuild.
   - Scan sorted `wiki/` and `sources/` paths.
   - Insert `documents`.
   - Insert denormalized `sections`.
   - Populate FTS explicitly with `INSERT INTO sections_fts(sections_fts) VALUES('rebuild')`.
   - Write manifest metadata.

6. Implement staleness and status.
   - Compute manifest hash.
   - Check schema version.
   - Return fresh, missing, stale, or incompatible.
   - Keep status non-mutating.

7. Add `lumbrera index` CLI.
   - Implement `--status`.
   - Implement `--rebuild`.
   - Add clear failure messages.

8. Implement query API.
   - Sanitize natural-language queries for FTS5.
   - Run `MATCH`, `snippet`, and `bm25`.
   - Support simple `--kind` and `--path` filters.
   - Apply deterministic sort fallback.

9. Add `lumbrera search` CLI.
   - Add human output.
   - Add JSON output.
   - Auto-rebuild once if missing/stale.
   - Add clear read-only/locked cache failure path.

10. Implement recommended read order.
    - Deduplicate by document path.
    - Prefer wiki paths when wiki results exist.
    - Use source paths for `--kind source` or source-only result sets.
    - Order by each document's best-scoring matching section.

11. Update generated docs, skills, and scaffold ignores.
    - Update generated `AGENTS.md`.
    - Update `.agents/skills/lumbrera-query/SKILL.md`.
    - Update README command docs.
    - Update init scaffold templates so generated brain repos ignore `.brain/search.sqlite*` in `.gitignore`.

12. Broaden tests.
    - Add source indexing tests, including deterministic NOT NULL source metadata defaults.
    - Add returned tags/sources/links tests.
    - Add no-line-ranges tests.
    - Add verification failure behavior tests.
    - Add read-only/locked cache failure tests.
    - Add query sanitization tests.
    - Add FTS external-content population tests that fail if the explicit rebuild command is omitted.
    - Add source-only recommended read order tests.
    - Add scaffold `.gitignore` tests for `.brain/search.sqlite*`.
    - Add ranking fixture tests after the end-to-end path works.

## SQLite driver decision

Use `modernc.org/sqlite` for the first implementation.

Rationale:

- It is the Go module for the `gitlab.com/cznic/sqlite` project.
- It is CGO-free and better aligned with Lumbrera's standalone-binary goal.
- A local probe with `modernc.org/sqlite v1.50.0` reported SQLite `3.53.0` and `ENABLE_FTS5` in `PRAGMA compile_options`.
- `CREATE VIRTUAL TABLE ... USING fts5(...)` worked with `CGO_ENABLED=0`.

Driver import and open call:

```go
import (
    "database/sql"

    _ "modernc.org/sqlite"
)

db, err := sql.Open("sqlite", ".brain/search.sqlite")
```

Add a startup/test probe so failures are explicit:

```go
_, err := db.Exec(`CREATE VIRTUAL TABLE fts_probe USING fts5(body)`)
if err != nil {
    return fmt.Errorf("sqlite FTS5 unavailable: %w", err)
}
_, _ = db.Exec(`DROP TABLE fts_probe`)
```

Caveat: `modernc.org/sqlite` documents a fragile `modernc.org/libc` dependency. Keep module versions pinned through `go.mod`/`go.sum` and test release builds on supported target platforms.

## Deferred v2+ ideas

- Trim duplicated `sections` projection fields if rebuild size becomes annoying; first candidates are `title`, `summary`, `tags_json`, `sources_json`, and `links_json`.
- Exact `--tag` and `--source` filters if simple lexical search plus returned context is not enough.
- Separate link/citation tables if graph queries or exact citation filters become necessary.
- Optional semantic/vector search and embeddings.
- Existing Markdown repository import/adoption.
- Configurable directories, branches, lint policy, and source immutability policy.
- PDF/web/image/voice ingestion adapters that convert to Markdown before `lumbrera write`.
- Semantic lint suggestions for stale claims, duplicates, contradictions, weakly connected pages, and missing sources.
- LLM-assisted conflict reconciliation.
- Desktop/browser UI for browsing, capture, sync status, and review.

## Resolved decisions

- `lumbrera index --rebuild` is the explicit rebuild path.
- `lumbrera search` may auto-rebuild missing/stale indexes through the shared indexer, but should not expose a first-cut `--rebuild` flag.
- Auto-rebuild is attempted once; read-only, locked, or unavailable cache failures must produce a clear error pointing to `lumbrera index --rebuild`.
- `lumbrera init` should scaffold `.gitignore` so generated brain repos ignore `.brain/search.sqlite*` because the SQLite database is a derived cache.
- Index all raw Markdown sources under `sources/`, not only sources referenced by wiki pages.
- First cut uses deterministic full rebuilds; incremental sync is deferred until measured rebuild cost justifies it.
- Section IDs are rebuild-stable cache locators, not edit-stable citations.
- Search queries are sanitized natural language by default, not raw SQLite FTS5 syntax.
- `lumbrera index --status` should avoid mutation and report cache freshness/incompatibility. If deeper integrity problems block indexing, point to `lumbrera verify`.
- Use `modernc.org/sqlite`; release builds should include an FTS5 probe on supported target platforms.

# Lumbrera next

## Context

The old `2026-05-02-lumbrera-plan.md` has been renamed to this `next.md`. The current implementation has the local v1 shape mostly in place: `init`, `write`, and `verify`; generated `INDEX.md`, `CHANGELOG.md`, `BRAIN.sum`, and read-only `tags.md`; immutable raw sources; stable generated wiki document IDs; mandatory single-line wiki summary, 1-5 tags, and 400-line max wiki body; generated wiki frontmatter and source sections; link, heading-anchor, and inline source-citation validation; rollback on failed writes; and bundled ingest/query/lint skills.

The next milestone is v2: make Lumbrera easier for LLM agents to query by adding a derived SQLite search index and a new `lumbrera search` command.

## V2 goal

Build a nice local indexing and search tool backed by SQLite so agents can quickly find relevant wiki pages, source evidence, headings, links, citations, and related documents before deciding what to read in full.

The index is a derived cache, not source of truth. Markdown files, generated Lumbrera document IDs, generated metadata, `tags.md`, and `.brain/ops.log` remain canonical. The SQLite database must be safe to delete and rebuild. `tags.md` is itself generated from wiki frontmatter and read-only for agents.

## What is missing for v2

- SQLite-backed derived index stored under `.brain/`, likely `.brain/search.sqlite`.
- Incremental updates keyed by stable generated Lumbrera document IDs in wiki frontmatter; `verify` repairs missing IDs in older wiki pages.
- Index schema versioning and rebuild/staleness detection.
- Markdown section extraction for both `wiki/` and `sources/`; wiki pages are intentionally capped at 400 body lines so section indexing stays predictable.
- Full-text search over titles, summaries, tags, headings, body text, paths, and source citations.
- Search ranking that prefers wiki synthesis first, uses path/title/summary/tags for document routing and embeddings, but still exposes raw sources when evidence matters.
- Agent-readable `lumbrera search` command output with citations and stable JSON mode.
- Query-skill updates so agents use search before broad file reads.
- Tests for indexing, stale rebuilds, search ranking, CLI output, and generated scaffold docs.

## Proposed command

```sh
lumbrera search "atomic write protocol" --brain ./brain --limit 8
lumbrera search "source immutability" --kind wiki --json
lumbrera search "SQLite index" --rebuild
```

Options to consider:

```text
--brain <path>       target brain directory, default current directory
--limit <n>          max results, default 10
--kind <all|wiki|source>
--path <prefix>      restrict to a repo path prefix
--tag <tag>          restrict wiki documents by generated tag
--source <path>      restrict wiki results to pages citing a source
--json               machine-readable output for agents
--rebuild            force rebuild before searching
```

Default behavior: if the index is missing or stale, `search` rebuilds it automatically. Failed rebuilds should exit non-zero and point to `lumbrera verify` if the brain has deterministic integrity problems.

## Proposed index shape

Use SQLite FTS5 if available through the chosen Go driver. Prefer a pure-Go SQLite driver if practical so Lumbrera stays easy to ship as a standalone binary.

Core tables:

```sql
meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)
documents(
  id TEXT PRIMARY KEY,         -- generated Lumbrera document id from frontmatter
  path TEXT UNIQUE NOT NULL,
  kind TEXT NOT NULL,          -- wiki or source
  title TEXT NOT NULL,
  summary TEXT,
  tags TEXT,                   -- JSON array or comma-separated normalized tags
  hash TEXT NOT NULL,
  size_bytes INTEGER NOT NULL
)
sections(
  id INTEGER PRIMARY KEY,
  document_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  heading TEXT,
  anchor TEXT,
  level INTEGER,
  start_line INTEGER,
  end_line INTEGER,
  body TEXT NOT NULL
)
links(
  id INTEGER PRIMARY KEY,
  from_path TEXT NOT NULL,
  to_path TEXT NOT NULL,
  anchor TEXT,
  kind TEXT NOT NULL           -- wiki_link, source_link, citation
)
sections_fts USING fts5(title, path, heading, body, tags, content='sections', content_rowid='id')
```

Index inputs:

- `wiki/`: strip generated Lumbrera frontmatter, require generated id/title/summary/1-5 tags, preserve id/title/summary/tags/sources/links from frontmatter, split body by headings.
- `sources/`: preserve raw Markdown, split by headings, ignore unresolved links inside sources just like current verification does.
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
  ]
}
```

## Implementation plan

1. Add `internal/searchindex` with schema creation, schema version metadata, rebuild, stale detection, and query APIs.
2. Add Markdown section extraction utilities, reusing existing Markdown analysis where possible.
3. Add SQLite dependency and confirm FTS5 support in tests.
4. Implement `lumbrera search` argument parsing, help text, human output, and JSON output.
5. Make search rebuild missing/stale indexes automatically and support explicit `--rebuild`.
6. Add ranking: title/path/tag hits > heading hits > body hits; wiki results slightly boosted over sources; exact path and source-citation matches boosted.
7. Update generated `AGENTS.md` and `.agents/skills/lumbrera-query/SKILL.md` so agents start with `lumbrera search`, then read the best files fully.
8. Update README command docs.
9. Add tests with fixture brains covering wiki/source indexing, heading anchors, tags, source filters, stale rebuilds, JSON output, and verification failure behavior.

## Deferred v2+ ideas

- Optional semantic/vector search and embeddings.
- Existing Markdown repository import/adoption.
- Configurable directories, branches, lint policy, and source immutability policy.
- PDF/web/image/voice ingestion adapters that convert to Markdown before `lumbrera write`.
- Semantic lint suggestions for stale claims, duplicates, contradictions, weakly connected pages, and missing sources.
- LLM-assisted conflict reconciliation.
- Desktop/browser UI for browsing, capture, sync status, and review.

## Open questions

- Should v2 expose a separate `lumbrera index` command, or keep indexing private behind `lumbrera search --rebuild`?
- Should `.brain/search.sqlite*` be ignored automatically in generated brain repos?
- Should search include all raw sources, or only sources referenced by at least one wiki page?
- Should search snippets include line ranges even though Markdown line mapping can be approximate after frontmatter stripping?
- Which SQLite Go driver best fits standalone distribution and FTS5 support?

# Lumbrera next

## Context

Lumbrera v2's core SQLite lexical search milestone is mostly implemented. The CLI includes deterministic SQLite/FTS5 indexing under `.brain/search.sqlite`, `lumbrera index --status`, `lumbrera index --rebuild`, JSON `lumbrera search`, stale/missing auto-rebuild, wiki/source Markdown extraction, generated search-first agent guidance, and scaffold `.gitignore` coverage for `.brain/search.sqlite*`.

This file is now the active tracker for consolidated v2+ backlog ideas. The older SQLite search plan has been removed.

## Consolidated deferred v2+ backlog

### Search and index evolution

- Exact `--tag` and `--source` filters if lexical search plus returned context is not enough.
- Optional raw FTS or prefix-search mode behind an explicit flag.
- Separate link/citation tables if graph queries or exact citation filters become necessary.
- Graph traversal command if agents need structured neighborhood exploration.
- `.brain/ops.log` operation-history search.
- Incremental sync if measured full rebuild cost becomes painful.
- Configurable indexing policy beyond first-cut flags.
- Trim duplicated `sections` projection fields if rebuild size becomes annoying; first candidates are `title`, `summary`, `tags_json`, `sources_json`, and `links_json`.
- MCP server only if there is a concrete integration need.

### Retrieval intelligence

- Search-powered health/curation workflow: generated lint/health workflow uses ranked related documents as LLM review context for duplicates, contradictions, stale claims, missing cross-links, orphan pages, missing concept pages, and source gaps before any `lumbrera write` mutation.
- Optional semantic/vector search and embeddings.
- Hybrid lexical plus semantic retrieval.
- Semantic lint suggestions for stale claims, duplicates, contradictions, weakly connected pages, and missing sources.
- LLM-assisted conflict reconciliation.

### Repository and product expansion

- Existing Markdown repository import/adoption.
- Configurable directories, branches, lint policy, and source immutability policy.
- PDF/web/image/voice ingestion adapters that convert to Markdown before `lumbrera write`.
- Desktop/browser UI for browsing, capture, sync status, and review.

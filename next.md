# Lumbrera next

## Context

Lumbrera now has deterministic SQLite/FTS5 search and a first-cut health workflow. The CLI includes deterministic indexing under `.brain/search.sqlite`, `lumbrera index`, JSON `lumbrera search`, and read-only `lumbrera health` candidates for LLM review. Relationship facts (`document_links`, `document_citations`, and `document_tags`) and generated wiki `modified_date` metadata support explainable health candidates.

This file is the active tracker for consolidated v2+ backlog ideas. Completed implementation plans should be folded into this file and removed.

## Priority backlog

### P0 — Stabilize health output contracts

- Add formal compact human output fixtures for `lumbrera health` if humans or agents begin relying on exact text shape.
- Keep `candidates` terminology and avoid `drifted_pages` or other conclusion-shaped fields.

### P1 — Improve health candidate quality from real review feedback

- Continue ranking tuning only when real candidate review exposes noisy reason combinations.
- Consider splitting display semantics into relation strength vs action priority if `score` remains confusing.
- Downrank broad same-source/same-product relationships further if they do not produce useful links.
- Add page-neighborhood UX if global medium-confidence `missing_link` review feels repetitive.
- Consider persistent skip/ignore state only if repeated false positives become a practical problem.

### P2 — Health workflow expansion

- Source freshness: derive later from source path conventions, explicit source metadata, or `.brain/ops.log` indexing if source staleness becomes important.
- Consolidation moves: defer a dedicated move command until there is evidence that identity-preserving renames are needed. A true move would need multi-file atomicity, document ID preservation, and link rewriting.
- Cluster detection: intentionally deferred; first-cut health remains single-page and page-pair candidates.

## Consolidated deferred v2+ backlog

### Search and index evolution

- Optional raw FTS or prefix-search mode behind an explicit flag.
- Graph traversal command if agents need structured neighborhood exploration.
- `.brain/ops.log` operation-history search.
- Incremental sync if measured full rebuild cost becomes painful.
- Configurable indexing policy beyond first-cut flags.
- Trim duplicated `sections` projection fields if rebuild size becomes annoying; first candidates are `title`, `summary`, `tags_json`, `sources_json`, and `links_json`.
- MCP server only if there is a concrete integration need.

### Retrieval intelligence

- Optional semantic/vector search and embeddings.
- Hybrid lexical plus semantic retrieval.
- LLM-assisted conflict reconciliation.

### Repository and product expansion

- Existing Markdown repository import/adoption.
- Configurable directories, branches, lint policy, and source immutability policy.
- PDF/web/image/voice ingestion adapters that convert to Markdown before `lumbrera write`.
- Desktop/browser UI for browsing, capture, sync status, and review.

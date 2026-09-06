# Lumbrera Brain Agent Guide

This is a Lumbrera brain: a managed Markdown knowledge base for humans and LLM agents.

## Read

- Use .agents/skills/lumbrera-query/SKILL.md when answering questions.
- Run one intent-preserving `lumbrera search` before reading files; rewrite conversational questions into stable searchable concepts instead of blindly using exact wording.
- Treat recommended_sections as the primary read plan. Read those path#anchor targets first, then the top wiki or note entries from recommended_read_order only if more context is needed.
- Check coverage on comparison/entity questions; if a named entity is missing, say so or refine the search before answering.
- Do not scan the whole repo, run broad find/rg, or read every INDEX.md entry unless search is insufficient.
- Use INDEX.md, SOURCES.md, NOTES.md, ASSETS.md, and tags.md for fallback navigation, not as evidence.

## Write

- Do not create, edit, move, delete, or overwrite files directly.
- All mutations to sources/, notes/, wiki/, and assets/ must use lumbrera write or lumbrera delete.
- Do not modify existing files under sources/; sources are immutable.
- Do not modify existing files under assets/; assets are immutable. To replace an asset, delete it and re-add it.
- Asset writes require --file to copy a local file: lumbrera write assets/<path> --file <local-path> --reason "..."
- Do not edit generated files: INDEX.md, SOURCES.md, NOTES.md, ASSETS.md, CHANGELOG.md, BRAIN.sum, or tags.md.
- Do not edit Lumbrera internals under .brain/, .agents/, or .claude.
- .brain/search.sqlite is a disposable generated cache; rebuild it with lumbrera index, do not edit or cite it.

## Source tiers

Sources, notes, and wiki pages are ranked by tier, inferred from path:

- `sources/`, `notes/`, and `wiki/` — canonical (current product docs, first-party knowledge, operations, reference). Default.
- `sources/design/` and `wiki/design/` — design proposals, ADRs, specs. Not implemented.
- `sources/reference/` — preserved context that rarely surfaces (historical, competition, meeting notes).

Search ranks canonical content above design and reference. Use `--tier` to filter search results.

When ingesting design docs, preserve under `sources/design/` and create wiki pages under `wiki/design/` with tags like `design` and `draft`. Mark the wiki body with the proposal status. Do not tag design pages with `operations` or `architecture`.

## Wiki style

- Atomic, evidence-grounded, searchable, and useful without reopening the evidence.
- One durable concept, task, runbook, decision, or reference per page; not organized by source chunk.
- Clear title, single-line summary, 1-5 lowercase slug tags, and a stable path.
- Every page under the hard maximum of 400 Markdown body lines.
- Use precise evidence citations for important claims and real managed-document links for related knowledge.
- Troubleshooting/runbooks prefer symptom → cause → fix structure.
- Split when a draft covers multiple concepts, has independent sections, needs more than 5 tags, or becomes a grab bag.
- Every new wiki page should link to a related wiki page or note unless genuinely standalone.
- Prefer inline links; use a short "Related pages" section only when inline links are awkward.
- Reuse existing tags from tags.md when they fit.
- Every wiki page must retain at least one evidence path under `sources/` or `notes/`.
- Lumbrera generates document IDs, frontmatter, Sources sections, indexes, changelog, checksums, and the tag registry.

## Note style

- Record one durable first-party observation, decision, failed experiment, or local convention per note.
- Search first and skip the note when a wiki page or existing note already covers the fact.
- Notes require a clear title, single-line summary, 1-5 lowercase slug tags, and at most 400 Markdown body lines.
- Notes do not accept `--source` and must not contain a `## Sources` section.
- Use ordinary links to connect related notes and wiki pages.
- Prefer zero notes over transcript dumps, session recaps, speculation, or low-value reminders.

## Inline evidence citations

~~~md
Important claim [source: ../sources/example.md#heading-anchor].
First-party claim [source: ../notes/observation.md#heading-anchor].
~~~

- Use for operationally important, numeric, destructive, version-sensitive, surprising, or easily disputed claims.
- Prefer stable heading anchors over line numbers.
- Pass file-level evidence with `lumbrera write --source`; paths may identify sources or notes, and inline citations complement it.
- Do not add citations to every sentence.

## Commands

~~~sh
lumbrera search "question" --brain . --json
lumbrera search "question" --tier design --json
lumbrera index --status --brain .
lumbrera index --rebuild --brain .
lumbrera verify --brain .
lumbrera write sources/<path>.md --reason "Preserve source" < source.md
lumbrera write notes/<path>.md --title "Title" --summary "Summary" --tag tag --reason "Record first-party knowledge" < note.md
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<path>.md --reason "Distill source" < page.md
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source notes/<path>.md --reason "Synthesize note evidence" < page.md
lumbrera write assets/<path> --file <local-path> --reason "Add diagram"
lumbrera delete sources/<path>.md --reason "Remove bad source"
lumbrera delete notes/<path>.md --reason "Remove obsolete note"
lumbrera delete wiki/<path>.md --reason "Remove obsolete page"
lumbrera delete assets/<path> --reason "Remove outdated asset"
lumbrera migrate --brain . # upgrade a v1 or v2 brain to v3
~~~

## Team Git/GitHub errors

- Git/GitHub is external coordination; `lumbrera write` and `lumbrera delete` remain the only content mutation paths.
- If commit, pull, merge, rebase, or push fails, stop and report the exact error and repository state.
- Do not resolve merge conflicts by directly editing sources/, notes/, wiki/, generated files, or Lumbrera internals.
- Do not commit conflict markers.
- Prefer returning to a clean tree, updating from remote, rerunning the Lumbrera operation, then running lumbrera verify.
- If a wiki conflict needs semantic resolution, ask for human direction; the final resolved wiki content must still be written through lumbrera write.
- Before any commit or push, run lumbrera verify --brain . and require it to pass.

## Skills

- Ingest sources into wiki pages: .agents/skills/lumbrera-ingest/SKILL.md
- Record durable first-party knowledge: .agents/skills/lumbrera-note/SKILL.md
- Answer questions from the brain: .agents/skills/lumbrera-query/SKILL.md
- Check semantic health: .agents/skills/lumbrera-health/SKILL.md
- Delete sources, notes, wiki pages, or assets: .agents/skills/lumbrera-delete/SKILL.md

# Lumbrera Brain Agent Guide

This is a Lumbrera brain: a managed Markdown knowledge base for humans and LLM agents.

## Read

- Use .agents/skills/lumbrera-query/SKILL.md when answering questions.
- Run lumbrera search "<question>" --json before reading files.
- Treat recommended_sections as the primary read plan. Read those path#anchor targets first, then the top wiki pages from recommended_read_order only if more context is needed.
- Check coverage on comparison/entity questions; if a named entity is missing, say so or refine the search before answering.
- Do not scan the whole repo, run broad find/rg, or read every INDEX.md entry unless search is insufficient.
- Use INDEX.md and tags.md for fallback navigation, not as source evidence.

## Write

- Do not create, edit, move, delete, or overwrite files directly.
- All mutations to sources/ and wiki/ must use lumbrera write.
- Do not modify existing files under sources/; sources are immutable.
- Do not edit generated files: INDEX.md, CHANGELOG.md, BRAIN.sum, or tags.md.
- Do not edit Lumbrera internals under .brain/, .agents/, or .claude.
- .brain/search.sqlite is a disposable generated cache; rebuild it with lumbrera index, do not edit or cite it.

## Wiki rules

- Wiki pages require a title, single-line summary, 1-5 lowercase slug tags, source references, and at most 400 Markdown body lines.
- Lumbrera generates document IDs, frontmatter, source sections, index, changelog, checksums, and the tag registry.
- Read tags.md before wiki writes and reuse existing tags when they fit.

## Commands

~~~sh
lumbrera search "question" --brain . --json
lumbrera index --status --brain .
lumbrera index --rebuild --brain .
lumbrera verify --brain .
lumbrera write sources/<path>.md --reason "Preserve source" < source.md
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<path>.md --reason "Distill source" < page.md
~~~

## Team Git/GitHub errors

- Git/GitHub is external coordination; lumbrera write remains the only content mutation path.
- If commit, pull, merge, rebase, or push fails, stop and report the exact error and repository state.
- Do not resolve merge conflicts by directly editing sources/, wiki/, generated files, or Lumbrera internals.
- Do not commit conflict markers.
- Prefer returning to a clean tree, updating from remote, rerunning the Lumbrera operation, then running lumbrera verify.
- If a wiki conflict needs semantic resolution, ask for human direction; the final resolved wiki content must still be written through lumbrera write.
- Before any commit or push, run lumbrera verify --brain . and require it to pass.

## Skills

- Ingest sources into wiki pages: .agents/skills/lumbrera-ingest/SKILL.md
- Answer questions from the brain: .agents/skills/lumbrera-query/SKILL.md
- Check semantic health: .agents/skills/lumbrera-lint/SKILL.md

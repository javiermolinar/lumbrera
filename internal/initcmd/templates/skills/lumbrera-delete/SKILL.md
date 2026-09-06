---
name: lumbrera-delete
description: Delete a source, note, wiki page, or asset from a Lumbrera brain with evidence cascades and managed-link cleanup.
---

# Lumbrera Delete

Use when asked to remove content from a Lumbrera brain.

## Purpose

Remove one content path while keeping every surviving managed document valid. Lumbrera plans evidence cascades and ordinary-link cleanup before mutation, regenerates derived files, and rolls back the complete operation if verification fails.

## Contract

- Do not delete files directly; use only `lumbrera delete`.
- Always provide a single-line `--reason`.
- Confirm with the user before deletion, especially when a note or source may be the sole evidence for a wiki page.
- Run `lumbrera verify` afterward; the delete command already does this automatically.

## Cascade behavior

### Note deletion

1. Remove the note from every wiki page's `lumbrera.sources` metadata and generated `## Sources` section.
2. Strip matching inline `[source: ...]` citations.
3. Cascade-delete a wiki page when the note was its last remaining evidence path.
4. Remove ordinary inbound links to the deleted note and any cascade-deleted wiki pages from surviving notes and wiki pages.
5. Regenerate `NOTES.md`, `INDEX.md`, `BRAIN.sum`, and `tags.md`.

A wiki backed by other source or note evidence survives.

### Source deletion

1. Remove the source from every wiki page's evidence metadata and generated Sources section.
2. Strip matching inline citations.
3. Cascade-delete wiki pages left with no source or note evidence.
4. Remove ordinary inbound links to the deleted source and any cascade-deleted wiki pages.

Notes do not use sources as evidence, but ordinary links from notes to a deleted source are cleaned.

### Wiki deletion

- Remove ordinary inbound links from both wiki pages and notes.
- Wiki-to-wiki or note-to-wiki link cleanup does not cause another evidence cascade.

### Asset deletion

- Remove image embeds and regular links to the asset from both wiki pages and notes.
- Never cascade-delete a managed document because an asset was removed.
- Review affected prose afterward for text that no longer reads cleanly.

## Workflow

1. Identify the exact path and reason.
2. Search for the target to understand likely evidence and link impact:

   ~~~sh
   lumbrera search "<target title and concepts>" --brain . --json
   ~~~

3. Tell the user which pages may be rewritten or cascade-deleted, then obtain approval.
4. Run the delete:

   ~~~sh
   lumbrera delete notes/<path>.md --reason "Remove obsolete note"
   lumbrera delete sources/<path>.md --reason "Remove bad source"
   lumbrera delete wiki/<path>.md --reason "Remove obsolete page"
   lumbrera delete assets/<path> --reason "Remove outdated asset"
   ~~~

5. Review command output and run `lumbrera verify --brain .` if an explicit confirmation is useful.
6. Report deleted paths, cascade deletions, rewritten managed documents, and verification status.

## Guardrails

- Prefer consolidation before deleting knowledge that remains useful.
- Do not manually remove frontmatter evidence, Sources entries, citations, links, or generated-file entries.
- Do not use `lumbrera write --delete`; it is deprecated and delegates to `lumbrera delete`.
- Treat the operation as atomic. If the command reports rollback failure, stop and report the exact repository state.

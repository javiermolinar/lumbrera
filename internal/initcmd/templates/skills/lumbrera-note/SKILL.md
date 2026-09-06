---
name: lumbrera-note
description: Record one durable first-party observation, decision, failed experiment, or local convention as a focused Lumbrera note.
---

# Lumbrera Note

Use when the user asks to preserve durable knowledge learned directly through work. Use `lumbrera-ingest` when the knowledge comes from an external source document.

## Workflow

1. Distill one reusable observation, decision, failed experiment, local convention, or equivalent fact.
2. Search existing wiki and source coverage, then existing notes:

   ~~~sh
   lumbrera search "<stable concepts and entities>" --brain . --json
   lumbrera search "<stable concepts and entities>" --brain . --kind note --json
   ~~~

3. Read `recommended_sections`, then the top entries from `recommended_read_order` when needed.
4. When existing coverage matches, report its path as the outcome.
5. Otherwise create one focused note or update the clearly matching note:

   ~~~sh
   lumbrera write notes/<path>.md --brain . --title "Title" --summary "Single-line summary" --tag <tag> --reason "Record first-party knowledge" < note.md
   ~~~

6. Connect related notes and wiki pages with ordinary Markdown links.
7. When notes support stable reusable guidance, synthesize or update a wiki page with each note path passed as `--source`.
8. Run `lumbrera verify --brain .` and report the note created, updated, or reused.

## Note shape

- Capture one durable fact with likely future use.
- Supply body Markdown; Lumbrera manages frontmatter, catalogs, and generated metadata.
- Use a clear title, a single-line summary, and 1-5 lowercase slug tags.
- Keep the body under 400 lines.
- Use `lumbrera write` for changes and review the `lumbrera delete` cascade plan before removal.

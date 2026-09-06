---
name: lumbrera-note
description: Record one durable first-party observation, decision, failed experiment, or local convention as a focused Lumbrera note.
---

# Lumbrera Note

Use when the user asks to preserve durable first-party knowledge that has no external source document to ingest.

## Goal

Record one focused fact only when the brain does not already cover it. Prefer zero notes over a low-value note, transcript dump, or session recap.

## Workflow

1. Identify one durable first-party fact: an observation, decision, failed experiment, local convention, or equivalent reusable knowledge.
2. Search all content for existing coverage:

   ~~~sh
   lumbrera search "<stable concepts and entities>" --brain . --json
   ~~~

3. Read only `recommended_sections`, then the top entries from `recommended_read_order` if needed.
4. Skip the write when a wiki page or existing note already covers the fact. Report the matching path.
5. Otherwise create one focused note, or update the clearly matching existing note:

   ~~~sh
   lumbrera write notes/<path>.md --title "Title" --summary "Single-line summary" --tag <tag> --reason "Record first-party knowledge" < note.md
   ~~~

6. When one or more notes now support stable reusable guidance, synthesize or update a wiki page with each note path passed as `--source`. A wiki page may be backed only by notes.
7. Run `lumbrera verify --brain .` and report the note created, updated, or skipped.

## Contract

- Do not ingest external source material through this skill. Use the ingest skill for source documents.
- Do not write transcript dumps, session recaps, speculative reminders, or facts with no likely future use.
- Supply body Markdown only. Do not include frontmatter or a `## Sources` section.
- Do not pass `--source` when writing a note.
- Use ordinary Markdown links to connect related notes and wiki pages.
- Do not delete a canonicalized note when it remains the wiki page's only evidence or contains unique supporting detail; use `lumbrera delete` only after reviewing the cascade plan.
- Keep each note under 400 Markdown body lines with a clear title, single-line summary, and 1-5 lowercase slug tags.
- Mutate only through `lumbrera write`; never edit note or generated files directly.

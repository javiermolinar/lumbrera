---
name: lumbrera-ingest
description: Ingest referenced Markdown source material into a Lumbrera LLM Wiki by preserving raw sources, searching for overlap, and writing distilled wiki knowledge through lumbrera write.
---

# Lumbrera Ingest

Use when asked to turn source material into durable wiki pages.

## Purpose

Preserve raw source material and distill it into small, source-grounded wiki pages that improve the brain without creating avoidable duplicates or semantic drift.

## Expected output

- One or more created or updated wiki pages, or an explicit skip decision when the source is already covered.
- A final report listing overlap searches performed, existing pages reviewed, created/updated/skipped pages, covered source sections, skipped source sections, uncertainties, and follow-up work.
- All mutations performed only through `lumbrera write`, followed by `lumbrera verify`.

## Contract

- Do not edit files directly; write only with `lumbrera write`.
- Preserve raw source material; do not alter existing `sources/` files.
- Provide wiki body Markdown only. Lumbrera generates document IDs, frontmatter, Sources sections, indexes, changelog, checksums, and tags.
- Prefer creating a new focused page over mutating an existing page unless search shows a clear same-topic canonical page that should absorb the new source.

## Workflow

1. Read the source.
2. Draft candidate wiki pages by durable topic, not by source chunk. Identify title, summary, tags, key entities, and source anchors.
3. Search for overlap before writing:

   ~~~sh
   lumbrera search "<candidate title summary key entities>" --json
   lumbrera search "<candidate key terms>" --tag <candidate-tag> --json
   ~~~

4. Read `recommended_sections` first, then top wiki pages from `recommended_read_order` only if needed.
5. Decide per candidate: update existing page, create new page, create linked page, or skip.
6. Write with `lumbrera write`, then run `lumbrera verify`.
7. Report overlap decisions, pages changed, source coverage, skipped material, uncertainties, and follow-up work.

## Write command

For a new wiki page, pass --title, --summary, 1-5 --tag flags, --source, and --reason:

~~~sh
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<source>.md --reason "Distill source" < page.md
~~~

## Delete command

If a source is bad, incorrect, or superseded, use `lumbrera delete` to remove it and cascade-clean all wiki pages that reference it. Wiki pages left with zero sources are automatically deleted.

~~~sh
lumbrera delete sources/<path>.md --reason "Remove bad source"
~~~

Use the dedicated delete skill (.agents/skills/lumbrera-delete/SKILL.md) when removing sources or wiki pages.

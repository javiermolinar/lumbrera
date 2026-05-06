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
- A final report listing overlap searches performed, existing pages reviewed, created/updated/skipped pages, covered source sections, skipped source sections, uncertainties, and follow-up pages or questions.
- All mutations performed only through `lumbrera write`, followed by `lumbrera verify`.

## Contract

- Do not edit files directly; write only with `lumbrera write`.
- Preserve raw source material; do not alter existing `sources/` files.
- Provide wiki body Markdown only. Lumbrera generates document IDs, frontmatter, Sources sections, indexes, changelog, checksums, and tags.
- Prefer creating a new focused page over mutating an existing page unless search shows a clear same-topic canonical page that should absorb the new source.

## Good wiki page

- Atomic, source-grounded, searchable, and useful without reopening the source.
- Covers one durable concept, task, runbook, decision, or reference; it is not organized by source chunk.
- Has a clear title, a single-line summary, 1-5 lowercase slug tags, and a stable path.
- Uses precise source citations for important claims and real wiki links for related knowledge.
- Troubleshooting/runbooks prefer symptom → cause → fix structure.
- Split when a draft covers multiple concepts, has independent sections, needs more than 5 tags, or becomes a grab bag.
- Avoid pages like "everything from source X", "part 1", giant mixed-topic notes, filler tags, unsupported synthesis, or duplicates of existing pages.

## Workflow

1. Read the source. Chunk large sources only for reading.
2. Draft candidate wiki pages by durable topic/task, not by source chunk. For each candidate, identify the title, summary, tags, key entities, key claims, and source anchors.
3. Search for overlap before writing:

   ~~~sh
   lumbrera search "<candidate title summary key entities>" --json
   lumbrera search "<candidate tags and key claim terms>" --json
   lumbrera search "<candidate key terms>" --tag <candidate-tag> --json
   lumbrera search "<candidate key terms>" --source sources/<source>.md --json
   ~~~

4. Use exact `--tag` and `--source` filters only when they narrow the review set; broad searches still come first so differently tagged or uncited overlap is not missed.
5. Read `recommended_sections` first, then the top wiki pages from `recommended_read_order` only if needed. Use `INDEX.md` and `tags.md` as fallback navigation and tag reference, not as a substitute for search.
6. Make an explicit overlap decision for each candidate:
   - update an existing page only when it is clearly the same canonical topic and the source adds durable value;
   - create a new page when the topic is distinct;
   - create a new linked page when the topic is related but should stay separate;
   - skip when the existing wiki already covers the source with enough fidelity.
7. Draft small pages and keep every wiki page under the hard maximum of 400 Markdown body lines.
8. Choose a clear title, single-line summary, and 1-5 `--tag` values, reusing `tags.md` when possible.
9. Add real wiki links and precise source citations where needed.
10. Write with `lumbrera write`, then run `lumbrera verify`.
11. Report the overlap decision, pages changed, source coverage, skipped material, uncertainties, and follow-up work.

## Links

- From the overlap search results, identify 3-7 existing wiki pages that may overlap, depend on this page, act as prerequisites, or deserve cross-links.
- Add contextual wiki links for real relationships: prerequisite, related task, deeper reference, operational follow-up, or contrasting behavior.
- Prefer inline links; use a short "Related pages" section only when inline links are awkward.
- Every new wiki page should have at least one wiki link unless genuinely standalone. If no links are added, explain why in the final report.

## Inline source citations

Inline citations are allowed and encouraged when exact provenance matters.

~~~md
Large Mimir series-limit increases should be reviewed against ingester capacity
[source: ../sources/mimir-docs.compact.md#reviewing-changes-to-per-tenant-limits].
~~~

- Prefer stable heading anchors over line numbers.
- Use inline citations for operationally important, numeric, destructive, version-sensitive, surprising, customer-impacting, or easily disputed claims.
- Still pass file-level provenance with `lumbrera write --source`; inline citations complement it, they do not replace it.
- Do not add citations to every sentence.

## Write command

For a new wiki page, pass --title, --summary, 1-5 --tag flags, --source, and --reason:

~~~sh
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<source>.md --reason "Distill source" < page.md
~~~

After writing, run `lumbrera verify` and report created/updated pages, overlap decisions, covered source sections, skipped sections, uncertainties, and follow-up pages.

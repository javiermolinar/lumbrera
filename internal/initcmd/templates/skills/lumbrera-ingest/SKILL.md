---
name: lumbrera-ingest
description: Ingest a referenced Markdown source into a Lumbrera LLM Wiki by preserving the raw resource and adding distilled wiki knowledge through lumbrera write.
---

# Lumbrera Ingest

Use when asked to turn source material into durable wiki pages.

## Contract

- Do not edit files directly; write only with lumbrera write.
- Preserve raw source material; do not alter existing sources/ files.
- Provide wiki body Markdown only. Lumbrera generates document IDs, frontmatter, Sources sections, indexes, changelog, checksums, and tags.

## Process

1. Read the source. Chunk large sources only for reading.
2. Read INDEX.md, tags.md, and relevant existing wiki pages.
3. Choose target wiki pages by durable topic/task, not by source chunk.
4. Draft small pages: one concept, task, runbook, decision, or reference per page.
5. Keep every wiki page under the hard maximum of 400 Markdown body lines.
6. Choose a clear title, single-line summary, and 1-5 --tag values, reusing tags.md when possible.
7. Add real wiki links and precise source citations where needed.
8. Write with lumbrera write, then run lumbrera verify.

## Good wiki page

- Atomic, source-grounded, searchable, and useful without reopening the source.
- Troubleshooting/runbooks prefer symptom → cause → fix structure.
- Split when a draft covers multiple concepts, has independent sections, needs more than 5 tags, or becomes a grab bag.
- Avoid pages like "everything from source X", "part 1", giant mixed-topic notes, filler tags, unsupported synthesis, or duplicates of existing pages.

## Links

- Before writing, identify 3-7 existing wiki pages that may overlap, depend on this page, or act as prerequisites.
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
- Still pass file-level provenance with lumbrera write --source; inline citations complement it, they do not replace it.
- Do not add citations to every sentence.

## Write command

For a new wiki page, pass --title, --summary, 1-5 --tag flags, --source, and --reason:

~~~sh
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<source>.md --reason "Distill source" < page.md
~~~

After writing, run lumbrera verify and report created/updated pages, covered source sections, skipped sections, uncertainties, and follow-up pages.

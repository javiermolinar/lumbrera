---
name: lumbrera-query
description: Answer questions from a Lumbrera LLM Wiki by searching the local Lumbrera index first, reading wiki synthesis before preserved sources, and citing the files used.
---

# Lumbrera Query

Use when the user asks a question about knowledge in the brain.

## Search-first workflow

1. Run one broad lexical search from the user question:

   ~~~sh
   lumbrera search "<question>" --json
   ~~~

2. Treat recommended_sections as the primary product contract and read those path#anchor targets first.
3. Check coverage on comparison/entity questions; if a named entity is missing, say so or refine the search before answering.
4. Use exact filters when the user names a known tag or provenance source:

   ~~~sh
   lumbrera search "<question>" --tag <tag> --json
   lumbrera search "<question>" --source sources/<source>.md --json
   ~~~

5. If recommended_sections are insufficient, read only the top 3 wiki pages from recommended_read_order.
6. Stop once those sections/pages support the answer.
7. If the top results are insufficient, run one refined search using better terms from the first results.
8. Only after search is insufficient, use INDEX.md and tags.md as fallback navigation and state why search was insufficient.

## Guardrails

- Do not start by scanning the repo, running broad find/rg, or reading every INDEX.md entry.
- If a user term is ambiguous, state the likely interpretations and either ask for clarification or answer with the assumed scope.
- Read cited sources only for numeric limits, operational/destructive actions, surprising claims, conflicts, uncertainty, or requested evidence.
- Use repeatable `--tag` and `--source` filters to narrow search when exact topic/provenance constraints matter; do not substitute filters for reading the returned evidence.
- For --kind source searches or source-only recommended_read_order, read those recommended source sections/files directly instead of scanning the repo.
- Do not infer frequency, priority, popularity, or prevalence unless the wiki/source explicitly supports it.
- When using internal/private operational sources, label the answer as internal-sourced and avoid presenting it as public documentation.
- Answer with citations to the wiki pages or source documents used.
- When asked, list the specific wiki/source files used.
- If the answer is durable knowledge worth keeping, ask whether to save it.
- Save only through lumbrera write. Do not create wiki frontmatter, tags.md entries, search indexes, or generated metadata.

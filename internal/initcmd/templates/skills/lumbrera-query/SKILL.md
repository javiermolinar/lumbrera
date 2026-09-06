---
name: lumbrera-query
description: Answer questions from a Lumbrera brain by searching first, prioritizing canonical wiki synthesis, and using first-party notes or preserved sources when needed.
---

# Lumbrera Query

Use when the user asks a question about knowledge in the brain.

## Search-first workflow

1. Run one broad intent-preserving search from the user question. Do not blindly pass exact user wording when it contains conversational phrasing, filler, or vague meta terms.

   - Keep named entities, technical nouns, tags, file/source names, and domain terms.
   - Rewrite soft questions into searchable concepts.
   - Add obvious synonyms only when they improve recall.
   - If the user asks for your experience using the brain/tool rather than repository knowledge, answer from the current interaction; search only for documented policy/history.

   ~~~sh
   lumbrera search "<intent-preserving query>" --json
   ~~~

2. Treat recommended_sections as the primary product contract and read those path#anchor targets first.
3. Check coverage on comparison/entity questions; if a named entity is missing, say so or refine the search before answering.
4. Use exact filters when the user names a known tag or evidence path:

   ~~~sh
   lumbrera search "<intent-preserving query>" --tag <tag> --json
   lumbrera search "<intent-preserving query>" --source sources/<source>.md --json
   lumbrera search "<intent-preserving query>" --source notes/<note>.md --json
   ~~~

5. If recommended_sections are insufficient, read only the top 3 paths from recommended_read_order. Prefer wiki synthesis; when no wiki matches, follow note recommendations before raw sources.
6. Stop once those sections/pages support the answer.
7. If the top results are insufficient, run one refined search using better terms from the first results.
8. Only after search is insufficient, use INDEX.md, NOTES.md, and tags.md as fallback navigation and state why search was insufficient.

## Tier awareness

Search results include a `tier` field: `canonical`, `design`, or `reference`. Canonical content ranks highest by default.

- Prefer canonical results for operational, how-to, and current-behavior questions.
- Use design-tier results only when the user asks about proposals, future plans, or design rationale.
- When citing design-tier content, state that it is a proposal and not implemented.
- Use `--tier design` to search only design proposals: `lumbrera search "<query>" --tier design --json`

## Guardrails

- Do not start by scanning the repo, running broad find/rg, or reading every INDEX.md entry.
- If a user term is ambiguous, state the likely interpretations and either ask for clarification or answer with the assumed scope.
- Read cited sources or notes only for numeric limits, operational/destructive actions, surprising claims, conflicts, uncertainty, or requested evidence.
- Use repeatable `--tag` and `--source` filters to narrow search when exact topic/evidence constraints matter; `--source` accepts source or note paths. Do not substitute filters for reading the returned evidence.
- For `--kind note` or `--kind source` searches, read the recommended sections/files directly instead of scanning the repo.
- Do not infer frequency, priority, popularity, or prevalence unless the wiki, note, or source explicitly supports it.
- When using internal/private operational sources, label the answer as internal-sourced and avoid presenting it as public documentation.
- Answer with citations to the wiki pages, notes, or source documents used.
- When asked, list the specific wiki, note, and source files used.
- When durable notes support a synthesized answer that lacks wiki coverage, suggest creating a wiki page backed by those note paths.
- If the answer contains new durable first-party knowledge worth keeping, ask whether to save it as a focused note.
- Save only through `lumbrera write`. Do not create frontmatter, Sources sections, tags.md entries, search indexes, or generated metadata.

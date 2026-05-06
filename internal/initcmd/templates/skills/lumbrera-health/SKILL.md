---
name: lumbrera-health
description: Review Lumbrera health candidates for duplicated concepts, missing cross-links, stale-risk, source coverage gaps, and other semantic maintenance work.
---

# Lumbrera Health

Use when the user asks for a semantic health check, consolidation review, link-health pass, stale-risk review, or source coverage review of a Lumbrera brain.

## Goal

Use deterministic `lumbrera health` candidates to decide what an LLM should review next. The goal is not to prove semantic drift automatically. The goal is to narrow review to likely maintenance opportunities, then read the relevant wiki pages and preserved sources before classifying any action.

`lumbrera health` returns review candidates, not conclusions. Treat `possible_duplicate`, `missing_link`, `orphan_page`, `underlinked_page`, and `uncited_source` as prompts for investigation.

Lumbrera handles deterministic consistency for managed wiki content: wiki document IDs, frontmatter, tag registry, index, changelog, checksums, source sections, broken links, heading anchors, path policy, and generated files. Do not spend LLM health-review effort on those.

## Workflow

1. Run `lumbrera health --json` before broad repository exploration.
2. Review the top candidate first. Do not scan the repo unless candidates are insufficient.
3. Use the candidate's `suggested_queries` with `lumbrera search "<query>" --json` when evidence is insufficient.
4. When a candidate reason names a tag or source, optionally inspect that local neighborhood with exact filters:

   ~~~sh
   lumbrera search "<query>" --tag <tag> --json
   lumbrera search "<query>" --source sources/<source>.md --json
   ~~~

5. Read the candidate pages and cited sources before deciding.
6. Classify the result as one of:
   - duplicate or consolidation opportunity;
   - overlapping but distinct pages;
   - missing cross-link;
   - stale-risk requiring source review;
   - missing concept or source coverage gap;
   - no action.
7. Report affected paths, deterministic reasons, evidence read, classification, and suggested next action.
8. If a mutation is needed, ask for explicit user approval first.
9. After approval, mutate only with `lumbrera write`, then run `lumbrera verify --brain .`.

## What to look for

- Pages that duplicate or fragment one concept and should be merged, clarified, or rewritten as canonical-plus-stub.
- Related pages that should link contextually or include a short Related pages section.
- Pages that cite similar sources but make stale or inconsistent claims.
- Uncited source files that contain concepts missing from the wiki.
- Orphan or weakly connected pages that should be linked, merged, or intentionally left standalone.
- Identify high-risk claims that need claim-level citations: limits, breaking changes, destructive procedures, security/auth behavior, and internal operational workflows.
- Internal-only knowledge that should be clearly marked and not presented as public documentation.

## Missing-link triage

Medium-confidence `missing_link` candidates are a review queue, not a todo list.

- Process candidates one by one.
- Add a link only when a reader of one page would benefit from discovering the other page at that point.
- Skip relationships that are only broad same-product, same-source, or same-tag overlap without a clear reader task.
- Prefer a contextual inline link near the relevant claim or procedure.
- Use a short Related pages section only when the relationship is page-level rather than paragraph-level.
- For a batch review, report candidates as reviewed, skipped, or actionable before asking for mutation approval.

## Duplicate/consolidation helper

Use this when a `possible_duplicate` candidate appears or reading a `missing_link` candidate suggests duplicated concepts.

Compare:

- page title and summary scope;
- reader task or question each page answers;
- claims that overlap or contradict;
- cited sources and claim-level evidence;
- unique facts or procedures that would be lost in a merge;
- existing inbound/outbound links.

Classify as:

- `duplicate`: same concept and same reader task; choose a canonical page.
- `overlap`: related but distinct scope; add cross-links or clarify page summaries instead of merging.
- `stale-risk`: one page appears older or contradicted by source evidence; reconcile against sources.
- `no action`: deterministic reasons were not semantically meaningful after reading.

If consolidation is approved, prefer updating the canonical page first, then rewriting the duplicate page as a short stub that links to the canonical page. Delete only when content is fully covered elsewhere and verification confirms no broken links.

## Guardrails

- Do not present deterministic candidates as proven semantic drift.
- Do not report lack of links as a problem unless there is a clear semantic relationship.
- Do not edit files directly or create generated metadata, including tags.md entries.
- Do not mutate sources; sources are preserved raw material.
- Prefer updating a canonical page plus adding a duplicate-page stub before deletion.
- Use `lumbrera write --delete` only when content is fully covered elsewhere and verification confirms no broken links.

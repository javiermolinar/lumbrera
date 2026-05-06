---
name: lumbrera-lint
description: Semantically health-check a Lumbrera LLM Wiki for stale synthesis, contradictions, unsupported claims, duplicated concepts, and useful follow-up questions.
---

# Lumbrera Lint

Use when the user asks for a semantic health check of the wiki.

Lumbrera handles deterministic consistency for managed wiki content: wiki document IDs, frontmatter, tag registry, index, changelog, checksums, source sections, broken links, heading anchors, path policy, and generated files. Do not spend LLM linting effort on those.

## Workflow

- Read the relevant wiki/ pages and their preserved sources/ documents.
- Look for semantic drift: stale claims, contradictions, synthesis that no longer matches sources, or claims not actually supported by cited sources.
- Identify high-risk claims that need claim-level citations: limits, breaking changes, destructive procedures, security/auth behavior, and internal operational workflows.
- Check whether internal-only knowledge is clearly marked and not presented as public documentation.
- Look for duplicated or fragmented concepts that should be merged or clarified.
- Identify important open questions or data gaps that need new sources.
- Report task-navigation gaps, such as missing troubleshooting quick references, FAQ-style pages, or symptom → cause → fix runbooks.
- Report findings with affected paths, evidence, and suggested next actions.
- If asked to fix semantic issues, use lumbrera write. Do not edit files directly or create wiki generated metadata, including tags.md entries.

## Semantic link health

- Read INDEX.md as the wiki map.
- Identify orphan or weakly connected wiki pages.
- Check whether related concepts are linked contextually.
- Look for pages that duplicate or depend on another page without linking to it.
- Suggest inline links or a short 'Related pages' section where useful.
- Do not report lack of links as a problem unless there is a clear semantic relationship.

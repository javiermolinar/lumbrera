---
name: lumbrera-delete
description: Delete a source, wiki page, or asset from a Lumbrera brain, cascading cleanup through all referencing wiki pages, and removing orphaned pages left with no sources.
---

# Lumbrera Delete

Use when asked to remove a source, delete a wiki page, remove an asset, or clean up a bad/superseded source and its downstream effects.

## Purpose

Safely remove a source or wiki page from the brain while keeping every remaining page in a valid, verify-passing state. Cascade cleanup handles all downstream references so the brain never contains broken links, stale citations, or orphaned pages.

## When to use

- A source is incorrect, outdated, superseded, or "poisoned" and all wiki knowledge derived from it must be cleaned up.
- A wiki page is no longer needed and pages linking to it should have their links removed.
- An asset (diagram, image, PDF) is outdated and should be removed along with its wiki references.
- Health review identified a page that should be deleted after its content was consolidated elsewhere.

## Contract

- Do not delete files directly; use only `lumbrera delete`.
- Always provide a `--reason` explaining why the file is being removed.
- Run `lumbrera verify` after deletion to confirm brain integrity (the delete command does this automatically).
- Confirm with the user before deleting, especially when cascade will remove wiki pages.

## Cascade behavior

### Source deletion

Deleting a source triggers cleanup in this order:

1. **Strip inline citations**: `[source: ../sources/removed.md#anchor]` markers are removed from wiki body text.
2. **Update frontmatter**: the source is removed from `lumbrera.sources` in each referencing wiki page.
3. **Regenerate Sources sections**: the `## Sources` section is rebuilt from remaining sources.
4. **Cascade-delete orphaned wikis**: wiki pages left with zero sources are deleted (no provenance = no value).
5. **Clean broken wiki links**: pages that linked to cascade-deleted wiki pages have those links unwrapped (`[text](link)` → `text`).

### Wiki deletion

Deleting a wiki page:

1. **Unwrap links**: other wiki pages linking to the deleted page have `[text](link)` replaced with `text`.
2. **Update frontmatter**: `lumbrera.links` is updated in referencing pages.

Wiki-to-wiki link cleanup does **not** trigger further wiki deletions.

### Asset deletion

Deleting an asset:

1. **Scrub image embeds**: `![alt](assets/path)` is removed entirely from wiki page bodies.
2. **Scrub links**: `[text](assets/path)` is removed entirely from wiki page bodies.
3. **No cascade deletion**: wiki pages are never deleted when an asset is removed.

After asset deletion, the LLM should review affected wiki pages and rewrite surrounding prose if it reads awkwardly without the image/link.

## Workflow

1. Identify the file to delete and why.
2. Check what will be affected:

   ~~~sh
   lumbrera search "<source or wiki name>" --json
   ~~~

   Review search results to understand which wiki pages reference the target.

3. Inform the user of the cascade impact before proceeding:
   - Which wiki pages will be updated (source/link removed).
   - Which wiki pages will be cascade-deleted (zero sources remaining).
   - Which pages will have links unwrapped.

4. After user approval, run the delete:

   ~~~sh
   lumbrera delete sources/<path>.md --reason "Remove bad source: <explanation>"
   lumbrera delete wiki/<path>.md --reason "Remove page: <explanation>"
   ~~~

5. Review the output for the list of deleted and updated files.
6. Report: files deleted, files updated, cascade deletions, and verify status.

## Command reference

~~~sh
# Delete a source and cascade-clean all referencing wiki pages.
lumbrera delete sources/<path>.md --reason "<reason>"

# Delete a wiki page and clean links in referencing pages.
lumbrera delete wiki/<path>.md --reason "<reason>"

# Delete an asset and scrub references from wiki pages.
lumbrera delete assets/<path> --reason "<reason>"

# Specify actor and brain directory.
lumbrera delete <path> --reason "<reason>" --actor "<actor>" --brain <dir>
~~~

## Guardrails

- Always confirm with the user before executing, stating what will be deleted and updated.
- Prefer consolidation (update canonical page, then delete duplicate) over outright deletion.
- The delete command is atomic: if verification fails, all changes are rolled back.
- Do not use `lumbrera write --delete`; it is deprecated and delegates to `lumbrera delete`.
- After deletion, consider running `lumbrera health --json` to check for new maintenance candidates caused by the removal.

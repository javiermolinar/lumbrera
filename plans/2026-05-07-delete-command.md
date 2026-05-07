# Lumbrera Delete Command

## Context

Sources are currently immutable — once added, they cannot be removed. A bad source ("poison fountain") propagates through every wiki page that cites it. There is no way to cleanly rip out a source and its downstream effects. The existing `lumbrera write --delete` only works for wiki pages and does no reference cleanup, so it fails verify if other pages link to the deleted file.

## Goals

- Allow deleting source files and cascading cleanup through all referencing wiki pages
- Delete wiki pages left with zero sources (no provenance = no value)
- Clean up broken wiki-to-wiki links caused by cascading wiki deletions
- Keep the brain in a valid, verify-passing state after every delete

## Constraints

- Must be a single atomic transaction with rollback on failure
- Must log a changelog/ops entry for every file deleted
- Must regenerate all generated files (INDEX.md, CHANGELOG.md, BRAIN.sum, tags.md)
- Must pass `lumbrera verify` at the end of the transaction
- Wiki-to-wiki link removal does NOT trigger further wiki deletions (only zero-source pages get deleted)

## Proposed plan

### 1. New `lumbrera delete` command

Separate from `lumbrera write --delete`. Delete has fundamentally different semantics: no stdin, no `--title`/`--summary`/`--tag`, and cascading cleanup logic.

```
lumbrera delete <path> --reason <reason> [--actor <actor>] [--brain <path>]
```

- `<path>` — target under `sources/` or `wiki/`
- Accepts both source and wiki targets
- `--reason` required for changelog

### 2. Create `internal/deletecmd/` package

Mirror the structure of `writecmd/`: args parsing, validation, execution, tests.

Files:
- `delete.go` — `Run()` entry point, transaction orchestration
- `args.go` — parse `--reason`, `--actor`, `--brain`, target path
- `cascade.go` — reference graph walking and cleanup logic
- `cleanup.go` — wiki file mutation (strip citations, regenerate sources section, update frontmatter)
- `delete_test.go` — unit + integration tests

### 3. Cascade algorithm

```
deleteSource(path):
  1. collect all wiki pages where frontmatter.sources includes path
  2. for each wiki page:
     a. remove source from frontmatter.sources
     b. strip inline [source: <path>#...] citations from body
     c. regenerate ## Sources section from remaining sources
     d. if sources list is now empty → mark for deletion
     e. else → rewrite the wiki file with updated frontmatter + body
  3. delete the source file
  4. for each wiki page marked for deletion:
     a. deleteWiki(wikiPath)

deleteWiki(path):
  1. collect all wiki pages where frontmatter.links includes path
  2. for each referencing wiki page:
     a. remove body links pointing to path
     b. update frontmatter.links
     c. rewrite the wiki file
  3. delete the wiki file
```

Key: `deleteWiki` does NOT recurse into further wiki deletions. Only zero-source pages trigger deletion. Link cleanup just removes the broken link from the referencing page's body and frontmatter.

### 4. Wiki file cleanup operations

These are the mutations needed on a wiki file during cascade:

| Operation | What changes |
|---|---|
| Remove source reference | `lumbrera.sources` frontmatter list |
| Strip inline citations | Body text: remove `[source: ../sources/foo.md#anchor]` matches |
| Regenerate Sources section | `## Sources` section rebuilt from remaining sources |
| Remove wiki link | Body text: unwrap `[label](../wiki/deleted.md)` → `label` |
| Update link frontmatter | `lumbrera.links` frontmatter list |

Reuse existing infrastructure:
- `markdown.RemoveSourcesSection` / `AppendSourcesSection` — already exist
- `markdown.Analyze` — finds citations, links, sources
- `frontmatter.Split` / `Attach` — read/write frontmatter
- `markdown.sourceCitationPattern` — regex for finding inline citations (needs to be exported or duplicated)
- New: function to strip citations matching a specific source path
- New: function to unwrap links to a specific wiki path (replace `[text](link)` with `text`)

### 5. Transaction and backup

Extend the backup approach from `writecmd`:
- Back up ALL files that will be mutated (target + all affected wiki pages + generated files + ops.log)
- On any error, restore all backups
- On success, append one ops entry per deleted file, regenerate generated files, verify

### 6. Wire into CLI

Add `"delete"` case in `cmd/lumbrera/main.go` dispatch.

### 7. Deprecate `write --delete`

In `writecmd`:
- When `--delete` is parsed, print to stderr: `warning: --delete is deprecated, use "lumbrera delete <path> --reason <reason>" instead`
- Delegate to `deletecmd.Run` with the same `--brain`, `--actor`, `--reason`, and target path
- Remove `opDelete` handling from `writecmd` mutation/validation logic (dead code after delegation)

### 8. Tests

Unit tests in `delete_test.go`:
- Delete source with one referencing wiki → wiki cleaned, source gone
- Delete source with multiple referencing wikis → all cleaned
- Delete source that is the only source for a wiki → wiki cascade-deleted
- Cascade wiki deletion cleans links in other wiki pages
- Delete source with inline citations → citations stripped from body
- Delete wiki page directly → links cleaned in referencing pages
- Delete non-existent file → error
- Transaction rollback on verify failure
- Concurrent brain lock rejection

E2E test:
- Full init → sources → wiki pages → delete source → verify passes

## Open questions

- Should delete print a summary of affected files before executing, or just report after? (Dry-run `--dry-run` flag for later?)
- ~~Should the `lumbrera write --delete` flag be deprecated?~~ **Decided: yes.** `write --delete` will print a deprecation warning pointing to `lumbrera delete` and delegate to `deletecmd.Run`.
- When stripping inline citations, should we leave a marker comment (e.g. `<!-- citation removed -->`) or strip silently? Leaning toward silent — the changelog records what happened.
- When unwrapping wiki links (`[text](link)` → `text`), is plain text the right replacement, or should it become ~~strikethrough~~ or similar?

## Next steps

1. Implement `internal/deletecmd/args.go` — argument parsing
2. Implement `internal/deletecmd/cascade.go` — reference graph collection
3. Implement `internal/deletecmd/cleanup.go` — wiki file mutation helpers
4. Implement `internal/deletecmd/delete.go` — orchestration with transaction
5. Wire into `main.go`
6. Write tests

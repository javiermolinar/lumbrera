# Lumbrera Plan

## Context
Lumbrera is a proposed backendless, Git-backed, Markdown-native second brain for humans and LLM agents. It builds on Andrej Karpathy's "LLM Wiki" pattern: preserve raw sources, distill them into a human-readable Markdown wiki, and let agents maintain the knowledge base over time.

Reference: https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f

The distinctive part of Lumbrera is not just Markdown storage. Karpathy's idea describes the pattern; Lumbrera is an opinionated implementation with a strict write boundary: agents may read files directly with normal tools, but all mutations must go through a tiny CLI that syncs, writes, regenerates metadata, commits, and pushes atomically.

## Relationship to Karpathy's LLM Wiki idea

Karpathy's gist defines the high-level pattern:

```text
raw sources -> LLM-maintained Markdown wiki -> index/log for navigation and history
```

Lumbrera keeps that core idea and adds an enforcement layer:

```text
read files directly
write only through Lumbrera
sync/generated metadata/checksums/hooks handled by the tool
```

So Lumbrera can be framed as:

> A backendless, agent-safe implementation of the LLM Wiki pattern.

## Goals
- Build a backendless second brain where the Git repo is the shared state.
- Keep raw Markdown files human-readable and organized in a logical hierarchy.
- Preserve source material before distilling it into durable wiki pages.
- Allow bring-your-own-agent usage: Pi, Claude Code, Cursor, Slack bots, custom agents, or humans.
- Keep the public API minimal: `init`, `sync`, and `write`.
- Avoid a database, vector search, CRDTs, or a custom backend in v1.
- Generate `INDEX.md`, `CHANGELOG.md`, and `BRAIN.sum` automatically.
- Use Git hooks as defense-in-depth against direct edits or drift.

## Non-goals for v1
- No SQLite index.
- No vector search.
- No CRDTs.
- No server/backend.
- No custom agent from scratch.
- No desktop app initially.
- No PDF/web/image ingestion initially; ingest Markdown only.
- No large agent API with many note-specific commands.

## V2 ideas
- Add optional configuration for custom branches, directories, lint policy, and source immutability.
- Build a derived database from the Markdown hierarchy, optional frontmatter, `## Sources` sections, and Markdown links.
- Treat Markdown links as the canonical knowledge graph and derive backlinks, orphan pages, broken links, and graph traversal from them.
- Add full-text search over pages and sources.
- Add optional vector search/embeddings for semantic discovery.
- Add richer linting: stale claims, missing sources, weakly connected pages, duplicate concepts, and broken provenance.
- Add PDF/web/image/voice ingestion adapters that convert inputs to Markdown before calling `lumbrera write`.
- Add LLM-assisted conflict reconciliation.
- Add a desktop app for browsing, capture, sync status, and conflict review.

## Core architecture

```text
local Markdown repo
  ↑
lumbrera CLI
  ↑
Pi / Claude Code / Cursor / Slack bot / human shell
```

The Git remote acts as the backendless synchronization point. Lumbrera owns writes and Git operations. Agents can inspect the repo directly but must not mutate files directly.

## Repository layout

V1 is configless and convention-based. Lumbrera should prefer a small fixed set of defaults over user configuration until the core protocol is proven.

V1 conventions:
- canonical branch is `main` for new repos; existing repos use the current branch during init,
- allowed content directories are `sources/` and `wiki/` only,
- generated files are `INDEX.md`, `CHANGELOG.md`, and `BRAIN.sum`,
- generated files are never manually editable,
- source files are immutable after creation,
- distilled pages require a `## Sources` section,
- Markdown links to local files must resolve,
- one successful write creates one commit and one push.

Initial scaffold:

```text
INDEX.md              # generated navigation map
CHANGELOG.md          # generated human-readable history
BRAIN.sum             # generated checksum/integrity manifest
AGENTS.md             # BYOA instructions / agent contract

sources/              # preserved raw input, mostly immutable
wiki/                 # distilled maintained knowledge; users choose the hierarchy inside it

.brain/
  conflicts/          # temporary conflict context for failed sync/write attempts
  hooks/              # git hooks installed via core.hooksPath
```

The exact hierarchy can evolve, but the source/distillation split is important:

```text
sources/ -> preserved inputs
wiki/    -> distilled, maintained knowledge
```

## Public CLI API

### `lumbrera init <repo>`
Creates the minimal scaffold and installs repository hooks.

Responsibilities:
- Create directory structure.
- Generate initial `INDEX.md`, `CHANGELOG.md`, and `BRAIN.sum`.
- Generate `AGENTS.md`.
- Install hooks via `git config core.hooksPath .brain/hooks`.
- Initialize Git if needed.

### `lumbrera sync --repo <repo>`
Makes the local repo current, clean, generated, linted, verified, and pushed.

Responsibilities:
1. Acquire a local repo lock.
2. Ensure the canonical branch is checked out.
3. Fetch remote changes.
4. Pull/rebase latest changes.
5. Regenerate generated files.
6. Run deterministic lint.
7. Verify integrity internally.
8. Commit generated repairs if needed.
9. Push local commits.
10. Leave working tree clean.

`verify`, `status`, and `lint` are internal functions for v1, not public commands.

### Sync convergence invariant

`sync` is a convergence operation. It attempts to move the repo into a valid Lumbrera state. If it cannot, it exits non-zero and reports the blocking reason. It must not commit or push partial repairs that hide invalid local state.

A valid Lumbrera state means:
- generated files exactly match the current Markdown state,
- `BRAIN.sum` matches tracked content,
- deterministic lint passes,
- every Markdown mutation is represented by a Lumbrera changelog commit subject,
- the Git working tree is clean,
- the local branch can be pushed to the canonical remote branch.

Failure classes:
- Regeneratable drift: stale `INDEX.md`, `CHANGELOG.md`, or `BRAIN.sum`. `sync` may regenerate these files, commit the repair, and push.
- Invalid knowledge state: missing sources, broken provenance, malformed optional frontmatter, source immutability violation, or paths outside policy. `sync` fails until the user or agent fixes the state through `lumbrera write`.
- Clean remote divergence: remote changed but rebase/pull applies cleanly. `sync` continues, regenerates, verifies, and pushes if needed.
- Git or semantic conflict: same file or semantic area changed incompatibly. `sync` fails with conflict context for human or LLM-assisted resolution.

### `lumbrera write <path> [options] < content.md`
Performs one atomic repository mutation transaction. Most writes receive content through stdin; delete operations do not.

Example: preserve a source:

```bash
lumbrera write sources/2026/05/01/backendless-discussion.md \
  --reason "Preserve original design discussion" \
  < discussion.md
```

Example: write distilled wiki page:

```bash
lumbrera write wiki/architecture/backendless.md \
  --source sources/2026/05/01/backendless-discussion.md \
  --reason "Distill backendless design" \
  < backendless.md
```

Example: append to an existing section:

```bash
lumbrera write wiki/architecture/backendless.md \
  --append "Sync model" \
  --source sources/2026/05/01/backendless-discussion.md \
  --reason "Add sync behavior" \
  < sync-model.md
```

Potential options:

```text
--repo <path>          target brain repo
--source <path>        provenance source; required for distilled knowledge writes
--reason <reason>      human-readable operation reason
--actor <actor>        actor label for changelog, defaulting to the Git user or calling agent when available
--append <section>     append stdin content to a named section
--delete               delete the target file
```

There is no separate `--change-type` in v1. The changelog operation is inferred from the command shape and file state:
- no mutation flag and path does not exist: `create`,
- no mutation flag and path exists: `update` by replacing the file with stdin content,
- no mutation flag, path does not exist, and path is under `sources/`: `source`,
- `--append <section>` and path exists: `append`,
- `--delete` and path exists: `delete`.

Mutation flags are mutually exclusive. `--append` and `--delete` cannot be combined. Create, update, source, and append operations require stdin content. Delete operations do not read stdin.

Because sources are immutable after creation, `lumbrera write` must reject updates, appends, and deletes targeting existing files under `sources/`. Corrections must be added as new source files.

V1 has no move primitive. Moves or renames are represented as separate create/update and delete transactions.

Path safety rules:
- Targets must be repo-relative paths.
- Absolute paths are rejected.
- Paths containing `..` are rejected.
- Paths with a leading `./` are normalized or rejected consistently.
- Targets must be under an allowed content directory: `sources/` or `wiki/`.
- Direct writes to `.git/`, `.brain/`, generated files, or other internal paths are rejected.
- Symlink traversal must not allow reads or writes outside the repo.
- The resolved target path must remain inside the repo root after normalization.

Every successful `write` must follow the transaction invariant:

```text
one successful lumbrera write = one Lumbrera transaction = one Git commit = one push
```

Responsibilities:
1. Acquire the local repo lock.
2. Fetch/rebase remote changes before mutation.
3. Verify the repo is already in a valid Lumbrera state after rebase.
4. If the repo requires generated repairs or has invalid local state, fail and instruct the caller to run `lumbrera sync` first.
5. Apply the mutation.
6. Regenerate `INDEX.md`.
7. Regenerate `CHANGELOG.md`.
8. Regenerate `BRAIN.sum`.
9. Verify internally.
10. Create exactly one transaction commit containing both content changes and generated-file updates. The commit subject must follow the Lumbrera changelog convention: `[operation] [actor]: reason`, where `operation` is inferred from the mutation.
11. Push exactly that transaction commit.
12. If push fails because the remote changed, retry by rebasing, regenerating, verifying, and amending/recreating the same transaction commit if possible.
13. If retry cannot preserve the one-transaction/one-commit/one-push invariant, fail with conflict context.
14. Leave the working tree clean on success.

## Provenance rules

Every distilled page must reference its sources. This is a core anti-hallucination and anti-drift rule.

- Writes to `sources/` do not require `--source`; the file itself is the source.
- Knowledge-bearing writes under `wiki/` require `--source`.
- Each distilled Markdown page must contain a `## Sources` section or equivalent source metadata listing the source files that support the page.
- `lumbrera write` should automatically insert or update the page's source references from the provided `--source` flags.
- A distilled page with no source references is invalid, unless it is an empty scaffold.
- Deletes do not require `--source`, but they do require `--reason` and are recorded as `delete` changelog entries.

Distilled pages may also include normal Markdown links to related pages. These links form the canonical knowledge graph in v1. No custom graph format is needed.

### Frontmatter policy

Frontmatter is optional in v1. Required protocol metadata should not depend on an LLM hand-writing YAML correctly.

Rules:
- LLMs and agents provide Markdown body content and CLI flags such as `--source` and `--reason`.
- Lumbrera owns required protocol metadata, source references, generated files, checksums, and changelog commits.
- If frontmatter is present, Lumbrera validates and preserves it.
- Required provenance should be represented through `--source` and the `## Sources` section, not only through frontmatter.
- Future versions may let Lumbrera insert or normalize frontmatter, but v1 should not require agents to author it manually.

Suggested page structure:

```md
# Backendless architecture

...

## Related

- [Atomic write protocol](./atomic-write-protocol.md)
- [BYOA agent contract](../agents/byoa-contract.md)

## Sources

- [Backendless discussion](../../sources/2026/05/01/backendless-discussion.md)
```

This prevents agents from silently inventing knowledge, makes drift detectable, gives humans a way to audit every claim back to preserved source material, and creates a simple Markdown-native graph that can later be indexed into a database.

## Enforcement model

Lumbrera v1 should hard-enforce mechanical integrity, not semantic truth.

Hard-enforced by CLI, deterministic lint, hooks, and checksums:
- all mutations go through `lumbrera write`,
- targets stay under `sources/` or `wiki/`,
- sources are immutable after creation,
- `wiki/` writes require `--source`,
- distilled pages require `## Sources`,
- local Markdown links resolve,
- generated files are current,
- `BRAIN.sum` matches tracked Markdown files,
- commit subject follows `[operation] [actor]: reason`,
- one successful write creates one commit and one push,
- working tree is clean after success.

Soft-enforced by templates, review, and agent instructions:
- quality of summaries,
- whether a page is well structured,
- whether decisions include context and consequences,
- whether related links are useful,
- whether a source really supports every claim.

V1 should not try to automatically prove semantic correctness. It should make unsupported or unaudited knowledge changes difficult and visible.

## Deterministic lint

Karpathy's LLM Wiki idea includes a lint/health-check stage. In Lumbrera v1, this should be implemented deterministically inside `sync`, not as a separate public primitive.

`write` runs the same deterministic verification before committing. It may fetch/rebase remote changes, but if the repo needs generated repairs or invalid-state cleanup, `write` fails and instructs the caller to run `sync` first.

Blocking lint errors:
- Missing `## Sources` section on distilled pages.
- Missing or invalid source links.
- Broken local Markdown links.
- Malformed optional frontmatter.
- `BRAIN.sum` mismatch after regeneration.
- Modified source files after creation.
- Markdown changes not committed with a Lumbrera changelog subject.
- Paths outside allowed directories or failing path safety rules.

Non-blocking lint warnings:
- Orphan wiki pages with no inbound links.
- Duplicate titles or slugs.
- Pages with no `## Related` section.
- Source files not referenced by any distilled page.
- Very large pages that may need splitting.

Future semantic lint can use LLMs and embeddings to suggest duplicate concepts, stale claims, contradictions, missing links, and pages that should be merged. Those checks should produce suggestions, not mandatory rewrites.

## Generated files

### `INDEX.md`
Generated from the Markdown hierarchy, headings, links, and optional frontmatter. It should be readable and useful as a navigation map, not necessarily a full database dump.

### `CHANGELOG.md`
Generated from Lumbrera commit subjects and commit dates, not raw diffs or separate operation records.

`lumbrera write` requires only two changelog inputs:
- `--actor`, defaulting to the Git user or calling agent when available,
- `--reason`, a human-readable explanation.

The operation label is inferred by Lumbrera from the command shape and file state: `source`, `create`, `append`, `update`, or `delete`.

Each successful `write` creates one commit with this subject convention:

```text
[operation] [actor]: reason
```

`CHANGELOG.md` renders each Lumbrera commit as:

```text
date [operation] [actor]: reason
```

Example generated entries:

```text
2026-05-02 [source] [human]: Preserve original design discussion
2026-05-02 [create] [pi-agent]: Add backendless architecture page
2026-05-02 [append] [pi-agent]: Add sync behavior
2026-05-02 [update] [pi-agent]: Fix links after page split
2026-05-02 [delete] [pi-agent]: Remove obsolete capture page
```

`sync` may create repair commits for generated-file drift using `[sync] [lumbrera]: Regenerate generated files`.

V1 should not require separate `.brain/ops/` operation files, commit trailers, or user-supplied change types. If commit subjects become too limited later, richer metadata can be added as a v2 extension.

### `BRAIN.sum`
Generated manifest/checksum file, similar in spirit to `go.sum`. It records normalized repo-relative paths and content checksums for tracked Markdown files.

Suggested v1 format:

```text
lumbrera-sum-v1 sha256
sources/2026/05/01/backendless-discussion.md sha256:<content-hash>
wiki/architecture/backendless.md sha256:<content-hash>
```

Rules:
- Each entry binds a normalized repo-relative path to a content hash.
- The content hash is computed from normalized file content only.
- Path identity is enforced by the manifest entry itself: if a file moves, the old path is missing and the new path is untracked until Lumbrera records the change.
- Paths use POSIX `/`, have no leading `./`, and must not contain `..`.
- Entries are sorted lexicographically by path.
- Exclude `BRAIN.sum` itself, `.git/`, `.brain/`, and generated files.

Purpose:
- Detect direct edits.
- Detect missing or changed files.
- Detect stale generated files.
- Support hook enforcement.

## Git hooks

Hooks are defense-in-depth, not a complete security boundary.

Install:

```bash
git config core.hooksPath .brain/hooks
```

Suggested hooks:

### `pre-commit`
Reject commits when:
- generated files are stale,
- `BRAIN.sum` does not match Markdown files,
- Markdown changes are not committed with a Lumbrera changelog subject,
- optional frontmatter is malformed,
- generated files appear manually edited,
- paths violate Lumbrera policy.

### `commit-msg`
Require the Lumbrera changelog subject convention:

```text
[operation] [actor]: reason
```

Example:

```text
[create] [pi-agent]: Add backendless architecture page
```

### `pre-push`
Run internal verification and reject unresolved conflicts or stale generated state.

Hooks can be bypassed with Git flags, so the main safety model remains: agents should only receive instructions to write through Lumbrera.

## Agent contract

`AGENTS.md` should say:

```text
You may:
- run `lumbrera sync --repo <repo>` before reading,
- read Markdown files directly,
- use read-only tools such as ls, find, tree, grep, rg, and read,
- inspect INDEX.md, CHANGELOG.md, and BRAIN.sum.

You must not:
- create, edit, move, delete, or overwrite files directly,
- edit generated files,
- run Git commands directly,
- treat generated files or future indexes as source of truth.

All mutations must use `lumbrera write`.
```

## BYOA integration

Lumbrera should not be Pi-specific. The CLI protocol is the product boundary.

First integrations:
- Generic `AGENTS.md` generated by `init`.
- Pi skill explaining the Lumbrera contract.

Future integrations:
- Claude Code instructions.
- Cursor rules.
- MCP adapter if useful.
- Slack bot wrapper that shells out to `lumbrera sync` and `lumbrera write`.

## Agent choice

For v1, reuse Pi rather than building a new agent.

Reasons:
- Pi already provides TUI, sessions, providers, skills, extensions, SDK/RPC.
- The hard problem is the write protocol, not the chat UI.
- Pi can be the first/default agent client without becoming a dependency of the core format.

Keep Lumbrera core independent from Pi.

## Feeding model

Start with Markdown-only input.

Flow:

```text
Markdown input -> sources/ preserved file -> distilled wiki updates
```

In v1, PDF/web/image ingestion should be handled outside Lumbrera by converting to Markdown first. Later adapters can convert documents into Markdown before calling `lumbrera write`.

## Parallel agents and backendless sync

There is no central coordinator. Multiple agents may operate from separate clones against the same canonical branch.

Lumbrera should minimize conflicts by:
- syncing before writes,
- making every write atomic,
- preferring unique source files for raw input,
- regenerating generated files deterministically,
- treating generated-file conflicts as disposable/regenerable,
- stopping on real Markdown conflicts.

If two agents edit the same Markdown page, Git conflicts can still occur. V1 should detect and stop rather than silently resolve semantic conflicts.

### Conflict handling contract

Conflict resolution is semantic, not just textual, so the CLI should not silently call an LLM and auto-merge in v1.

On a write/sync conflict, Lumbrera should:
- abort the automatic transaction before committing/pushing an ambiguous merge,
- preserve enough context for recovery under `.brain/conflicts/<txn>/`,
- report conflicted paths and base/local/remote versions,
- leave the repo either clean or in an explicit recoverable conflict state,
- exit non-zero with machine-readable output, preferably JSON.

Example conflict response:

```json
{
  "error": "conflict",
  "transaction": "txn_20260502_abc123",
  "conflicts": [
    {
      "path": "wiki/architecture/backendless.md",
      "base": ".brain/conflicts/txn_20260502_abc123/base/wiki/architecture/backendless.md",
      "local": ".brain/conflicts/txn_20260502_abc123/local/wiki/architecture/backendless.md",
      "remote": ".brain/conflicts/txn_20260502_abc123/remote/wiki/architecture/backendless.md"
    }
  ],
  "next": "Resolve semantically, then retry lumbrera write with the resolved content."
}
```

Agents may use an LLM to inspect the conflict context and propose a semantic resolution, but the final mutation must still go through `lumbrera write` so provenance, generated files, checksums, commits, and pushes remain controlled by Lumbrera.

Future option: provide a helper command or integration for LLM-assisted reconciliation, but keep it outside the hidden automatic write path.

## Implementation language

Use TypeScript for v1.

Reasons:
- Pi is TypeScript/npm-based.
- npm distribution fits Pi skills and CLI installation.
- Good ecosystem for Markdown, optional frontmatter validation, CLI parsing, JSON schema, and Git wrappers.

Possible package layout:

```text
lumbrera/
  src/
    cli.ts
    core/
      init.ts
      sync.ts
      write.ts
      git.ts
      manifest.ts
      generators.ts
      hooks.ts
  skills/
    pi/
      SKILL.md
```

Keep the repo format and write protocol language-independent.

## Prioritized implementation plan

1. Define the v1 repo layout and convention-based defaults.
2. Define the `write` command interface and path/provenance rules.
3. Implement `lumbrera init <repo>`.
4. Implement manifest generation for `BRAIN.sum`.
5. Implement hierarchical `INDEX.md` generation.
6. Implement `CHANGELOG.md` generation from Lumbrera commit subjects.
7. Implement internal verification.
8. Implement `lumbrera write` with preflight verification, local commit, and push.
9. Add Git commit creation per write.
10. Add hooks.
11. Implement deterministic lint inside `sync`.
12. Implement `lumbrera sync`.
13. Make `write` fetch/rebase before mutation and fail when explicit `sync` repair is required.
14. Add push-rejected retry handling.
15. Add basic generated-file conflict handling.
16. Generate `AGENTS.md` during init.
17. Ship a Pi skill.
18. Dogfood with Pi on a real Lumbrera repo.
19. Add tests for init, write, append, generated files, checksum drift, deterministic lint, hooks, sync, and conflict cases.

## MVP cut

Smallest useful MVP:

```text
lumbrera init
lumbrera write
INDEX.md generation
CHANGELOG.md generation
BRAIN.sum generation
one Git commit per successful write
one push per successful write
AGENTS.md generation
```

Then add:

```text
lumbrera sync
hooks
Pi skill
```

## Open questions

- Should source files be strictly immutable after creation, or allow metadata-only edits?
- Should source references remain only in `## Sources`, or should future versions also support YAML frontmatter or inline citations?
- Should `write` ever support batch operations, or should multi-file changes remain multiple one-commit transactions?
- What exact format should `BRAIN.sum` use: line-oriented text, JSON, or TOML?
- How much conflict reconciliation belongs in v1?

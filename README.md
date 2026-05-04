# Lumbrera

Lumbrera is a backendless, Markdown-native second brain for humans and LLM agents.

It is inspired by the LLM Wiki pattern: preserve raw source material, distill it into a durable human-readable Markdown wiki, and let agents help maintain that knowledge base over time.

## What problem does it solve?

LLM agents are useful for summarizing, organizing, and updating knowledge, but they need a safe way to write durable shared memory. Plain folders of Markdown are easy to read and edit, but direct edits by humans and agents can drift, lose provenance, or silently overwrite important context. Hosted knowledge tools solve some of this with backends and product-specific workflows, but they can make the data less portable and harder to audit.

Lumbrera keeps the data as ordinary files and makes the CLI the mutation boundary. Agents may read Markdown directly, but durable changes go through `lumbrera write`, which applies path/provenance rules, regenerates metadata, and updates an internal operation log.

## Goals

- Keep knowledge in ordinary Markdown files that humans and agents can read directly.
- Preserve source material before distilling it into wiki pages.
- Generate `INDEX.md`, `CHANGELOG.md`, and `BRAIN.sum` from the current brain state.
- Require source references for distilled knowledge so claims can be traced back to preserved material.
- Validate local Markdown links, heading anchors, and optional inline source citations such as `[source: ../sources/input.md#section]` during writes.
- Support bring-your-own-agent workflows, including Pi, Claude Code, Cursor, Slack bots, scripts, and humans.
- Avoid a database, custom backend, CRDT layer, hosted service, or mandatory Git workflow in the first version.

Lumbrera is not trying to be a new chat UI or a full knowledge-management app. It is a small protocol and CLI boundary for maintaining source-grounded Markdown knowledge safely in local files. Git, cloud sync, backups, and sharing are external choices.

## Install

```sh
go install github.com/javiermolinar/lumbrera/cmd/lumbrera@main
```

The module root is not an installable command package; use `/cmd/lumbrera`.

## Commands

Humans usually start with:

```sh
lumbrera init ./brain
```

Agents then use the generated `AGENTS.md` and bundled skills. The core protocol is intentionally small:

- `lumbrera write ...` is the only supported mutation boundary for `sources/` and `wiki/`.
- `lumbrera verify --brain <path>` checks deterministic integrity: provenance, links, generated files, and checksums.
- `lumbrera init <path>` creates a brain scaffold. It does not initialize Git, install hooks, commit, or push.

## External guardrails

Lumbrera does not own Git or GitHub. If a brain is stored in Git, humans can optionally add hooks, GitHub Actions, CI checks, or branch protection that run:

```sh
lumbrera verify --brain .
```

These guardrails are defense-in-depth. They catch drift and direct edits, but they do not replace `lumbrera write`. If generated files drift, restore them from your external versioning or backup system, then retry the Lumbrera operation.

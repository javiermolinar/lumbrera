# Lumbrera

Lumbrera is a backendless, Git-backed, Markdown-native second brain for humans and LLM agents.

It is inspired by the LLM Wiki pattern: preserve raw source material, distill it into a durable human-readable Markdown wiki, and let agents help maintain that knowledge base over time.

## What problem does it solve?

LLM agents are useful for summarizing, organizing, and updating knowledge, but they need a safe way to write durable shared memory. Plain folders of Markdown are easy to read and edit, but direct edits by multiple humans and agents can drift, lose provenance, or silently overwrite important context. Hosted knowledge tools solve some of this with backends and product-specific workflows, but they can make the data less portable and harder to audit.

Lumbrera treats a Git repository as the shared source of truth. Raw sources are preserved, distilled knowledge is written as Markdown, and every mutation is intended to go through a small CLI that verifies provenance, regenerates metadata, commits, and pushes the change.

## Goals

- Keep knowledge in ordinary Markdown files that humans and agents can read directly.
- Preserve source material before distilling it into wiki pages.
- Make knowledge changes auditable through Git history and generated changelogs.
- Require source references for distilled knowledge so claims can be traced back to preserved material.
- Validate local Markdown links, heading anchors, and optional inline source citations such as `[source: ../sources/input.md#section]` during writes.
- Support bring-your-own-agent workflows, including Pi, Claude Code, Cursor, Slack bots, scripts, and humans.
- Avoid a database, custom backend, CRDT layer, or hosted service in the first version.

Lumbrera is not trying to be a new chat UI or a full knowledge-management app. It is a small protocol and CLI boundary for maintaining source-grounded Markdown knowledge safely in Git.

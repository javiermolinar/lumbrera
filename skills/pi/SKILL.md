---
name: lumbrera
description: Use the Lumbrera CLI contract for backendless, Git-backed Markdown knowledge bases.
---

# Lumbrera Agent Contract

- Read Markdown files directly.
- Do not create, edit, move, delete, or overwrite files directly in a Lumbrera brain repo.
- Run `lumbrera sync --repo <repo>` before relying on local state.
- Use `lumbrera write` for every mutation.
- Do not edit generated files: `INDEX.md`, `CHANGELOG.md`, or `BRAIN.sum`.

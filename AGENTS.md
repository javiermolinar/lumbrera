# Lumbrera Implementation Agent Notes

This repository contains the Lumbrera CLI implementation. It is not itself a Lumbrera brain repository unless explicitly initialized as one for testing.

## Repository boundary

Agents working in this implementation repo may edit source files directly when implementing the CLI.

The Lumbrera brain contract applies to repositories managed by the Lumbrera CLI, not automatically to this implementation repo. In a Lumbrera brain repo, mutations should go through `lumbrera write`; in this implementation repo, normal coding-agent file edits are allowed.

## Product-facing agent instructions

Do not duplicate the Lumbrera brain workflow in this file. This file is for contributors working on the CLI implementation.

The product-facing brain `AGENTS.md` and generated `.agents/skills/*/SKILL.md` contents are scaffolded by `lumbrera init` from `internal/initcmd/scaffold.go`.

When changing product-facing agent behavior, update the scaffold templates and related tests/help output. Avoid adding those brain instructions here.

## Testing

When testing `lumbrera init`, `lumbrera sync`, or `lumbrera write`, use a temporary fixture repo or test directory. Do not accidentally treat the implementation repo as the managed brain repo unless the test explicitly requires it.

## Generated brain files

Files such as `INDEX.md`, `CHANGELOG.md`, `BRAIN.sum`, `sources/`, `wiki/`, and `.brain/` are generated or managed inside Lumbrera brain repos. They should not be created at the root of this implementation repo except as intentional test fixtures.

## Skills

The repo may contain checked-in skill templates for review or distribution, but `lumbrera init` currently scaffolds skills from `internal/initcmd/scaffold.go`. Keep this distinction explicit; do not assume root-level skills are active in generated brain repos unless the implementation wires them in.

## Commit messages

The Lumbrera brain commit subject convention is:

```text
[operation] [actor]: reason
```

That convention is part of the product behavior for managed brain repos. It is optional in this implementation repo. Normal software commit messages such as `chore: scaffold cli` or `feat: implement init` are acceptable here.

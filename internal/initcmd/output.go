package initcmd

import "fmt"

func printHelp() {
	fmt.Println(`Usage:
  lumbrera init <repo>

Initializes <repo> as a Lumbrera brain directory.

A brain is a Markdown knowledge base. It preserves raw sources under sources/
and distilled knowledge under wiki/. After initialization, agents should not
edit brain files directly; they should use lumbrera write.

Creates:
  INDEX.md          generated navigation map
  CHANGELOG.md      generated semantic history from .brain/ops.log
  BRAIN.sum         generated checksum manifest
  AGENTS.md         standing instructions for agents
  CLAUDE.md         symlink to AGENTS.md for Claude
  .agents/skills/   bundled Lumbrera ingest, query, and lint skills
  .claude           symlink to .agents for Claude skills
  sources/          preserved raw source material
  wiki/             distilled knowledge
  .brain/VERSION    Lumbrera brain format marker
  .brain/ops.log    Lumbrera operation log

Behavior:
  - creates <repo> if it does not exist
  - accepts empty directories and common boilerplate files such as README.md,
    LICENSE, and .gitignore
  - refuses existing content directories that are not already Lumbrera brains
  - does not initialize Git, commit, push, or install hooks

Examples:
  lumbrera init ./brain
  lumbrera init /path/to/empty-directory

After init:
  Use the generated AGENTS.md and bundled skills. Agents may read Markdown
  directly, but all mutations should go through lumbrera write.`)
}

func printAlreadyInitialized(repo string) {
	fmt.Printf("Lumbrera brain already initialized at %s\n", repo)
}

func printSuccess(repo string) {
	fmt.Printf(`Initialized Lumbrera brain at %s

Created:
  sources/
  wiki/
  INDEX.md
  CHANGELOG.md
  BRAIN.sum
  AGENTS.md
  CLAUDE.md -> AGENTS.md
  .agents/skills/lumbrera-ingest/SKILL.md
  .agents/skills/lumbrera-query/SKILL.md
  .agents/skills/lumbrera-lint/SKILL.md
  .claude -> .agents
  .brain/VERSION
  .brain/ops.log

Agents should follow AGENTS.md, CLAUDE.md, or the bundled Lumbrera ingest/query/lint skills and use lumbrera write for all future mutations.
`, repo)
}

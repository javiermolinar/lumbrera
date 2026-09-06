package initcmd

import "fmt"

func printHelp() {
	fmt.Println(`Usage:
  lumbrera init <repo>

Initializes <repo> as a Lumbrera brain directory.

A brain is a Markdown knowledge base. It preserves raw sources under sources/,
first-party knowledge under notes/, and distilled knowledge under wiki/. After initialization, agents should not
edit brain files directly; they should use lumbrera write or lumbrera delete. Verify can repair
missing generated managed-document IDs for backward compatibility.

Creates:
  INDEX.md          generated wiki navigation map
  SOURCES.md        generated source listing
  NOTES.md          generated first-party note listing
  ASSETS.md         generated asset listing
  CHANGELOG.md      append-only operation changelog
  BRAIN.sum         generated managed-document checksum manifest
  tags.md           generated read-only tag registry from wiki and note frontmatter
  .gitignore        ignores disposable Lumbrera search cache files
  AGENTS.md         standing instructions for agents
  CLAUDE.md         symlink to AGENTS.md for Claude
  .agents/skills/   bundled Lumbrera ingest, note, query, health, and delete skills
  .claude           symlink to .agents for Claude skills
  sources/          preserved raw source material
  notes/            first-party durable knowledge
  wiki/             distilled knowledge
  assets/           binary attachments (diagrams, images, PDFs)
  VERSION           Lumbrera brain format marker

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
  directly, but all mutations should go through lumbrera write or lumbrera delete.`)
}

func printAlreadyInitialized(repo string) {
	fmt.Printf("Lumbrera brain already initialized at %s\n", repo)
}

func printSuccess(repo string) {
	fmt.Printf(`Initialized Lumbrera brain at %s

Created:
  sources/
  notes/
  wiki/
  assets/
  INDEX.md
  SOURCES.md
  NOTES.md
  ASSETS.md
  CHANGELOG.md
  BRAIN.sum
  tags.md
  .gitignore
  AGENTS.md
  CLAUDE.md -> AGENTS.md
  .agents/skills/lumbrera-ingest/SKILL.md
  .agents/skills/lumbrera-query/SKILL.md
  .agents/skills/lumbrera-note/SKILL.md
  .agents/skills/lumbrera-health/SKILL.md
  .agents/skills/lumbrera-delete/SKILL.md
  .claude -> .agents
  VERSION

Agents should follow AGENTS.md, CLAUDE.md, or the bundled Lumbrera skills. Use lumbrera search before broad file exploration, lumbrera write for mutations, and lumbrera delete for removals.
`, repo)
}

package initcmd

import (
	"fmt"
	"strings"
)

func printHelp() {
	fmt.Println(`Usage:
  lumbrera init <repo>

Initializes <repo> as a local Lumbrera brain repository.

A brain repo is a Git-backed Markdown knowledge base. It preserves raw sources
under sources/ and distilled knowledge under wiki/. After initialization,
agents should not edit files directly; they should use lumbrera write.

Creates:
  INDEX.md          generated navigation map
  CHANGELOG.md      generated semantic history
  BRAIN.sum         generated checksum manifest
  AGENTS.md         standing instructions for agents
  CLAUDE.md         symlink to AGENTS.md for Claude
  .agents/skills/   bundled Lumbrera agent skill
  .claude           symlink to .agents for Claude skills
  sources/          preserved raw source material
  wiki/             distilled knowledge
  .brain/VERSION    Lumbrera brain format marker
  .brain/hooks/     Git hook scripts
  .brain/conflicts/ conflict context for failed sync/write attempts

Behavior:
  - creates <repo> if it does not exist
  - initializes Git if needed
  - accepts empty repos and clean GitHub boilerplate files such as README.md,
    LICENSE, and .gitignore
  - refuses existing content repos that are not already Lumbrera brains
  - installs hooks using core.hooksPath
  - creates an initial local commit
  - does not push

Examples:
  lumbrera init ./brain
  lumbrera init /path/to/empty-git-repo

After init:
  Configure a push remote before the first write if one is not already set.
  Then preserve source material and distill knowledge with lumbrera write.`)
}

func printAlreadyInitialized(repo string) {
	fmt.Printf("Lumbrera brain already initialized at %s\n", repo)
}

func printSuccess(repo, branch string, remotes []string) {
	fmt.Printf(`Initialized Lumbrera brain at %s

Created:
  sources/
  wiki/
  INDEX.md
  CHANGELOG.md
  BRAIN.sum
  AGENTS.md
  CLAUDE.md -> AGENTS.md
  .agents/skills/lumbrera/SKILL.md
  .claude -> .agents
  .brain/VERSION
  .brain/hooks/
  .brain/conflicts/

Created initial local commit:
  [init] [lumbrera]: Initialize Lumbrera brain

`, repo)
	if len(remotes) > 0 {
		if branch == "" {
			branch = "<branch>"
		}
		fmt.Printf("Remote detected: %s\n", strings.Join(remotes, ", "))
		fmt.Printf("Init is local-only. To publish the scaffold, run:\n  git push -u origin %s\n\n", branch)
	} else {
		if branch == "" {
			branch = "main"
		}
		fmt.Printf("No remote configured. Before the first lumbrera write, configure a push remote:\n  git remote add origin <url>\n  git push -u origin %s\n\n", branch)
	}
	fmt.Println("Agents should follow AGENTS.md, CLAUDE.md, or the bundled lumbrera skill and use lumbrera write for all future mutations.")
}

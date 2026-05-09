package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/deletecmd"
	"github.com/javiermolinar/lumbrera/internal/healthcmd"
	"github.com/javiermolinar/lumbrera/internal/indexcmd"
	initcmd "github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/migratecmd"
	"github.com/javiermolinar/lumbrera/internal/movecmd"
	"github.com/javiermolinar/lumbrera/internal/searchcmd"
	"github.com/javiermolinar/lumbrera/internal/verifycmd"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	command, rest := args[0], args[1:]
	switch command {
	case "init":
		return initcmd.Run(rest)
	case "migrate":
		return migratecmd.Run(rest)
	case "move":
		return movecmd.Run(rest)
	case "index":
		return indexcmd.Run(rest)
	case "health":
		return healthcmd.Run(rest)
	case "search":
		return searchcmd.Run(rest)
	case "verify":
		return verifycmd.Run(rest)
	case "delete":
		return deletecmd.Run(rest)
	case "write":
		// Deprecation: intercept --delete and delegate to delete command.
		if writeHasDelete(rest) {
			fmt.Fprintln(os.Stderr, `warning: "write --delete" is deprecated, use "lumbrera delete <path> --reason <reason>" instead`)
			return deletecmd.Run(writeDeleteArgs(rest))
		}
		return writecmd.Run(rest, os.Stdin)
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func printUsage() {
	fmt.Println(`Lumbrera is a Markdown knowledge base for humans and LLM agents.

A Lumbrera brain repo stores:
  sources/   preserved raw source material
  wiki/      distilled knowledge

Agents may read Markdown directly, but all mutations must go through Lumbrera.

Usage:
  lumbrera <command> [options]

Commands:
  init <repo>              Initialize a Lumbrera brain repo
  migrate [options]        Upgrade a v1 brain to v2
  move <from> <to>         Move a file and rewrite all references
  index [options]          Manage the local SQLite search index
  health [options]         Return deterministic health/consolidation candidates
  search <query> [options] Search the local SQLite index with JSON output
  verify [--brain <path>]  Check deterministic brain integrity
  write <path> [options]   Perform one atomic knowledge mutation
  delete <path> [options]  Delete a source or wiki page with cascade cleanup

Run:
  lumbrera <command> --help

Examples:
  lumbrera init ./brain
  lumbrera index --status --brain ./brain
  lumbrera index --rebuild --brain ./brain
  lumbrera health --brain ./brain --json
  lumbrera search "tempo downscale" --brain ./brain --json
  lumbrera write wiki/topic.md --title "Topic" --summary "Durable summary" --tag topic --source sources/input.md --reason "Create topic page" < topic.md
  lumbrera move wiki/old.md wiki/new/path.md --reason "Reorganize"
  lumbrera delete sources/bad.md --reason "Remove poison source"`)
}

// writeHasDelete returns true if the write args contain --delete.
func writeHasDelete(args []string) bool {
	for _, arg := range args {
		if arg == "--delete" {
			return true
		}
	}
	return false
}

// writeDeleteArgs converts write args into delete args by stripping --delete
// and any write-only flags (--title, --summary, --tag, --source, --append).
func writeDeleteArgs(args []string) []string {
	var out []string
	skipNext := false
	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--delete" {
			continue
		}
		// Strip write-only flags that delete doesn't accept.
		name, _, _ := strings.Cut(arg, "=")
		switch name {
		case "--title", "--summary", "--tag", "--source", "--append":
			// If value is not inline (no =), skip the next arg too.
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				skipNext = true
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

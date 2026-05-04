package main

import (
	"fmt"
	"os"

	initcmd "github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/synccmd"
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
	case "sync":
		return synccmd.Run(rest)
	case "verify":
		return verifycmd.Run(rest)
	case "write":
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
	fmt.Println(`Lumbrera is a Git-backed Markdown knowledge base for humans and LLM agents.

A Lumbrera brain repo stores:
  sources/   preserved raw source material
  wiki/      distilled knowledge

Agents may read Markdown directly, but all mutations must go through Lumbrera.

Usage:
  lumbrera <command> [options]

Commands:
  init <repo>              Initialize a Lumbrera brain repo
  sync --repo <repo>       Converge a brain repo to a valid state
  verify [--repo <repo>]   Check deterministic brain integrity
  write <path> [options]   Perform one atomic knowledge mutation

Run:
  lumbrera <command> --help

Examples:
  lumbrera init ./brain
  lumbrera sync --repo ./brain
  lumbrera write wiki/topic.md --title "Topic" --source sources/input.md --reason "Create topic page" < topic.md`)
}

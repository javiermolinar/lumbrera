package main

import (
	"fmt"
	"os"

	"github.com/javiermolinar/lumbrera/internal/indexcmd"
	initcmd "github.com/javiermolinar/lumbrera/internal/initcmd"
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
	case "index":
		return indexcmd.Run(rest)
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
	fmt.Println(`Lumbrera is a Markdown knowledge base for humans and LLM agents.

A Lumbrera brain repo stores:
  sources/   preserved raw source material
  wiki/      distilled knowledge

Agents may read Markdown directly, but all mutations must go through Lumbrera.

Usage:
  lumbrera <command> [options]

Commands:
  init <repo>              Initialize a Lumbrera brain repo
  index [options]          Manage the local SQLite search index
  verify [--brain <path>]  Check deterministic brain integrity
  write <path> [options]   Perform one atomic knowledge mutation

Run:
  lumbrera <command> --help

Examples:
  lumbrera init ./brain
  lumbrera index --status --brain ./brain
  lumbrera index --rebuild --brain ./brain
  lumbrera write wiki/topic.md --title "Topic" --summary "Durable summary" --tag topic --source sources/input.md --reason "Create topic page" < topic.md`)
}

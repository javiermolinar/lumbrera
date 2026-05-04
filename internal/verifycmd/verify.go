package verifycmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain string
	Help  bool
}

func Run(args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp()
		return err
	}
	if opts.Help {
		printHelp()
		return nil
	}
	brainDir, err := resolveBrain(opts.Brain)
	if err != nil {
		return err
	}
	lock, err := brainlock.Acquire(brainDir, "verify")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return err
	}
	fmt.Printf("Lumbrera verify passed: %s\n", brainDir)
	return nil
}

func parseArgs(args []string) (options, error) {
	for _, arg := range args {
		if isHelp(arg) {
			return options{Help: true}, nil
		}
	}
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	var opts options
	fs.StringVar(&opts.Brain, "brain", "", "target Lumbrera brain directory")
	fs.StringVar(&opts.Brain, "repo", "", "deprecated alias for --brain")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("verify does not accept positional arguments")
	}
	return opts, nil
}

func resolveBrain(brainDir string) (string, error) {
	if strings.TrimSpace(brainDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		brainDir = cwd
	}
	abs, err := filepath.Abs(brainDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func printHelp() {
	fmt.Println(`Verify a Lumbrera brain repo for deterministic consistency.

Usage:
  lumbrera verify [--brain <path>]

Behavior:
  - repairs missing wiki frontmatter document IDs for backward compatibility
  - then checks deterministic consistency

Checks:
  - .brain/VERSION matches the supported brain format
  - content paths obey Lumbrera policy
  - wiki documents have valid generated frontmatter
  - wiki pages have resolving source references
  - local Markdown links and heading anchors resolve
  - INDEX.md, CHANGELOG.md, BRAIN.sum, and tags.md match regenerated output

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --repo <path>       deprecated alias for --brain

This command may repair missing generated document IDs. Knowledge mutations should still use lumbrera write.`)
}

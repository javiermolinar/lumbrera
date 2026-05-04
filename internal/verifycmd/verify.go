package verifycmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/git"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Repo          string
	SkipChangelog bool
	Help          bool
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
	if err := git.EnsureAvailable(); err != nil {
		return err
	}
	repo, err := resolveRepo(opts.Repo)
	if err != nil {
		return err
	}
	if err := verify.Run(repo, verify.Options{SkipChangelog: opts.SkipChangelog}); err != nil {
		return err
	}
	fmt.Printf("Lumbrera verify passed: %s\n", repo)
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
	fs.StringVar(&opts.Repo, "repo", "", "target Lumbrera brain repo")
	fs.BoolVar(&opts.SkipChangelog, "skip-changelog", false, "skip CHANGELOG.md drift checks; intended for pre-commit hooks before the commit subject exists")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("verify does not accept positional arguments")
	}
	return opts, nil
}

func resolveRepo(repo string) (string, error) {
	if strings.TrimSpace(repo) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		if root, err := git.WorkTreeRoot(cwd); err == nil {
			repo = root
		} else {
			repo = cwd
		}
	}
	abs, err := filepath.Abs(repo)
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
  lumbrera verify [--repo <repo>] [--skip-changelog]

Checks:
  - .brain/VERSION matches the supported brain format
  - content paths obey Lumbrera policy
  - source and wiki documents have valid generated frontmatter
  - wiki pages have resolving source references
  - local Markdown links and heading anchors resolve
  - INDEX.md, CHANGELOG.md, and BRAIN.sum match regenerated output

Options:
  --repo <repo>       target brain repo, defaults to the current Git worktree root
  --skip-changelog    skip CHANGELOG.md drift checks for pre-commit hooks

This command is primarily for hooks and diagnostics. Knowledge mutations should still use lumbrera write.`)
}

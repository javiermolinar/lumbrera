package indexcmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain   string
	Status  bool
	Rebuild bool
	Help    bool
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

	ctx := context.Background()
	if opts.Status {
		status, err := searchindex.CheckStatus(ctx, brainDir)
		if err != nil {
			return err
		}
		printStatus(brainDir, status)
		return nil
	}

	if _, err := searchindex.CheckStatus(ctx, brainDir); err != nil {
		return err
	}

	lock, err := brainlock.Acquire(brainDir, "index")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("cannot rebuild search index because brain verification failed: %w; run lumbrera verify --brain %s", err, brainDir)
	}
	if err := searchindex.RebuildBrain(ctx, brainDir); err != nil {
		return err
	}
	fmt.Printf("Lumbrera search index rebuilt: %s\n", searchindex.SearchIndexPath(brainDir))
	return nil
}

func parseArgs(args []string) (options, error) {
	for _, arg := range args {
		if isHelp(arg) {
			return options{Help: true}, nil
		}
	}

	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	var opts options
	fs.StringVar(&opts.Brain, "brain", "", "target Lumbrera brain directory")
	fs.StringVar(&opts.Brain, "repo", "", "deprecated alias for --brain")
	fs.BoolVar(&opts.Status, "status", false, "report search index freshness without mutating files")
	fs.BoolVar(&opts.Rebuild, "rebuild", false, "force a full deterministic search index rebuild")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("index does not accept positional arguments")
	}
	if opts.Status == opts.Rebuild {
		return options{}, fmt.Errorf("index requires exactly one of --status or --rebuild")
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

func printStatus(brainDir string, status searchindex.Status) {
	fmt.Printf("Lumbrera search index status: %s\n", status.State)
	fmt.Printf("brain: %s\n", brainDir)
	fmt.Printf("index: %s\n", status.Path)
	fmt.Printf("exists: %t\n", status.Exists)
	fmt.Printf("schema_version: %d\n", status.SchemaVersion)
	fmt.Printf("expected_schema_version: %d\n", status.ExpectedVersion)
	if status.ManifestHash != "" {
		fmt.Printf("manifest_hash: %s\n", status.ManifestHash)
	}
	if status.ExpectedHash != "" {
		fmt.Printf("expected_manifest_hash: %s\n", status.ExpectedHash)
	}
	if status.Reason != "" {
		fmt.Printf("reason: %s\n", status.Reason)
	}
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func printHelp() {
	fmt.Println(`Manage the local Lumbrera SQLite search index.

Usage:
  lumbrera index --status [--brain <path>]
  lumbrera index --rebuild [--brain <path>]

Behavior:
  - --status reports whether .brain/search.sqlite is missing, fresh, stale, or incompatible
  - --status does not mutate files
  - --rebuild verifies the brain and replaces .brain/search.sqlite with a deterministic full rebuild

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --repo <path>       deprecated alias for --brain
  --status            report search index freshness without mutating files
  --rebuild           force a full deterministic search index rebuild`)
}

package migratecmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/cmdutil"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain string
	Actor string
	Help  bool
}

func Run(args []string) (err error) {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp()
		return err
	}
	if opts.Help {
		printHelp()
		return nil
	}

	brainDir, err := cliutil.ResolveBrain(opts.Brain)
	if err != nil {
		return err
	}

	version, err := brain.RepoVersion(brainDir)
	if err != nil {
		return err
	}
	if version == brain.Version {
		return fmt.Errorf("brain is already %s; nothing to migrate", brain.Version)
	}

	lock, err := brainlock.Acquire(brainDir, "migrate")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = defaultActor()
	}

	// Create assets/ directory if missing.
	assetsDir := filepath.Join(brainDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("create assets/: %w", err)
	}

	// Regenerate all generated files in v2 format.
	files, err := generate.FilesForRepo(brainDir)
	if err != nil {
		return err
	}
	if err := generate.WriteFiles(brainDir, files); err != nil {
		return err
	}

	// Update VERSION marker.
	versionPath := filepath.Join(brainDir, brain.MarkerPath)
	if err := os.WriteFile(versionPath, []byte(brain.Version+"\n"), 0o644); err != nil {
		return err
	}

	// Log the migration.
	entry := ops.NewEntry("migrate", opts.Actor, fmt.Sprintf("Upgrade brain %s → %s", brain.VersionV1, brain.Version), time.Now())
	if err := ops.Append(brainDir, entry); err != nil {
		return err
	}

	// Verify integrity.
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("post-migration verify failed: %w", err)
	}

	fmt.Printf("Migrated brain from %s to %s: %s\n", brain.VersionV1, brain.Version, brainDir)
	return nil
}

func parseArgs(args []string) (options, error) {
	for _, arg := range args {
		if cmdutil.IsHelp(arg) {
			return options{Help: true}, nil
		}
	}
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	var opts options
	fs.StringVar(&opts.Brain, "brain", "", "target Lumbrera brain directory")
	fs.StringVar(&opts.Actor, "actor", "", "actor name for changelog entry")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("migrate does not accept positional arguments")
	}
	return opts, nil
}

func defaultActor() string {
	for _, key := range []string{"LUMBRERA_ACTOR", "USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "human"
}

func printHelp() {
	fmt.Println(`Upgrade a Lumbrera brain repo from v1 to v2.

Usage:
  lumbrera migrate [--brain <path>] [--actor <name>]

Behavior:
  - creates assets/ directory
  - splits INDEX.md into INDEX.md (wiki), SOURCES.md, and ASSETS.md
  - updates VERSION to lumbrera-brain-v2
  - logs a changelog entry for the migration
  - runs verify to confirm integrity

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --actor <name>      actor name for changelog entry`)
}

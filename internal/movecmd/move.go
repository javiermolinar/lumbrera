package movecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

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
	if err := brain.RequireV2(brainDir); err != nil {
		return err
	}

	lock, err := brainlock.Acquire(brainDir, "move")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return err
	}

	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = defaultActor()
	}
	if err := validateCommitFields(opts.Actor, opts.Reason); err != nil {
		return err
	}

	from, fromKind, err := pathpolicy.NormalizeTargetPath(opts.From)
	if err != nil {
		return fmt.Errorf("invalid source path: %w", err)
	}
	to, toKind, err := pathpolicy.NormalizeTargetPath(opts.To)
	if err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}
	if fromKind != toKind {
		return fmt.Errorf("cannot move across content roots: %s (%s) → %s (%s)", from, fromKind, to, toKind)
	}
	if from == to {
		return fmt.Errorf("source and destination are the same: %s", from)
	}

	if err := pathpolicy.EnsureSafeFilesystemTarget(brainDir, from); err != nil {
		return err
	}
	if err := pathpolicy.EnsureSafeFilesystemTarget(brainDir, to); err != nil {
		return err
	}

	absFrom := filepath.Join(brainDir, filepath.FromSlash(from))
	exists, err := pathpolicy.FileExists(absFrom)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot move %s: file does not exist", from)
	}

	absTo := filepath.Join(brainDir, filepath.FromSlash(to))
	destExists, err := pathpolicy.FileExists(absTo)
	if err != nil {
		return err
	}
	if destExists {
		return fmt.Errorf("cannot move to %s: file already exists", to)
	}

	// Plan the move: collect all wiki pages that need link rewriting.
	allRefs, err := loadWikiRefs(brainDir)
	if err != nil {
		return err
	}

	wikiUpdates, err := planMove(from, to, fromKind, allRefs)
	if err != nil {
		return err
	}

	// Build backup.
	backup, err := newMoveBackup(brainDir, from, to, wikiUpdates)
	if err != nil {
		return err
	}

	mutated := false
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if mutated {
			if rollbackErr := backup.Restore(); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}

	mutated = true

	// 1. Move the file.
	if err := os.MkdirAll(filepath.Dir(absTo), 0o755); err != nil {
		return fail(err)
	}
	content, err := os.ReadFile(absFrom)
	if err != nil {
		return fail(err)
	}

	// For wiki pages, update internal relative links and Sources section.
	if fromKind == "wiki" {
		content, err = rewriteMovedWikiPage(content, from, to)
		if err != nil {
			return fail(err)
		}
	}

	if err := os.WriteFile(absTo, content, 0o644); err != nil {
		return fail(err)
	}
	if err := os.Remove(absFrom); err != nil {
		return fail(err)
	}

	// 2. Rewrite links in affected wiki pages.
	for _, updated := range wikiUpdates {
		if err := writeWikiRef(brainDir, updated); err != nil {
			return fail(err)
		}
	}

	// 3. Log operation.
	entry := ops.NewEntry("move", opts.Actor, fmt.Sprintf("%s → %s: %s", from, to, opts.Reason), time.Now())
	if err := ops.Append(brainDir, entry); err != nil {
		return fail(err)
	}

	// 4. Regenerate generated files.
	files, err := generate.FilesForRepo(brainDir)
	if err != nil {
		return fail(err)
	}
	if err := generate.WriteFiles(brainDir, files); err != nil {
		return fail(err)
	}

	// 5. Verify.
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fail(err)
	}

	fmt.Printf("Moved %s → %s\n", from, to)
	for path := range wikiUpdates {
		fmt.Printf("  updated %s\n", path)
	}
	fmt.Printf("Applied Lumbrera move: [move] [%s]: %s → %s: %s\n", opts.Actor, from, to, opts.Reason)
	return nil
}

package migratecmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
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
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = defaultActor()
	}
	if err := validateActor(opts.Actor); err != nil {
		return err
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

	backup, err := newMigrationBackup(brainDir)
	if err != nil {
		return err
	}
	mutated := false
	fail := func(cause error) error {
		if cause == nil || !mutated {
			return cause
		}
		if restoreErr := backup.Restore(); restoreErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", cause, restoreErr)
		}
		return cause
	}

	originalVersion := version
	manualUpdates := []string{}
	if version == brain.VersionV1 {
		if err := verify.CheckVersion(brainDir, brain.VersionV1, verify.Options{}); err != nil {
			return fmt.Errorf("pre-migration %s verification failed: %w", brain.VersionV1, err)
		}
		mutated = true
		if err := migrateV1ToV2(brainDir, opts.Actor); err != nil {
			return fail(err)
		}
		version = brain.VersionV2
	}
	if version == brain.VersionV2 {
		if err := verify.CheckVersion(brainDir, brain.VersionV2, verify.Options{}); err != nil {
			return fail(fmt.Errorf("pre-migration %s verification failed: %w", brain.VersionV2, err))
		}
		mutated = true
		manualUpdates, err = migrateV2ToV3(brainDir, opts.Actor)
		if err != nil {
			return fail(err)
		}
	}

	fmt.Printf("Migrated brain from %s to %s: %s\n", originalVersion, brain.Version, brainDir)
	for _, path := range manualUpdates {
		fmt.Printf("Manual update required: reconcile %s with the v3 scaffold; the existing path was customized, non-regular, or missing.\n", path)
	}
	return nil
}

func migrateV1ToV2(repo, actor string) error {
	if err := createRequiredDirectory(filepath.Join(repo, "assets")); err != nil {
		return fmt.Errorf("create assets/: %w", err)
	}
	files, err := generate.FilesForRepoVersion(repo, brain.VersionV2)
	if err != nil {
		return err
	}
	if err := generate.WriteFilesForVersion(repo, brain.VersionV2, files); err != nil {
		return err
	}
	if err := writeVersion(repo, brain.VersionV2); err != nil {
		return err
	}
	entry := ops.NewEntry("migrate", actor, fmt.Sprintf("Upgrade brain %s → %s", brain.VersionV1, brain.VersionV2), time.Now())
	if err := ops.Append(repo, entry); err != nil {
		return err
	}
	if err := verify.CheckVersion(repo, brain.VersionV2, verify.Options{}); err != nil {
		return fmt.Errorf("post-migration %s verification failed: %w", brain.VersionV2, err)
	}
	return nil
}

func migrateV2ToV3(repo, actor string) ([]string, error) {
	if err := createRequiredDirectory(filepath.Join(repo, "notes")); err != nil {
		return nil, fmt.Errorf("create notes/: %w", err)
	}
	noteSkillNeedsManualUpdate, err := installNoteSkill(repo)
	if err != nil {
		return nil, err
	}
	manualUpdates, err := updateScaffoldedAgentFiles(repo)
	if err != nil {
		return nil, err
	}
	if noteSkillNeedsManualUpdate {
		noteSkillPath, _ := initcmd.NoteSkillTemplate()
		manualUpdates = append(manualUpdates, noteSkillPath)
	}
	if _, err := searchindex.RepairMissingModifiedDates(repo, time.Now().Format("2006-01-02")); err != nil {
		return nil, fmt.Errorf("repair legacy managed-document modified dates: %w", err)
	}

	files, err := generate.FilesForRepo(repo)
	if err != nil {
		return nil, err
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		return nil, err
	}
	if err := writeVersion(repo, brain.Version); err != nil {
		return nil, err
	}
	if err := removeSearchCache(repo); err != nil {
		return nil, err
	}
	entry := ops.NewEntry("migrate", actor, fmt.Sprintf("Upgrade brain %s → %s", brain.VersionV2, brain.Version), time.Now())
	if err := ops.Append(repo, entry); err != nil {
		return nil, err
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		return nil, fmt.Errorf("post-migration %s verification failed: %w", brain.Version, err)
	}
	return manualUpdates, nil
}

func createRequiredDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s must be a real directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func installNoteSkill(repo string) (needsManualUpdate bool, err error) {
	rel, content := initcmd.NoteSkillTemplate()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return true, nil
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		return string(existing) != content, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := createRequiredDirectory(filepath.Dir(path)); err != nil {
		return false, fmt.Errorf("create note skill directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return false, err
	}
	return false, file.Close()
}

func updateScaffoldedAgentFiles(repo string) ([]string, error) {
	var manual []string
	for _, template := range initcmd.MigrationTemplates() {
		needsManualUpdate, err := updateScaffoldedAgentFile(repo, template)
		if err != nil {
			return nil, err
		}
		if needsManualUpdate {
			manual = append(manual, template.Path)
		}
	}
	return manual, nil
}

func updateScaffoldedAgentFile(repo string, template initcmd.MigrationTemplate) (needsManualUpdate bool, err error) {
	path := filepath.Join(repo, filepath.FromSlash(template.Path))
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return true, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if string(content) == template.Content {
		return false, nil
	}
	hash := sha256.Sum256(content)
	digest := hex.EncodeToString(hash[:])
	if !containsHash(template.LegacySHA256, digest) {
		return true, nil
	}
	if err := os.WriteFile(path, []byte(template.Content), info.Mode().Perm()); err != nil {
		return false, err
	}
	return false, nil
}

func containsHash(hashes []string, want string) bool {
	for _, hash := range hashes {
		if hash == want {
			return true
		}
	}
	return false
}

func removeSearchCache(repo string) error {
	path := searchindex.SearchIndexPath(repo)
	for _, candidate := range []string{path, path + "-journal", path + "-wal", path + "-shm"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func writeVersion(repo, version string) error {
	return os.WriteFile(filepath.Join(repo, brain.MarkerPath), []byte(version+"\n"), 0o644)
}

func validateActor(actor string) error {
	entry := ops.NewEntry("migrate", actor, "Validate migration actor", time.Now())
	return ops.Validate(entry)
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
	fmt.Println(`Upgrade a supported older Lumbrera brain to the current version.

Usage:
  lumbrera migrate [--brain <path>] [--actor <name>]

Behavior:
  - migrates v1 brains through v2 and v3 in ordered steps
  - verifies each old contract before mutation
  - creates notes/ and NOTES.md for v3
  - installs the note skill when absent
  - updates unmodified scaffolded agent files and preserves customized files
  - removes the disposable SQLite search cache
  - updates VERSION to lumbrera-brain-v3
  - logs migration entries and verifies the final brain
  - rolls back every touched file if any step fails

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --actor <name>      actor name for changelog entry`)
}

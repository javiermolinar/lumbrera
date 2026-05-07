package deletecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

// Run executes the delete command.
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
	if err := brain.ValidateRepo(brainDir); err != nil {
		return err
	}

	lock, err := brainlock.Acquire(brainDir, "delete")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	// Preflight: brain must be healthy before we mutate.
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return err
	}

	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor, err = defaultActor()
		if err != nil {
			return err
		}
	}
	if err := validateCommitFields(opts.Actor, opts.Reason); err != nil {
		return err
	}

	target, kind, err := pathpolicy.NormalizeTargetPath(opts.Target)
	if err != nil {
		return err
	}
	if err := pathpolicy.EnsureSafeFilesystemTarget(brainDir, target); err != nil {
		return err
	}

	absTarget := filepath.Join(brainDir, filepath.FromSlash(target))
	exists, err := pathpolicy.FileExists(absTarget)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot delete %s: file does not exist", target)
	}

	// Load all wiki references for cascade planning.
	allRefs, err := loadWikiRefs(brainDir)
	if err != nil {
		return err
	}

	filesToDelete, wikiUpdates, err := planCascade(brainDir, target, kind, allRefs)
	if err != nil {
		return err
	}

	// Build backup of all files we'll touch.
	backup, err := newDeleteBackup(brainDir, filesToDelete, wikiUpdates)
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

	// 1. Write updated wiki pages (those that lost a source or a link but survive).
	for _, updated := range wikiUpdates {
		if err := writeWikiRef(brainDir, updated); err != nil {
			return fail(err)
		}
	}

	// 2. Delete files.
	for _, path := range filesToDelete {
		abs := filepath.Join(brainDir, filepath.FromSlash(path))
		if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
	}

	// 3. Append ops entries.
	now := time.Now()
	for _, path := range filesToDelete {
		reason := opts.Reason
		if path != target {
			reason = fmt.Sprintf("cascade from %s: %s", target, opts.Reason)
		}
		entry := ops.NewEntry("delete", opts.Actor, reason, now)
		if err := ops.Append(brainDir, entry); err != nil {
			return fail(err)
		}
	}

	// 4. Regenerate generated files.
	files, err := generate.FilesForRepo(brainDir)
	if err != nil {
		return fail(err)
	}
	if err := generate.WriteFiles(brainDir, files); err != nil {
		return fail(err)
	}

	// 5. Verify integrity.
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fail(err)
	}

	// Report.
	fmt.Printf("Deleted %s\n", target)
	for _, path := range filesToDelete {
		if path != target {
			fmt.Printf("  cascade-deleted %s\n", path)
		}
	}
	for path := range wikiUpdates {
		fmt.Printf("  updated %s\n", path)
	}
	fmt.Printf("Applied Lumbrera delete: [delete] [%s]: %s\n", opts.Actor, opts.Reason)
	return nil
}

// writeWikiRef writes the updated wiki ref back to disk with regenerated frontmatter.
func writeWikiRef(repo string, ref wikiRef) error {
	absPath := filepath.Join(repo, filepath.FromSlash(ref.relPath))

	// Re-analyze body to ensure links/sources are fresh.
	analysis, err := md.AnalyzeWithOptions(ref.relPath, ref.body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return fmt.Errorf("re-analyze %s: %w", ref.relPath, err)
	}

	links := filterWikiLinks(analysis.Links)
	citationPaths := referencePaths(analysis.SourceCitations)
	sources := mergePaths(ref.meta.Lumbrera.Sources, citationPaths)

	meta := frontmatter.NewWithID(
		ref.meta.Lumbrera.ID,
		ref.meta.Lumbrera.Kind,
		ref.meta.Title,
		ref.meta.Summary,
		ref.meta.Tags,
		sources,
		links,
	)
	meta.Lumbrera.ModifiedDate = ref.meta.Lumbrera.ModifiedDate

	content, err := frontmatter.Attach(meta, ref.body)
	if err != nil {
		return fmt.Errorf("attach frontmatter %s: %w", ref.relPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// defaultActor reads the actor from environment variables.
func defaultActor() (string, error) {
	for _, key := range []string{"LUMBRERA_ACTOR", "USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return sanitizeActor(value), nil
		}
	}
	return "human", nil
}

func sanitizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	actor = strings.ReplaceAll(actor, "]", "")
	actor = strings.ReplaceAll(actor, "\n", " ")
	actor = strings.ReplaceAll(actor, "\r", " ")
	if actor == "" {
		return "human"
	}
	return actor
}

func validateCommitFields(actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.ContainsAny(reason, "\r\n") {
		return fmt.Errorf("--reason must be a single line")
	}
	if actor != "" && strings.ContainsAny(actor, "]\r\n") {
		return fmt.Errorf("--actor must not contain ], carriage returns, or newlines")
	}
	return nil
}

// deleteBackup stores file contents for atomic rollback.
type deleteBackup struct {
	files []fileBackup
}

type fileBackup struct {
	path    string
	exists  bool
	content []byte
}

func newDeleteBackup(brainDir string, filesToDelete []string, wikiUpdates map[string]wikiRef) (*deleteBackup, error) {
	// Collect all paths we need to back up.
	seen := map[string]struct{}{}
	var paths []string

	add := func(rel string) {
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		paths = append(paths, rel)
	}

	for _, p := range filesToDelete {
		add(p)
	}
	for p := range wikiUpdates {
		add(p)
	}
	for _, p := range brain.GeneratedFilePaths() {
		add(p)
	}
	add(ops.LogPath)

	backup := &deleteBackup{}
	for _, rel := range paths {
		abs := filepath.Join(brainDir, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err == nil {
			backup.files = append(backup.files, fileBackup{path: abs, exists: true, content: content})
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			backup.files = append(backup.files, fileBackup{path: abs})
			continue
		}
		return nil, err
	}
	return backup, nil
}

func (b *deleteBackup) Restore() error {
	if b == nil {
		return nil
	}
	for _, file := range b.files {
		if file.exists {
			if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(file.path, file.content, 0o644); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

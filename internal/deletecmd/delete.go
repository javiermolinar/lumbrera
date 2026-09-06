package deletecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	if err := brain.RequireCurrent(brainDir); err != nil {
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

	// Load every managed document before planning the complete cascade.
	allRefs, err := loadManagedRefs(brainDir)
	if err != nil {
		return err
	}

	filesToDelete, managedUpdates, err := planCascade(brainDir, target, kind, allRefs)
	if err != nil {
		return err
	}

	// Build backup of all files we'll touch.
	backup, err := newDeleteBackup(brainDir, filesToDelete, managedUpdates)
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

	// 1. Rewrite surviving managed documents that lost evidence or a link.
	updatedPaths := make([]string, 0, len(managedUpdates))
	for path := range managedUpdates {
		updatedPaths = append(updatedPaths, path)
	}
	sort.Strings(updatedPaths)
	for _, path := range updatedPaths {
		if err := writeManagedRef(brainDir, managedUpdates[path]); err != nil {
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
	for _, path := range updatedPaths {
		fmt.Printf("  updated %s\n", path)
	}
	fmt.Printf("Applied Lumbrera delete: [delete] [%s]: %s\n", opts.Actor, opts.Reason)
	return nil
}

// writeManagedRef writes one planned managed-document update with regenerated
// links, evidence metadata, and policy-controlled frontmatter.
func writeManagedRef(repo string, ref managedRef) error {
	absPath := filepath.Join(repo, filepath.FromSlash(ref.relPath))
	analysis, err := md.AnalyzeWithOptions(ref.relPath, ref.body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(ref.policy.Kind)})
	if err != nil {
		return fmt.Errorf("re-analyze %s: %w", ref.relPath, err)
	}

	evidence := mergePaths(ref.meta.Lumbrera.Sources, referencePaths(analysis.SourceCitations))
	if !brain.AcceptsEvidence(ref.policy.Kind) {
		evidence = nil
	}
	meta := frontmatter.NewWithID(
		ref.meta.Lumbrera.ID,
		string(ref.policy.Kind),
		ref.meta.Title,
		ref.meta.Summary,
		ref.meta.Tags,
		evidence,
		filterManagedLinks(analysis.Links),
	)
	meta.Lumbrera.ModifiedDate = ref.meta.Lumbrera.ModifiedDate

	content, err := frontmatter.AttachForPolicy(meta, ref.policy, ref.body)
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
	mode    os.FileMode
	content []byte
}

func newDeleteBackup(brainDir string, filesToDelete []string, managedUpdates map[string]managedRef) (*deleteBackup, error) {
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
	for p := range managedUpdates {
		add(p)
	}
	for _, p := range brain.GeneratedFilePaths() {
		add(p)
	}
	add(brain.ChangelogPath)

	backup := &deleteBackup{}
	for _, rel := range paths {
		abs := filepath.Join(brainDir, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err == nil {
			info, statErr := os.Stat(abs)
			if statErr != nil {
				return nil, statErr
			}
			backup.files = append(backup.files, fileBackup{path: abs, exists: true, mode: info.Mode().Perm(), content: content})
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
			if err := os.WriteFile(file.path, file.content, file.mode); err != nil {
				return err
			}
			if err := os.Chmod(file.path, file.mode); err != nil {
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

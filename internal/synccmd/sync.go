package synccmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/git"
	"github.com/javiermolinar/lumbrera/internal/repolock"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

const syncCommitSubject = "[sync] [lumbrera]: Regenerate generated files"

type options struct {
	Repo string
	Help bool
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
	if err := git.EnsureAvailable(); err != nil {
		return err
	}
	repo, err := resolveRepo(opts.Repo)
	if err != nil {
		return err
	}
	if !git.IsRepo(repo) {
		return fmt.Errorf("%s is not a Git worktree root", repo)
	}
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}
	lock, err := repolock.Acquire(repo, "sync")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	if err := discardDirtyGeneratedOnly(repo); err != nil {
		return err
	}
	upstream, err := git.Upstream(repo)
	if err != nil {
		return fmt.Errorf("sync requires a configured upstream remote; after init run git remote add origin <url> and git push -u origin main: %w", err)
	}
	if err := git.Fetch(repo); err != nil {
		return fmt.Errorf("failed to fetch remote changes before sync: %w", err)
	}
	if err := git.Rebase(repo, upstream); err != nil {
		_ = git.RebaseAbort(repo)
		return fmt.Errorf("failed to rebase onto %s during sync; resolve conflicts before retrying: %w", upstream, err)
	}
	if err := ensureClean(repo); err != nil {
		return err
	}

	base, err := currentHead(repo)
	if err != nil {
		return err
	}
	if err := verify.ValidatePathPolicy(repo); err != nil {
		return err
	}
	if err := verify.ValidateDocuments(repo); err != nil {
		return err
	}

	files, err := generate.FilesForRepo(repo)
	if err != nil {
		return err
	}
	drift, err := generatedDrift(repo, files)
	if err != nil {
		return err
	}
	if !drift {
		if err := verify.Run(repo, verify.Options{}); err != nil {
			return err
		}
		return pushIfAhead(repo, upstream, "Lumbrera sync complete: already current")
	}

	pending := []generate.PendingChangelogEntry{{Date: time.Now(), Subject: syncCommitSubject}}
	files, err = generate.FilesForRepoWithPending(repo, pending)
	if err != nil {
		return err
	}
	mutated := false
	committed := false
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if mutated && !committed {
			if rollbackErr := resetHard(repo, base); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}

	mutated = true
	if err := generate.WriteFiles(repo, files); err != nil {
		return fail(err)
	}
	if err := verify.Run(repo, verify.Options{PendingChangelog: pending}); err != nil {
		return fail(err)
	}
	if err := addGeneratedFiles(repo); err != nil {
		return fail(err)
	}
	if err := git.Commit(repo, syncCommitSubject); err != nil {
		return fail(err)
	}
	committed = true
	if err := ensureClean(repo); err != nil {
		return err
	}
	if err := git.Push(repo); err != nil {
		return fmt.Errorf("sync committed generated repairs locally but push failed; local commit was preserved; inspect the remote before retrying: %w", err)
	}
	fmt.Printf("Lumbrera sync repaired generated files and pushed: %s\n", syncCommitSubject)
	return nil
}

func parseArgs(args []string) (options, error) {
	for _, arg := range args {
		if isHelp(arg) {
			return options{Help: true}, nil
		}
	}
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	var opts options
	fs.StringVar(&opts.Repo, "repo", "", "target Lumbrera brain repo")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("sync does not accept positional arguments")
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
	fmt.Println(`Usage:
  lumbrera sync [--repo <repo>]

Converges a Lumbrera brain clone to a valid generated state.

Behavior:
  - acquires a local repository lock
  - fetches and rebases onto the configured upstream
  - discards uncommitted edits to generated files only
  - rejects uncommitted content edits under sources/ or wiki/
  - regenerates INDEX.md, CHANGELOG.md, and BRAIN.sum
  - commits generated repairs as [sync] [lumbrera]: Regenerate generated files
  - pushes local commits
  - leaves the working tree clean

Options:
  --repo <repo>       target brain repo, default current Git worktree root`)
}

func discardDirtyGeneratedOnly(repo string) error {
	status, err := git.StatusPorcelain(repo)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		path := statusPath(line)
		if path == "" || !isGeneratedPath(path) {
			return fmt.Errorf("working tree has non-generated changes at %s; commit/revert them or use lumbrera write before sync", path)
		}
	}
	if _, err := git.Run(repo, "reset", "--", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath); err != nil {
		return err
	}
	if _, err := git.Run(repo, "checkout", "--", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath); err != nil {
		return err
	}
	if _, err := git.Run(repo, "clean", "-f", "--", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath); err != nil {
		return err
	}
	return ensureClean(repo)
}

func statusPath(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if i := strings.Index(line, " -> "); i >= 0 {
		return strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(line[i+4:])), "/")
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSuffix(filepath.ToSlash(fields[len(fields)-1]), "/")
}

func isGeneratedPath(path string) bool {
	switch path {
	case brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath:
		return true
	default:
		return false
	}
}

func ensureClean(repo string) error {
	clean, err := git.IsClean(repo)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree is not clean after sync operation")
	}
	return nil
}

func currentHead(repo string) (string, error) {
	result, err := git.Run(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(result.Stdout)
	if head == "" {
		return "", fmt.Errorf("git HEAD is empty")
	}
	return head, nil
}

func generatedDrift(repo string, files generate.Files) (bool, error) {
	checks := map[string]string{
		brain.IndexPath:     files.Index,
		brain.ChangelogPath: files.Changelog,
		brain.BrainSumPath:  files.BrainSum,
	}
	for rel, want := range checks {
		got, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return true, nil
		}
		if string(got) != want {
			return true, nil
		}
	}
	return false, nil
}

func addGeneratedFiles(repo string) error {
	_, err := git.Run(repo, "add", "--", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath)
	return err
}

func resetHard(repo, base string) error {
	if strings.TrimSpace(base) == "" {
		return fmt.Errorf("cannot rollback sync without a base commit")
	}
	_, err := git.Run(repo, "reset", "--hard", base)
	return err
}

func pushIfAhead(repo, upstream, message string) error {
	ahead, err := git.AheadCount(repo, upstream)
	if err != nil {
		return err
	}
	if ahead == 0 {
		fmt.Println(message)
		return nil
	}
	if err := git.Push(repo); err != nil {
		return fmt.Errorf("sync could not push %d local commit(s); inspect the remote before retrying: %w", ahead, err)
	}
	fmt.Printf("Lumbrera sync pushed %d local commit(s).\n", ahead)
	return nil
}

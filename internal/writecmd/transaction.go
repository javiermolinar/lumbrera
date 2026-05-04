package writecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/git"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func resolveRepo(repo string) (string, error) {
	if strings.TrimSpace(repo) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root, err := git.WorkTreeRoot(cwd)
		if err == nil {
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

func rollbackWrite(repo, base string) error {
	if strings.TrimSpace(base) == "" {
		return fmt.Errorf("cannot rollback write without a base commit")
	}
	if _, err := git.Run(repo, "reset", "--hard", base); err != nil {
		return err
	}
	_, err := git.Run(repo, "clean", "-fd", "--", "sources", "wiki", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath)
	return err
}

func preflight(repo string) error {
	if err := git.EnsureAvailable(); err != nil {
		return err
	}
	if !git.IsRepo(repo) {
		return fmt.Errorf("%s is not a Git worktree root", repo)
	}
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree is not clean; run lumbrera sync or commit/revert unrelated changes before write")
	}
	if err := fetchAndRebaseBeforeWrite(repo); err != nil {
		return err
	}
	return verify.Run(repo, verify.Options{})
}

func fetchAndRebaseBeforeWrite(repo string) error {
	upstream, err := git.Upstream(repo)
	if err != nil {
		return fmt.Errorf("write requires a configured upstream remote; after init run git remote add origin <url> and git push -u origin main: %w", err)
	}
	if err := git.Fetch(repo); err != nil {
		return fmt.Errorf("failed to fetch remote changes before write: %w", err)
	}
	if err := git.Rebase(repo, upstream); err != nil {
		_ = git.RebaseAbort(repo)
		return fmt.Errorf("failed to rebase onto %s before write; run lumbrera sync or resolve conflicts before retrying: %w", upstream, err)
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree is not clean after remote rebase; resolve local state before write")
	}
	ahead, err := git.AheadCount(repo, upstream)
	if err != nil {
		return err
	}
	if ahead != 0 {
		return fmt.Errorf("local branch has %d unpushed commit(s); run lumbrera sync before write", ahead)
	}
	return nil
}

func defaultActor(repo string) (string, error) {
	result, err := git.Run(repo, "config", "user.name")
	if err == nil {
		name := strings.TrimSpace(result.Stdout)
		if name != "" {
			return sanitizeActor(name), nil
		}
	}
	return "human", nil
}

func validateCommitSubject(actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.ContainsAny(reason, "\r\n") {
		return fmt.Errorf("--reason must be a single line")
	}
	if actor == "" {
		return nil
	}
	if strings.ContainsAny(actor, "]\r\n") {
		return fmt.Errorf("--actor must not contain ], carriage returns, or newlines")
	}
	return nil
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

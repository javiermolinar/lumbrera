package initcmd

import (
	"errors"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/git"
	"github.com/javiermolinar/lumbrera/internal/hooks"
)

func Run(args []string) error {
	if len(args) == 1 && isHelp(args[0]) {
		printHelp()
		return nil
	}
	if len(args) != 1 {
		printHelp()
		return errors.New("init requires exactly one <repo> argument")
	}
	if err := git.EnsureAvailable(); err != nil {
		return err
	}

	repo, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if err := prepareRepoDir(repo); err != nil {
		return err
	}

	state, err := detectInitState(repo)
	if err != nil {
		return err
	}

	switch state {
	case initStateComplete:
		printAlreadyInitialized(repo)
		return nil
	case initStateNeedsInit:
		return completeInit(repo)
	default:
		return errors.New("unknown init state")
	}
}

func completeInit(repo string) error {
	if err := ensureGitRepo(repo); err != nil {
		return err
	}
	if err := ensureScaffold(repo); err != nil {
		return err
	}
	if err := hooks.Install(repo); err != nil {
		return err
	}
	if err := git.AddAll(repo); err != nil {
		return err
	}
	if err := git.Commit(repo, initCommitSubject); err != nil {
		return err
	}

	branch, _ := git.CurrentBranch(repo)
	remotes, _ := git.Remotes(repo)
	printSuccess(repo, branch, remotes)
	return nil
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/git"
)

type initState int

const (
	initStateNeedsInit initState = iota
	initStateComplete
)

func detectInitState(repo string) (initState, error) {
	marker, err := readMarker(repo)
	if err != nil {
		return 0, err
	}
	if marker != "" && marker != brainVersion {
		return 0, fmt.Errorf("refusing to initialize %s: unsupported Lumbrera marker %q", repo, marker)
	}
	if marker == brainVersion {
		return detectMarkedRepoState(repo)
	}
	if exists(filepath.Join(repo, ".brain")) {
		return 0, fmt.Errorf("refusing to initialize %s: .brain/ exists but .brain/VERSION is missing or invalid", repo)
	}
	if !git.IsRepo(repo) {
		if err := validateEmptyNonGitDirectory(repo); err != nil {
			return 0, err
		}
		return initStateNeedsInit, nil
	}

	if err := validateFreshBoilerplate(repo); err != nil {
		return 0, err
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return 0, err
	}
	if !clean {
		return 0, fmt.Errorf("refusing to initialize %s: Git repository has uncommitted changes", repo)
	}
	return initStateNeedsInit, nil
}

func detectMarkedRepoState(repo string) (initState, error) {
	if !git.IsRepo(repo) {
		if err := validatePartialScaffold(repo); err != nil {
			return 0, err
		}
		return initStateNeedsInit, nil
	}
	if !git.HasCommits(repo) {
		if err := validatePartialScaffold(repo); err != nil {
			return 0, err
		}
		return initStateNeedsInit, nil
	}

	status, err := git.StatusPorcelain(repo)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(status) == "" {
		if !markerCommitted(repo) {
			return 0, fmt.Errorf("refusing to treat %s as initialized: .brain/VERSION exists but is not committed", repo)
		}
		return initStateComplete, nil
	}
	if markerCommitted(repo) || !statusOnlyInitScaffold(status) {
		return 0, fmt.Errorf("refusing to resume initialization in %s: Git repository has uncommitted changes", repo)
	}
	if err := validatePartialScaffold(repo); err != nil {
		return 0, err
	}
	return initStateNeedsInit, nil
}

func prepareRepoDir(repo string) error {
	info, err := os.Stat(repo)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("refusing to initialize %s: path exists and is not a directory", repo)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(repo, 0o755)
}

func ensureGitRepo(repo string) error {
	if git.IsRepo(repo) {
		return nil
	}
	return git.Init(repo)
}

func readMarker(repo string) (string, error) {
	content, err := os.ReadFile(filepath.Join(repo, markerPath))
	if err == nil {
		return strings.TrimSpace(string(content)), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", err
}

func markerCommitted(repo string) bool {
	_, err := git.Run(repo, "cat-file", "-e", "HEAD:"+markerPath)
	return err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

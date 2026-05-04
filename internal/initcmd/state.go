package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		return initStateComplete, nil
	}
	if exists(filepath.Join(repo, ".brain")) {
		return 0, fmt.Errorf("refusing to initialize %s: .brain/ exists but .brain/VERSION is missing or invalid", repo)
	}
	if err := validateFreshBoilerplate(repo); err != nil {
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

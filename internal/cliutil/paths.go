package cliutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveBrain returns an absolute, cleaned Lumbrera brain directory. Empty
// input resolves to the current working directory.
func ResolveBrain(brainDir string) (string, error) {
	if strings.TrimSpace(brainDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		brainDir = cwd
	}
	abs, err := filepath.Abs(brainDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

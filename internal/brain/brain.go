package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Version       = "lumbrera-brain-v1"
	MarkerPath    = ".brain/VERSION"
	IndexPath     = "INDEX.md"
	ChangelogPath = "CHANGELOG.md"
	BrainSumPath  = "BRAIN.sum"
)

func ValidateRepo(repo string) error {
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(MarkerPath)))
	if err != nil {
		return fmt.Errorf("not a Lumbrera brain repo: missing %s: %w", MarkerPath, err)
	}
	marker := strings.TrimSpace(string(content))
	if marker != Version {
		return fmt.Errorf("unsupported Lumbrera brain marker %q; expected %q", marker, Version)
	}
	return nil
}

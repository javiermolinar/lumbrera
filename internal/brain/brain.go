package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Version          = "lumbrera-brain-v2"
	VersionV1        = "lumbrera-brain-v1"
	MarkerPath       = "VERSION"
	IndexPath        = "INDEX.md"
	SourcesIndexPath = "SOURCES.md"
	AssetsIndexPath  = "ASSETS.md"
	ChangelogPath    = "CHANGELOG.md"
	BrainSumPath     = "BRAIN.sum"
	TagsPath         = "tags.md"
	MaxWikiBodyLines = 400
)

func GeneratedFilePaths() []string {
	return []string{IndexPath, SourcesIndexPath, AssetsIndexPath, BrainSumPath, TagsPath}
}

// RepoVersion reads the VERSION marker and returns the version string.
// Returns an error if the file is missing or unrecognized.
func RepoVersion(repo string) (string, error) {
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(MarkerPath)))
	if err != nil {
		return "", fmt.Errorf("not a Lumbrera brain repo: missing %s: %w", MarkerPath, err)
	}
	marker := strings.TrimSpace(string(content))
	switch marker {
	case Version, VersionV1:
		return marker, nil
	default:
		return "", fmt.Errorf("unsupported Lumbrera brain marker %q", marker)
	}
}

// ValidateRepo checks that the repo is a valid Lumbrera brain (any version).
func ValidateRepo(repo string) error {
	_, err := RepoVersion(repo)
	return err
}

// RequireV2 checks that the repo is a v2 brain. Returns a helpful error if v1.
func RequireV2(repo string) error {
	v, err := RepoVersion(repo)
	if err != nil {
		return err
	}
	if v == VersionV1 {
		return fmt.Errorf("brain is %s; run \"lumbrera migrate\" to upgrade to %s", VersionV1, Version)
	}
	return nil
}

package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Version                     = "lumbrera-brain-v3"
	VersionV2                   = "lumbrera-brain-v2"
	VersionV1                   = "lumbrera-brain-v1"
	MarkerPath                  = "VERSION"
	IndexPath                   = "INDEX.md"
	SourcesIndexPath            = "SOURCES.md"
	NotesIndexPath              = "NOTES.md"
	AssetsIndexPath             = "ASSETS.md"
	ChangelogPath               = "CHANGELOG.md"
	BrainSumPath                = "BRAIN.sum"
	TagsPath                    = "tags.md"
	MaxManagedDocumentBodyLines = 400
)

func GeneratedFilePaths() []string {
	paths := make([]string, 0, 6)
	for _, kind := range []Kind{KindWiki, KindSource, KindNote, KindAsset} {
		if path, ok := CatalogPathForKind(kind); ok {
			paths = append(paths, path)
		}
	}
	return append(paths, BrainSumPath, TagsPath)
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
	case Version, VersionV2, VersionV1:
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

// RequireCurrent checks that the repo uses the current brain contract.
func RequireCurrent(repo string) error {
	version, err := RepoVersion(repo)
	if err != nil {
		return err
	}
	if version != Version {
		return fmt.Errorf("brain is %s; run \"lumbrera migrate\" to upgrade to %s", version, Version)
	}
	return nil
}

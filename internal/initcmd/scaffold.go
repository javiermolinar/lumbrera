package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	brainVersion      = "lumbrera-brain-v1"
	initCommitSubject = "[init] [lumbrera]: Initialize Lumbrera brain"
	markerPath        = ".brain/VERSION"
)

var scaffoldDirs = []string{
	"sources",
	"wiki",
	".brain/conflicts",
	".brain/hooks",
}

var scaffoldFiles = map[string]string{
	markerPath:     brainVersion + "\n",
	"INDEX.md":     indexContent,
	"CHANGELOG.md": changelogContent,
	"BRAIN.sum":    brainSumContent,
	"AGENTS.md":    agentsContent,
}

var partialDirs = map[string]struct{}{
	".brain":           {},
	".brain/conflicts": {},
	".brain/hooks":     {},
	"sources":          {},
	"wiki":             {},
}

var partialFiles = map[string]struct{}{
	markerPath:                {},
	".brain/hooks/commit-msg": {},
	".brain/hooks/pre-commit": {},
	".brain/hooks/pre-push":   {},
	"AGENTS.md":               {},
	"BRAIN.sum":               {},
	"CHANGELOG.md":            {},
	"INDEX.md":                {},
}

func ensureScaffold(repo string) error {
	for _, rel := range scaffoldDirs {
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(rel)), 0o755); err != nil {
			return err
		}
	}
	for rel, content := range scaffoldFiles {
		if err := writeExpectedFile(filepath.Join(repo, filepath.FromSlash(rel)), content); err != nil {
			return err
		}
	}
	return nil
}

func writeExpectedFile(path, content string) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) != content {
			return fmt.Errorf("refusing to overwrite existing file %s with different content", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func validateFreshBoilerplate(repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" {
			continue
		}
		if entry.IsDir() {
			return fmt.Errorf("refusing to initialize %s: existing directory %q is not Lumbrera boilerplate", repo, name)
		}
		if !isAllowedBoilerplateFile(name) {
			return fmt.Errorf("refusing to initialize %s: existing file %q is not Lumbrera boilerplate", repo, name)
		}
	}
	return nil
}

func validateEmptyNonGitDirectory(repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	entry := entries[0]
	kind := "file"
	if entry.IsDir() {
		kind = "directory"
	}
	return fmt.Errorf("refusing to initialize %s: existing %s %q is not in an empty directory or clean Git boilerplate repo", repo, kind, entry.Name())
}

func validatePartialScaffold(repo string) error {
	return filepath.WalkDir(repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == repo {
			return nil
		}

		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if _, ok := partialDirs[rel]; ok {
				return nil
			}
			return fmt.Errorf("refusing to resume initialization in %s: existing directory %q is not part of a partial Lumbrera scaffold", repo, rel)
		}
		if _, ok := partialFiles[rel]; ok {
			if rel == markerPath {
				return validateMarkerFile(repo, path)
			}
			return nil
		}
		if !strings.Contains(rel, "/") && isAllowedBoilerplateFile(rel) {
			return nil
		}
		return fmt.Errorf("refusing to resume initialization in %s: existing path %q is not part of a partial Lumbrera scaffold", repo, rel)
	})
}

func validateMarkerFile(repo, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	marker := strings.TrimSpace(string(content))
	if marker != brainVersion {
		return fmt.Errorf("refusing to resume initialization in %s: unsupported Lumbrera marker %q", repo, marker)
	}
	return nil
}

func statusOnlyInitScaffold(status string) bool {
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if line == "" {
			continue
		}
		path := statusPath(line)
		if path == "" || !isInitStatusPath(path) {
			return false
		}
	}
	return true
}

func statusPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	path := line[3:]
	if i := strings.Index(path, " -> "); i >= 0 {
		path = path[i+4:]
	}
	return strings.TrimSuffix(filepath.ToSlash(path), "/")
}

func isInitStatusPath(path string) bool {
	if _, ok := partialFiles[path]; ok {
		return true
	}
	_, ok := partialDirs[path]
	return ok
}

func isAllowedBoilerplateFile(name string) bool {
	if name == ".gitignore" {
		return true
	}
	switch strings.ToUpper(name) {
	case "README", "README.MD", "LICENSE", "LICENSE.MD", "LICENSE.TXT", "COPYING", "COPYING.MD":
		return true
	default:
		return false
	}
}

const indexContent = `# Lumbrera Index

Generated by Lumbrera.

## Sources

No sources yet.

## Wiki

No wiki pages yet.
`

const changelogContent = `# Lumbrera Changelog

Generated from Lumbrera commit history.

No Lumbrera writes yet.
`

const brainSumContent = `lumbrera-sum-v1 sha256
`

const agentsContent = `# Lumbrera Brain Agent Contract

This repository is managed by Lumbrera.

You may:
- read Markdown files directly,
- inspect INDEX.md, CHANGELOG.md, and BRAIN.sum,
- run lumbrera sync --repo <repo> before relying on local state.

You must not:
- create, edit, move, delete, or overwrite files directly,
- edit generated files,
- run Git mutation commands directly for knowledge changes,
- modify files under .brain/.

All mutations must use lumbrera write.

Knowledge rules:
- preserve raw material under sources/,
- write distilled knowledge under wiki/,
- wiki/ pages require source references,
- sources are immutable after creation.

Remote setup is administrative. Humans usually configure the remote. Agents may do so only when explicitly instructed.
`

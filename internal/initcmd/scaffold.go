package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	brainVersion        = "lumbrera-brain-v1"
	markerPath          = ".brain/VERSION"
	agentsPath          = "AGENTS.md"
	claudePath          = "CLAUDE.md"
	claudeSymlinkTarget = agentsPath
	agentsDir           = ".agents"
	claudeDir           = ".claude"
	claudeDirTarget     = agentsDir
)

var scaffoldDirs = []string{
	"sources",
	"sources/design",
	"sources/reference",
	"wiki",
	"wiki/design",
	".brain",
	".agents/skills/lumbrera-ingest",
	".agents/skills/lumbrera-query",
	".agents/skills/lumbrera-health",
}

var scaffoldFiles = map[string]string{
	markerPath:       brainVersion + "\n",
	"INDEX.md":       indexContent,
	"CHANGELOG.md":   changelogContent,
	"BRAIN.sum":      brainSumContent,
	"tags.md":        tagsContent,
	".brain/ops.log": "",
	agentsPath:       agentsContent,
	".agents/skills/lumbrera-ingest/SKILL.md": ingestSkillContent,
	".agents/skills/lumbrera-query/SKILL.md":  querySkillContent,
	".agents/skills/lumbrera-health/SKILL.md": healthSkillContent,
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
	if err := ensureGitignore(filepath.Join(repo, ".gitignore")); err != nil {
		return err
	}
	if err := ensureSymlink(filepath.Join(repo, claudePath), claudeSymlinkTarget); err != nil {
		return err
	}
	return ensureSymlink(filepath.Join(repo, claudeDir), claudeDirTarget)
}

func ensureGitignore(path string) error {
	const searchIndexIgnore = ".brain/search.sqlite*"
	info, statErr := os.Lstat(path)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to update symlinked .gitignore %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to update non-regular .gitignore %s", path)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return writeExpectedFile(path, "# Lumbrera derived cache\n"+searchIndexIgnore+"\n")
	}
	if hasGitignoreLine(string(content), searchIndexIgnore) {
		return nil
	}

	updated := string(content)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if updated != "" {
		updated += "\n"
	}
	updated += "# Lumbrera derived cache\n" + searchIndexIgnore + "\n"
	return os.WriteFile(path, []byte(updated), 0o644)
}

func hasGitignoreLine(content, want string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
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

func ensureSymlink(path, target string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to overwrite existing file %s; expected symlink to %s", path, target)
		}
		current, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if current != target {
			return fmt.Errorf("refusing to replace existing symlink %s -> %s; expected %s", path, current, target)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Symlink(target, path)
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

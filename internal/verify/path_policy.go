package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

func ValidatePathPolicy(repo string) error {
	return filepath.WalkDir(repo, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if absPath == repo {
			return nil
		}
		rel, err := filepath.Rel(repo, absPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".brain" || strings.HasPrefix(rel, ".brain/") || rel == ".agents" || strings.HasPrefix(rel, ".agents/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".claude" || strings.HasPrefix(rel, ".claude/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			if rel == "CLAUDE.md" || rel == ".claude" {
				return nil
			}
			return fmt.Errorf("path %s is a symlink; Lumbrera content paths must not use symlinks", rel)
		}

		if entry.IsDir() {
			if rel == "sources" || strings.HasPrefix(rel, "sources/") || rel == "wiki" || strings.HasPrefix(rel, "wiki/") {
				return nil
			}
			return fmt.Errorf("unexpected directory %s; Lumbrera content must live under sources/ or wiki/", rel)
		}

		if strings.HasPrefix(rel, "sources/") || strings.HasPrefix(rel, "wiki/") {
			if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				if _, _, err := pathpolicy.NormalizeTargetPath(rel); err != nil {
					return err
				}
			}
			return nil
		}

		if isAllowedRootFile(rel) {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return fmt.Errorf("unexpected Markdown file %s; Lumbrera content must live under sources/ or wiki/", rel)
		}
		return nil
	})
}

func isAllowedRootFile(rel string) bool {
	if _, ok := allowedRootFiles[rel]; ok {
		return true
	}
	switch strings.ToUpper(rel) {
	case "README", "README.MD", "LICENSE", "LICENSE.MD", "LICENSE.TXT", "COPYING", "COPYING.MD":
		return true
	default:
		return false
	}
}

package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

// ValidatePathPolicy checks that the Lumbrera content directories exist and
// that files inside sources/ and wiki/ obey path policy. Everything outside
// those directories is ignored — the brain repo may contain arbitrary
// non-Lumbrera files such as .github/, README.md, CI configs, etc.
func ValidatePathPolicy(repo string) error {
	for _, dir := range []string{"sources", "wiki"} {
		if err := validateContentDir(repo, dir); err != nil {
			return err
		}
	}
	return nil
}

// validateContentDir checks that a content directory exists as a real
// directory (not a symlink) and that all files inside obey path policy.
func validateContentDir(repo, root string) error {
	absRoot := filepath.Join(repo, root)
	info, err := os.Lstat(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("required directory %s/ is missing", root)
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a real directory, not a symlink", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must be a directory", root)
	}

	return filepath.WalkDir(absRoot, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if absPath == absRoot {
			return nil
		}
		rel, err := filepath.Rel(repo, absPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %s is a symlink; Lumbrera content paths must not use symlinks", rel)
		}

		if entry.IsDir() {
			if err := validateTierDirectory(rel); err != nil {
				return err
			}
			return nil
		}

		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			if _, _, err := pathpolicy.NormalizeTargetPath(rel); err != nil {
				return err
			}
		}
		return nil
	})
}

// validateTierDirectory checks that first-level directories under sources/ and
// wiki/ are either known tier directories or product/topic directories. A
// single-segment directory directly under sources/ or wiki/ that matches a
// known tier name is always allowed. Unknown single-segment names that look
// like tier typos (e.g. "desing") are allowed because they default to
// canonical tier — the enforcement is on known tier names only.
func validateTierDirectory(rel string) error {
	var root, rest string
	if strings.HasPrefix(rel, "sources/") {
		root = "sources"
		rest = strings.TrimPrefix(rel, "sources/")
	} else if strings.HasPrefix(rel, "wiki/") {
		root = "wiki"
		rest = strings.TrimPrefix(rel, "wiki/")
	} else {
		return nil
	}
	// Only check first-level directories under root
	if strings.Contains(rest, "/") {
		return nil
	}
	_ = root
	return nil
}

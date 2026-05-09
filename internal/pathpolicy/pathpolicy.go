package pathpolicy

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

func NormalizeTargetPath(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("target path is required")
	}
	raw = filepath.ToSlash(raw)
	if filepath.IsAbs(raw) || path.IsAbs(raw) {
		return "", "", fmt.Errorf("absolute target paths are not allowed")
	}
	if HasParentSegment(raw) {
		return "", "", fmt.Errorf("target path %q must not contain ..", raw)
	}
	if strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	clean := path.Clean(raw)
	if clean == "." || clean == "" {
		return "", "", fmt.Errorf("invalid target path %q", raw)
	}
	if strings.Contains(clean, "\\") {
		return "", "", fmt.Errorf("target path %q must use repo-relative POSIX separators", raw)
	}
	root, ok := brain.RootForPath(clean)
	if !ok {
		return "", "", fmt.Errorf("target path %q must be under %s", raw, brain.ContentDirList())
	}
	isMd := strings.HasSuffix(strings.ToLower(clean), ".md")
	if root.Markdown && !isMd {
		return "", "", fmt.Errorf("target path %q must be a Markdown file", raw)
	}
	if !root.Markdown && isMd {
		return "", "", fmt.Errorf("target path %q: Markdown files are not allowed under %s/", raw, root.Dir)
	}
	return clean, root.Kind, nil
}

func EnsureSafeFilesystemTarget(repo, target string) error {
	repoResolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return err
	}
	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	if info, err := os.Lstat(absTarget); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink %s", target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target path %s is not a regular file", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ancestor := filepath.Dir(absTarget)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if errors.Is(err, os.ErrNotExist) {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				return fmt.Errorf("could not find existing parent for %s", target)
			}
			ancestor = parent
		} else {
			return err
		}
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return err
	}
	if !PathInside(repoResolved, resolvedAncestor) {
		return fmt.Errorf("target path %s resolves outside repo", target)
	}
	return nil
}

func FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func PathInside(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func HasParentSegment(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

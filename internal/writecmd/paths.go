package writecmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

func normalizeTargetPath(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("target path is required")
	}
	raw = filepath.ToSlash(raw)
	if filepath.IsAbs(raw) || path.IsAbs(raw) {
		return "", "", fmt.Errorf("absolute target paths are not allowed")
	}
	if hasParentSegment(raw) {
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
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return "", "", fmt.Errorf("target path %q must be a Markdown file", raw)
	}
	if strings.HasPrefix(clean, "sources/") && clean != "sources" {
		return clean, "source", nil
	}
	if strings.HasPrefix(clean, "wiki/") && clean != "wiki" {
		return clean, "wiki", nil
	}
	return "", "", fmt.Errorf("target path %q must be under sources/ or wiki/", raw)
}

func ensureSafeFilesystemTarget(repo, target string) error {
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
	if !pathInside(repoResolved, resolvedAncestor) {
		return fmt.Errorf("target path %s resolves outside repo", target)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func pathInside(root, candidate string) bool {
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

func hasParentSegment(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func mergePaths(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func referencePaths(refs []md.Reference) []string {
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	return mergePaths(paths)
}

func filterWikiLinks(links []string) []string {
	var out []string
	for _, link := range links {
		if strings.HasPrefix(link, "wiki/") {
			out = append(out, link)
		}
	}
	return mergePaths(out)
}

func sameStrings(a, b []string) bool {
	a = mergePaths(a)
	b = mergePaths(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

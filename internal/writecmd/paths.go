package writecmd

import (
	"sort"
	"strings"

	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

func normalizeTargetPath(raw string) (string, string, error) {
	return pathpolicy.NormalizeTargetPath(raw)
}

func ensureSafeFilesystemTarget(repo, target string) error {
	return pathpolicy.EnsureSafeFilesystemTarget(repo, target)
}

func fileExists(path string) (bool, error) {
	return pathpolicy.FileExists(path)
}

func pathInside(root, candidate string) bool {
	return pathpolicy.PathInside(root, candidate)
}

func hasParentSegment(p string) bool {
	return pathpolicy.HasParentSegment(p)
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

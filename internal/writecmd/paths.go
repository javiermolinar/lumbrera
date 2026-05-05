package writecmd

import (
	"strings"

	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/textutil"
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

func mergePaths(groups ...[]string) []string {
	return textutil.MergeStrings(groups...)
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
	return textutil.SameStringSet(a, b)
}

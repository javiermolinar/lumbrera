package markdown

import (
	"sort"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/textutil"
)

func sortedUnique(values []string) []string {
	return textutil.UniqueSorted(values)
}

func sortedUniqueReferences(values []Reference) []Reference {
	seen := make(map[string]struct{}, len(values))
	out := make([]Reference, 0, len(values))
	for _, value := range values {
		value.Path = strings.TrimSpace(value.Path)
		value.Anchor = strings.TrimSpace(value.Anchor)
		if value.Path == "" {
			continue
		}
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Anchor < out[j].Anchor
		}
		return out[i].Path < out[j].Path
	})
	return out
}

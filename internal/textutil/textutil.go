package textutil

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// UniqueSorted returns trimmed, non-empty, deduplicated strings in lexical order.
func UniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
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
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

// MergeStrings merges multiple string groups with the same normalization as
// UniqueSorted.
func MergeStrings(groups ...[]string) []string {
	var values []string
	for _, group := range groups {
		values = append(values, group...)
	}
	return UniqueSorted(values)
}

// SameStringSet reports whether two string lists contain the same normalized
// values, ignoring order and duplicates.
func SameStringSet(left, right []string) bool {
	left = UniqueSorted(left)
	right = UniqueSorted(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TitleForPath(relPath string) string {
	base := filepath.Base(relPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return relPath
	}
	return TitleWords(base)
}

func TitleWords(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

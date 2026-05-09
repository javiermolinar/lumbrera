package markdown

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/textutil"
)

func (r Reference) String() string {
	if r.Anchor == "" {
		return r.Path
	}
	return r.Path + "#" + r.Anchor
}

func NormalizeLink(fromPath, destination string) (string, error) {
	ref, ok, err := NormalizeReference(fromPath, destination)
	if err != nil || !ok || isFragmentOnly(destination) {
		return "", err
	}
	return ref.Path, nil
}

func NormalizeReference(fromPath, destination string) (Reference, bool, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" || isExternal(destination) {
		return Reference{}, false, nil
	}

	linkPath, fragment, hasFragment := splitDestination(destination)
	if linkPath == "" {
		if !hasFragment {
			return Reference{}, false, nil
		}
		anchor := NormalizeAnchor(fragment)
		if anchor == "" {
			return Reference{}, false, nil
		}
		return Reference{Path: path.Clean(fromPath), Anchor: anchor}, true, nil
	}

	normalized, err := normalizeLinkPath(fromPath, linkPath)
	if err != nil {
		return Reference{}, false, err
	}
	ref := Reference{Path: normalized}
	if hasFragment {
		ref.Anchor = NormalizeAnchor(fragment)
	}
	return ref, true, nil
}

func NormalizeAnchor(anchor string) string {
	anchor = strings.TrimSpace(strings.TrimPrefix(anchor, "#"))
	if anchor == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(anchor); err == nil {
		anchor = decoded
	}
	return strings.TrimSpace(anchor)
}

func AnchorForHeading(text string) string {
	text = strings.TrimSpace(text)
	var b strings.Builder
	lastDash := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '_':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "section"
	}
	return slug
}

func normalizeLinkPath(fromPath, destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "", nil
	}
	destination = path.Clean(strings.ReplaceAll(destination, "\\", "/"))
	if path.IsAbs(destination) {
		return "", fmt.Errorf("absolute Markdown link %q is not allowed", destination)
	}

	var normalized string
	if brain.IsContentPath(destination) {
		normalized = path.Clean(destination)
	} else {
		normalized = path.Clean(path.Join(path.Dir(fromPath), destination))
	}
	if normalized == "." || normalized == "" {
		return "", nil
	}
	if pathpolicy.HasParentSegment(normalized) {
		return "", fmt.Errorf("Markdown link %q resolves outside the repo", destination)
	}
	if !brain.IsContentPath(normalized) {
		return "", fmt.Errorf("Markdown link %q resolves to %q outside %s", destination, normalized, brain.ContentDirList())
	}
	return normalized, nil
}

func RelativeLink(fromPath, toPath string) string {
	fromDir := path.Dir(fromPath)
	rel, err := filepath.Rel(filepath.FromSlash(fromDir), filepath.FromSlash(toPath))
	if err != nil {
		return toPath
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

func splitDestination(destination string) (string, string, bool) {
	linkPath := destination
	fragment := ""
	hasFragment := false
	if i := strings.Index(linkPath, "#"); i >= 0 {
		hasFragment = true
		fragment = linkPath[i+1:]
		linkPath = linkPath[:i]
	}
	if i := strings.Index(linkPath, "?"); i >= 0 {
		linkPath = linkPath[:i]
	}
	return strings.TrimSpace(linkPath), strings.TrimSpace(fragment), hasFragment
}

func isFragmentOnly(destination string) bool {
	destination = strings.TrimSpace(destination)
	return strings.HasPrefix(destination, "#")
}

func isSelfAnchorReference(fromPath string, ref Reference) bool {
	return ref.Anchor != "" && ref.Path == path.Clean(fromPath)
}

func uniqueHeadingAnchor(text string, counts map[string]int) string {
	base := AnchorForHeading(text)
	count := counts[base]
	counts[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count)
}

func isExternal(destination string) bool {
	lower := strings.ToLower(destination)
	return strings.Contains(lower, "://") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") || strings.HasPrefix(lower, "urn:")
}

func linkLabel(repoPath string) string {
	base := path.Base(repoPath)
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return repoPath
	}
	return titleWords(base)
}

func titleWords(value string) string {
	return textutil.TitleWords(value)
}

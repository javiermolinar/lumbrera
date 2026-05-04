package markdown

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type Analysis struct {
	FirstH1 string
	Sources []string
	Links   []string
}

func Analyze(repoRelativePath, body string) (Analysis, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var analysis Analysis
	inSources := false
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *ast.Heading:
			text := strings.TrimSpace(string(n.Text(source)))
			if n.Level == 1 && analysis.FirstH1 == "" {
				analysis.FirstH1 = text
			}
			if n.Level <= 2 {
				inSources = n.Level == 2 && strings.EqualFold(text, "Sources")
			}
		case *ast.Link:
			destination := string(n.Destination)
			if isIgnorableLink(destination) {
				return ast.WalkContinue, nil
			}
			normalized, err := NormalizeLink(repoRelativePath, destination)
			if err != nil {
				return ast.WalkStop, err
			}
			if inSources {
				analysis.Sources = append(analysis.Sources, normalized)
			} else {
				analysis.Links = append(analysis.Links, normalized)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Analysis{}, err
	}
	analysis.Sources = sortedUnique(analysis.Sources)
	analysis.Links = sortedUnique(analysis.Links)
	return analysis, nil
}

func NormalizeLink(fromPath, destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" || strings.HasPrefix(destination, "#") || isExternal(destination) {
		return "", nil
	}
	if i := strings.IndexAny(destination, "#?"); i >= 0 {
		destination = destination[:i]
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "", nil
	}
	destination = path.Clean(strings.ReplaceAll(destination, "\\", "/"))
	if path.IsAbs(destination) {
		return "", fmt.Errorf("absolute Markdown link %q is not allowed", destination)
	}

	var normalized string
	if strings.HasPrefix(destination, "sources/") || strings.HasPrefix(destination, "wiki/") {
		normalized = path.Clean(destination)
	} else {
		normalized = path.Clean(path.Join(path.Dir(fromPath), destination))
	}
	if normalized == "." || normalized == "" {
		return "", nil
	}
	if hasParentSegment(normalized) {
		return "", fmt.Errorf("Markdown link %q resolves outside the repo", destination)
	}
	if !strings.HasPrefix(normalized, "sources/") && !strings.HasPrefix(normalized, "wiki/") {
		return "", fmt.Errorf("Markdown link %q resolves to %q outside sources/ or wiki/", destination, normalized)
	}
	return normalized, nil
}

func RemoveSourcesSection(body string) string {
	lines := strings.Split(body, "\n")
	start := -1
	end := len(lines)
	inFence := false
	for i, line := range lines {
		if togglesFence(line) {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		level, text, ok := heading(line)
		if !ok {
			continue
		}
		if start < 0 {
			if level == 2 && strings.EqualFold(text, "Sources") {
				start = i
			}
			continue
		}
		if level <= 2 {
			end = i
			break
		}
	}
	if start < 0 {
		return strings.TrimRight(body, "\n")
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, lines[end:]...)
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func AppendSourcesSection(body, docPath string, sources []string) string {
	body = strings.TrimRight(RemoveSourcesSection(body), "\n")
	var b strings.Builder
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	b.WriteString("## Sources\n\n")
	for _, source := range sortedUnique(sources) {
		link := RelativeLink(docPath, source)
		fmt.Fprintf(&b, "- [%s](%s)\n", linkLabel(source), link)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func AppendToSection(body, section, snippet string) string {
	section = strings.TrimSpace(section)
	snippet = strings.Trim(snippet, "\n")
	if section == "" || snippet == "" {
		return strings.TrimRight(body, "\n") + "\n"
	}
	body = strings.TrimRight(body, "\n")
	lines := strings.Split(body, "\n")
	inFence := false
	start := -1
	end := len(lines)
	sectionLevel := 0
	for i, line := range lines {
		if togglesFence(line) {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		level, text, ok := heading(line)
		if !ok {
			continue
		}
		if start < 0 {
			if text == section {
				start = i
				sectionLevel = level
			}
			continue
		}
		if level <= sectionLevel {
			end = i
			break
		}
	}

	if start < 0 {
		var b strings.Builder
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n\n")
		}
		b.WriteString("## ")
		b.WriteString(section)
		b.WriteString("\n\n")
		b.WriteString(snippet)
		b.WriteByte('\n')
		return b.String()
	}

	out := append([]string{}, lines[:end]...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, strings.Split(snippet, "\n")...)
	out = append(out, "")
	out = append(out, lines[end:]...)
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
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

func heading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for _, r := range trimmed {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 || level > 6 || len(trimmed) <= level || !unicode.IsSpace(rune(trimmed[level])) {
		return 0, "", false
	}
	text := strings.TrimSpace(trimmed[level:])
	text = strings.TrimRight(text, "#")
	text = strings.TrimSpace(text)
	return level, text, true
}

func togglesFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func isIgnorableLink(destination string) bool {
	destination = strings.TrimSpace(destination)
	return destination == "" || strings.HasPrefix(destination, "#") || isExternal(destination)
}

func isExternal(destination string) bool {
	lower := strings.ToLower(destination)
	return strings.Contains(lower, "://") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") || strings.HasPrefix(lower, "urn:")
}

func hasParentSegment(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func sortedUnique(values []string) []string {
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
	return out
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

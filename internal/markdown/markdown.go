package markdown

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type Reference struct {
	Path   string
	Anchor string
}

func (r Reference) String() string {
	if r.Anchor == "" {
		return r.Path
	}
	return r.Path + "#" + r.Anchor
}

type Heading struct {
	Level  int
	Text   string
	Anchor string
}

type Analysis struct {
	FirstH1          string
	Sources          []string
	Links            []string
	Headings         []Heading
	Anchors          []string
	LinkReferences   []Reference
	SourceReferences []Reference
	SourceCitations  []Reference
}

type AnalyzeOptions struct {
	SourceCitations bool
	IgnoreLinks     bool
}

var sourceCitationPattern = regexp.MustCompile(`(?i)\[source:\s*([^\]\r\n]+?)\]`)

func Analyze(repoRelativePath, body string) (Analysis, error) {
	return AnalyzeWithOptions(repoRelativePath, body, AnalyzeOptions{SourceCitations: true})
}

func AnalyzeWithOptions(repoRelativePath, body string, opts AnalyzeOptions) (Analysis, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var analysis Analysis
	inSources := false
	anchorCounts := map[string]int{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := node.(type) {
		case *ast.Heading:
			if !entering {
				return ast.WalkContinue, nil
			}
			text := strings.TrimSpace(string(n.Text(source)))
			anchor := uniqueHeadingAnchor(text, anchorCounts)
			analysis.Headings = append(analysis.Headings, Heading{Level: n.Level, Text: text, Anchor: anchor})
			analysis.Anchors = append(analysis.Anchors, anchor)
			if n.Level == 1 && analysis.FirstH1 == "" {
				analysis.FirstH1 = text
			}
			if n.Level <= 2 {
				inSources = n.Level == 2 && strings.EqualFold(text, "Sources")
			}
		case *ast.Link:
			if !entering {
				return ast.WalkContinue, nil
			}
			if opts.IgnoreLinks {
				return ast.WalkSkipChildren, nil
			}
			ref, ok, err := NormalizeReference(repoRelativePath, string(n.Destination))
			if err != nil {
				return ast.WalkStop, err
			}
			if ok {
				if inSources {
					analysis.SourceReferences = append(analysis.SourceReferences, ref)
					analysis.Sources = append(analysis.Sources, ref.Path)
				} else {
					analysis.LinkReferences = append(analysis.LinkReferences, ref)
					if !isSelfAnchorReference(repoRelativePath, ref) {
						analysis.Links = append(analysis.Links, ref.Path)
					}
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.Paragraph:
			if !entering || inSources || !opts.SourceCitations {
				return ast.WalkContinue, nil
			}
			refs, err := sourceCitationReferences(repoRelativePath, paragraphCitationText(n, source))
			if err != nil {
				return ast.WalkStop, err
			}
			analysis.SourceCitations = append(analysis.SourceCitations, refs...)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Analysis{}, err
	}
	analysis.Sources = sortedUnique(analysis.Sources)
	analysis.Links = sortedUnique(analysis.Links)
	analysis.Anchors = sortedUnique(analysis.Anchors)
	analysis.LinkReferences = sortedUniqueReferences(analysis.LinkReferences)
	analysis.SourceReferences = sortedUniqueReferences(analysis.SourceReferences)
	analysis.SourceCitations = sortedUniqueReferences(analysis.SourceCitations)
	return analysis, nil
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

func paragraphCitationText(paragraph ast.Node, source []byte) string {
	var b strings.Builder
	for child := paragraph.FirstChild(); child != nil; child = child.NextSibling() {
		appendCitationText(&b, child, source)
	}
	return b.String()
}

func appendCitationText(b *strings.Builder, node ast.Node, source []byte) {
	switch n := node.(type) {
	case *ast.CodeSpan, *ast.Link:
		b.WriteByte('\n')
		return
	case *ast.Text:
		b.Write(n.Text(source))
		return
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		appendCitationText(b, child, source)
	}
}

func sourceCitationReferences(fromPath, text string) ([]Reference, error) {
	matches := sourceCitationPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	refs := make([]Reference, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 || citationIsLinkLabel(text, match[1]) {
			continue
		}
		destination := strings.TrimSpace(text[match[2]:match[3]])
		if !looksLikeSourceCitationDestination(destination) {
			continue
		}
		ref, ok, err := NormalizeReference(fromPath, destination)
		if err != nil {
			return nil, fmt.Errorf("invalid source citation %q: %w", destination, err)
		}
		if !ok {
			return nil, fmt.Errorf("source citation %q must reference a local Markdown source", destination)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func citationIsLinkLabel(text string, citationEnd int) bool {
	return citationEnd < len(text) && text[citationEnd] == '('
}

func looksLikeSourceCitationDestination(destination string) bool {
	destination = strings.TrimSpace(destination)
	return strings.HasPrefix(destination, "#") ||
		strings.HasPrefix(destination, "./") ||
		strings.HasPrefix(destination, "../") ||
		strings.HasPrefix(destination, "sources/") ||
		strings.HasPrefix(destination, "wiki/")
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

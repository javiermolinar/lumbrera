package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
)

var sourceCitationPattern = regexp.MustCompile(`(?i)\[source:\s*([^\]\r\n]+?)\]`)

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

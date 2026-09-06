package markdown

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type sourceEdit struct {
	start       int
	end         int
	replacement string
}

type linkRewriteMode uint8

const (
	linkRewriteUnwrap linkRewriteMode = iota
	linkRewriteRemove
)

// RemoveEvidenceCitations removes source-citation tokens that resolve to
// evidencePath while preserving code spans, code blocks, links, and all other
// source bytes.
func RemoveEvidenceCitations(body, fromPath, evidencePath string) (string, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
	var edits []sourceEdit

	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || !isCitationContainer(node) {
			return ast.WalkContinue, nil
		}
		citationText, offsets := citationTextWithOffsets(node, source)
		for _, match := range sourceCitationPattern.FindAllStringSubmatchIndex(citationText, -1) {
			if len(match) < 4 {
				continue
			}
			destination := strings.TrimSpace(citationText[match[2]:match[3]])
			if !looksLikeSourceCitationDestination(destination) {
				continue
			}
			ref, ok, err := NormalizeReference(fromPath, destination)
			if err != nil {
				return ast.WalkStop, fmt.Errorf("invalid source citation %q: %w", destination, err)
			}
			if !ok || ref.Path != evidencePath {
				continue
			}
			start, end, ok := contiguousSourceRange(offsets, match[0], match[1])
			if !ok {
				return ast.WalkStop, fmt.Errorf("source citation %q does not map to one source range", destination)
			}
			edits = append(edits, sourceEdit{start: start, end: end})
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return applySourceEdits(source, edits)
}

// UnwrapLinksToPath replaces Markdown links and images that resolve to
// targetPath with their original label or alt text. Reference definitions for
// the target are removed after their uses are unwrapped.
func UnwrapLinksToPath(body, fromPath, targetPath string) (string, error) {
	return rewriteLinksToPath(body, fromPath, targetPath, linkRewriteUnwrap)
}

// RemoveLinksAndImagesToPath removes Markdown links, images, and reference
// definitions that resolve to targetPath while preserving all unrelated bytes.
func RemoveLinksAndImagesToPath(body, fromPath, targetPath string) (string, error) {
	return rewriteLinksToPath(body, fromPath, targetPath, linkRewriteRemove)
}

func rewriteLinksToPath(body, fromPath, targetPath string, mode linkRewriteMode) (string, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
	var edits []sourceEdit

	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Link:
			matches, err := destinationMatchesPath(fromPath, n.Destination, targetPath)
			if err != nil {
				return ast.WalkStop, err
			}
			if !matches {
				return ast.WalkContinue, nil
			}
			start, end, labelStart, labelEnd, err := linkSourceSpan(source, n.Pos(), n.Reference)
			if err != nil {
				return ast.WalkStop, err
			}
			replacement := ""
			if mode == linkRewriteUnwrap {
				replacement = string(source[labelStart:labelEnd])
			}
			edits = append(edits, sourceEdit{start: start, end: end, replacement: replacement})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			matches, err := destinationMatchesPath(fromPath, n.Destination, targetPath)
			if err != nil {
				return ast.WalkStop, err
			}
			if !matches {
				return ast.WalkContinue, nil
			}
			start, end, labelStart, labelEnd, err := linkSourceSpan(source, n.Pos(), n.Reference)
			if err != nil {
				return ast.WalkStop, err
			}
			replacement := ""
			if mode == linkRewriteUnwrap {
				replacement = string(source[labelStart:labelEnd])
			}
			edits = append(edits, sourceEdit{start: start, end: end, replacement: replacement})
			return ast.WalkSkipChildren, nil
		case *ast.LinkReferenceDefinition:
			matches, err := destinationMatchesPath(fromPath, n.Destination, targetPath)
			if err != nil {
				return ast.WalkStop, err
			}
			if !matches {
				return ast.WalkContinue, nil
			}
			start, end, err := referenceDefinitionSourceSpan(source, n)
			if err != nil {
				return ast.WalkStop, err
			}
			edits = append(edits, sourceEdit{start: start, end: end})
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return applySourceEdits(source, edits)
}

func destinationMatchesPath(fromPath string, destination []byte, targetPath string) (bool, error) {
	ref, ok, err := NormalizeReference(fromPath, string(destination))
	if err != nil {
		return false, err
	}
	return ok && ref.Path == targetPath, nil
}

func isCitationContainer(node ast.Node) bool {
	switch node.(type) {
	case *ast.Paragraph, *ast.Heading, *ast.TextBlock:
		return true
	default:
		return false
	}
}

func citationTextWithOffsets(container ast.Node, source []byte) (string, []int) {
	var b strings.Builder
	var offsets []int
	var appendNode func(ast.Node)
	appendSentinel := func() {
		b.WriteByte('\n')
		offsets = append(offsets, -1)
	}
	appendNode = func(node ast.Node) {
		switch n := node.(type) {
		case *ast.CodeSpan, *ast.Link, *ast.Image, *ast.RawHTML:
			appendSentinel()
			return
		case *ast.Text:
			if n.Segment.Start < 0 || n.Segment.Stop > len(source) || n.Segment.Start > n.Segment.Stop {
				appendSentinel()
				return
			}
			value := source[n.Segment.Start:n.Segment.Stop]
			b.Write(value)
			for i := range value {
				offsets = append(offsets, n.Segment.Start+i)
			}
			return
		}
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			appendNode(child)
		}
	}
	for child := container.FirstChild(); child != nil; child = child.NextSibling() {
		appendNode(child)
	}
	return b.String(), offsets
}

func contiguousSourceRange(offsets []int, start, end int) (int, int, bool) {
	if start < 0 || end <= start || end > len(offsets) || offsets[start] < 0 {
		return 0, 0, false
	}
	for i := start + 1; i < end; i++ {
		if offsets[i] != offsets[i-1]+1 {
			return 0, 0, false
		}
	}
	return offsets[start], offsets[end-1] + 1, true
}

func linkSourceSpan(source []byte, start int, reference *ast.ReferenceLink) (int, int, int, int, error) {
	if start < 0 || start >= len(source) {
		return 0, 0, 0, 0, fmt.Errorf("Markdown link has invalid source position %d", start)
	}
	labelOpen := start
	if source[labelOpen] == '!' {
		labelOpen++
	}
	if labelOpen >= len(source) || source[labelOpen] != '[' {
		return 0, 0, 0, 0, fmt.Errorf("Markdown link at byte %d has no opening label", start)
	}
	labelClose, err := matchingDelimiter(source, labelOpen, '[', ']')
	if err != nil {
		return 0, 0, 0, 0, err
	}
	end := labelClose + 1
	if reference == nil {
		if end >= len(source) || source[end] != '(' {
			return 0, 0, 0, 0, fmt.Errorf("inline Markdown link at byte %d has no destination", start)
		}
		close, err := matchingLinkParenthesis(source, end)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		end = close + 1
	} else {
		switch reference.Type {
		case ast.ReferenceLinkFull, ast.ReferenceLinkCollapsed:
			if end >= len(source) || source[end] != '[' {
				return 0, 0, 0, 0, fmt.Errorf("reference Markdown link at byte %d has no reference label", start)
			}
			close, err := matchingDelimiter(source, end, '[', ']')
			if err != nil {
				return 0, 0, 0, 0, err
			}
			end = close + 1
		case ast.ReferenceLinkShortcut:
		default:
			return 0, 0, 0, 0, fmt.Errorf("Markdown link at byte %d has unsupported reference type %d", start, reference.Type)
		}
	}
	return start, end, labelOpen + 1, labelClose, nil
}

func matchingDelimiter(source []byte, openIndex int, opener, closer byte) (int, error) {
	depth := 0
	for i := openIndex; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == '`' {
			if codeEnd, ok := codeSpanEnd(source, i); ok {
				i = codeEnd
				continue
			}
		}
		switch source[i] {
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("Markdown delimiter %q at byte %d is not closed", opener, openIndex)
}

func codeSpanEnd(source []byte, openIndex int) (int, bool) {
	ticks := 1
	for openIndex+ticks < len(source) && source[openIndex+ticks] == '`' {
		ticks++
	}
	for i := openIndex + ticks; i < len(source); {
		if source[i] != '`' {
			i++
			continue
		}
		run := 1
		for i+run < len(source) && source[i+run] == '`' {
			run++
		}
		if run == ticks {
			return i + run - 1, true
		}
		i += run
	}
	return 0, false
}

func matchingLinkParenthesis(source []byte, openIndex int) (int, error) {
	depth := 0
	var quote byte
	inAngleDestination := false
	for i := openIndex; i < len(source); i++ {
		current := source[i]
		if current == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if inAngleDestination {
			if current == '>' {
				inAngleDestination = false
			}
			continue
		}
		if current == '<' {
			inAngleDestination = true
			continue
		}
		if (current == '\'' || current == '"') && i > openIndex && isMarkdownSpace(source[i-1]) {
			quote = current
			continue
		}
		switch current {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("Markdown link destination at byte %d is not closed", openIndex)
}

func isMarkdownSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func referenceDefinitionSourceSpan(source []byte, definition *ast.LinkReferenceDefinition) (int, int, error) {
	lines := definition.Lines()
	if lines.Len() == 0 {
		return 0, 0, fmt.Errorf("Markdown reference definition has no source lines")
	}
	start := lines.At(0).Start
	end := lines.At(lines.Len() - 1).Stop
	if start < 0 || end < start || end > len(source) {
		return 0, 0, fmt.Errorf("Markdown reference definition has invalid source range %d:%d", start, end)
	}
	if end < len(source) && source[end] == '\r' {
		end++
	}
	if end < len(source) && source[end] == '\n' {
		end++
	}
	return start, end, nil
}

func applySourceEdits(source []byte, edits []sourceEdit) (string, error) {
	if len(edits) == 0 {
		return string(source), nil
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		return edits[i].end < edits[j].end
	})

	var b strings.Builder
	cursor := 0
	for i, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(source) {
			return "", fmt.Errorf("invalid Markdown source edit %d:%d", edit.start, edit.end)
		}
		if edit.start < cursor {
			previous := edits[i-1]
			if edit.start == previous.start && edit.end == previous.end && edit.replacement == previous.replacement {
				continue
			}
			return "", fmt.Errorf("overlapping Markdown source edits %d:%d and %d:%d", previous.start, previous.end, edit.start, edit.end)
		}
		b.Write(source[cursor:edit.start])
		b.WriteString(edit.replacement)
		cursor = edit.end
	}
	b.Write(source[cursor:])
	return b.String(), nil
}

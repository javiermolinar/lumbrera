package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Section is a deterministic Markdown section split by parser-recognized
// headings. Body contains the Markdown content under the heading and excludes
// the heading line itself. Documents without headings produce one body-only
// section.
type Section struct {
	Ordinal int
	Heading string
	Anchor  string
	Level   int
	Body    string
}

type sectionHeading struct {
	start  int
	end    int
	level  int
	text   string
	anchor string
}

func SplitSections(body string) ([]Section, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var headings []sectionHeading
	anchorCounts := map[string]int{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}

		lines := heading.Lines()
		if lines.Len() == 0 {
			return ast.WalkContinue, nil
		}
		headingText := strings.TrimSpace(string(heading.Text(source)))
		blockStart := lineStart(source, lines.At(0).Start)
		blockEnd := lineEnd(source, lines.At(lines.Len()-1).Stop)
		if !isATXHeadingLine(source[blockStart:blockEnd]) {
			blockEnd = includeSetextUnderline(source, blockEnd)
		}
		headings = append(headings, sectionHeading{
			start:  blockStart,
			end:    blockEnd,
			level:  heading.Level,
			text:   headingText,
			anchor: uniqueHeadingAnchor(headingText, anchorCounts),
		})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	if len(headings) == 0 {
		return []Section{{Ordinal: 1, Body: trimSectionBody(body)}}, nil
	}

	sections := make([]Section, 0, len(headings)+1)
	if intro := trimSectionBody(string(source[:headings[0].start])); intro != "" {
		sections = append(sections, Section{Body: intro})
	}

	for i, heading := range headings {
		sectionEnd := len(source)
		if i+1 < len(headings) {
			sectionEnd = headings[i+1].start
		}
		bodyStart := heading.end
		if bodyStart > sectionEnd {
			bodyStart = sectionEnd
		}
		sections = append(sections, Section{
			Heading: heading.text,
			Anchor:  heading.anchor,
			Level:   heading.level,
			Body:    trimSectionBody(string(source[bodyStart:sectionEnd])),
		})
	}

	for i := range sections {
		sections[i].Ordinal = i + 1
	}
	return sections, nil
}

func lineStart(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

func lineEnd(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	for offset < len(source) && source[offset] != '\n' {
		offset++
	}
	return offset
}

func isATXHeadingLine(line []byte) bool {
	return strings.HasPrefix(strings.TrimLeft(string(line), " \t"), "#")
}

func includeSetextUnderline(source []byte, headingEnd int) int {
	if headingEnd >= len(source) || source[headingEnd] != '\n' {
		return headingEnd
	}
	underlineStart := headingEnd + 1
	underlineEnd := lineEnd(source, underlineStart)
	if isSetextUnderline(string(source[underlineStart:underlineEnd])) {
		return underlineEnd
	}
	return headingEnd
}

func isSetextUnderline(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	marker := line[0]
	if marker != '=' && marker != '-' {
		return false
	}
	for i := 1; i < len(line); i++ {
		if line[i] != marker {
			return false
		}
	}
	return true
}

func trimSectionBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.Trim(body, "\n")
}

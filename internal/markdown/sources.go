package markdown

import (
	"fmt"
	"strings"
	"unicode"
)

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

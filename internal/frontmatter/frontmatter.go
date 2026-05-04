package frontmatter

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Schema  = "document-v1"
	MaxTags = 5
)

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Document struct {
	Title    string       `yaml:"title"`
	Summary  string       `yaml:"summary,omitempty"`
	Tags     []string     `yaml:"tags,omitempty"`
	Lumbrera LumbreraMeta `yaml:"lumbrera"`
}

type LumbreraMeta struct {
	Schema  string   `yaml:"schema"`
	Kind    string   `yaml:"kind"`
	Sources []string `yaml:"sources"`
	Links   []string `yaml:"links"`
}

func New(kind, title, summary string, tags, sources, links []string) Document {
	return Document{
		Title:   strings.TrimSpace(title),
		Summary: strings.TrimSpace(summary),
		Tags:    sortedUnique(tags),
		Lumbrera: LumbreraMeta{
			Schema:  Schema,
			Kind:    kind,
			Sources: sortedUnique(sources),
			Links:   sortedUnique(links),
		},
	}
}

func Render(doc Document) (string, error) {
	if err := Validate(doc); err != nil {
		return "", err
	}
	doc.Tags = sortedUnique(doc.Tags)
	doc.Lumbrera.Sources = sortedUnique(doc.Lumbrera.Sources)
	doc.Lumbrera.Links = sortedUnique(doc.Lumbrera.Links)

	body, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return "---\n" + string(body) + "---\n\n", nil
}

func Attach(doc Document, markdownBody string) (string, error) {
	fm, err := Render(doc)
	if err != nil {
		return "", err
	}
	return fm + strings.TrimLeft(markdownBody, "\n"), nil
}

func Split(content []byte) (Document, string, bool, error) {
	if !StartsWithFrontmatter(content) {
		return Document{}, string(content), false, nil
	}

	lines := bytes.Split(content, []byte("\n"))
	if len(lines) == 0 || string(bytes.TrimSpace(lines[0])) != "---" {
		return Document{}, string(content), false, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if string(bytes.TrimSpace(lines[i])) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Document{}, "", true, fmt.Errorf("malformed frontmatter: missing closing ---")
	}

	var doc Document
	frontmatterBytes := bytes.Join(lines[1:end], []byte("\n"))
	if err := yaml.Unmarshal(frontmatterBytes, &doc); err != nil {
		return Document{}, "", true, fmt.Errorf("malformed frontmatter: %w", err)
	}
	if err := Validate(doc); err != nil {
		return Document{}, "", true, err
	}

	bodyLines := lines[end+1:]
	if len(bodyLines) > 0 && len(bodyLines[0]) == 0 {
		bodyLines = bodyLines[1:]
	}
	return doc, string(bytes.Join(bodyLines, []byte("\n"))), true, nil
}

func StartsWithFrontmatter(content []byte) bool {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	if bytes.HasPrefix(content, []byte("---\n")) || bytes.HasPrefix(content, []byte("---\r\n")) {
		return true
	}
	return string(bytes.TrimSpace(content)) == "---"
}

func Validate(doc Document) error {
	if strings.TrimSpace(doc.Title) == "" {
		return fmt.Errorf("frontmatter title is required")
	}
	if doc.Lumbrera.Schema != Schema {
		return fmt.Errorf("frontmatter lumbrera.schema must be %q", Schema)
	}
	if doc.Lumbrera.Kind != "source" && doc.Lumbrera.Kind != "wiki" {
		return fmt.Errorf("frontmatter lumbrera.kind must be source or wiki")
	}
	if doc.Lumbrera.Kind == "wiki" {
		summary := strings.TrimSpace(doc.Summary)
		if summary == "" {
			return fmt.Errorf("frontmatter summary is required for wiki documents")
		}
		if strings.ContainsAny(summary, "\r\n") {
			return fmt.Errorf("frontmatter summary must be a single line")
		}
		if err := ValidateTags(doc.Tags); err != nil {
			return err
		}
	}
	return nil
}

func ValidateTags(tags []string) error {
	normalized := sortedUnique(tags)
	if len(normalized) == 0 {
		return fmt.Errorf("frontmatter tags are required for wiki documents")
	}
	if len(normalized) > MaxTags {
		return fmt.Errorf("frontmatter tags exceed maximum of %d", MaxTags)
	}
	for _, tag := range normalized {
		if !tagPattern.MatchString(tag) {
			return fmt.Errorf("frontmatter tag %q must be a lowercase slug using a-z, 0-9, and hyphen", tag)
		}
	}
	return nil
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
	if out == nil {
		return []string{}
	}
	return out
}

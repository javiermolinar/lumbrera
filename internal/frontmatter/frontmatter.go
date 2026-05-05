package frontmatter

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/textutil"
	"gopkg.in/yaml.v3"
)

const (
	Schema  = "document-v1"
	MaxTags = 5
)

var (
	idPattern  = regexp.MustCompile(`^doc_[a-f0-9]{32}$`)
	tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

type Document struct {
	Title    string       `yaml:"title"`
	Summary  string       `yaml:"summary,omitempty"`
	Tags     []string     `yaml:"tags,omitempty"`
	Lumbrera LumbreraMeta `yaml:"lumbrera"`
}

type LumbreraMeta struct {
	ID      string   `yaml:"id"`
	Schema  string   `yaml:"schema"`
	Kind    string   `yaml:"kind"`
	Sources []string `yaml:"sources"`
	Links   []string `yaml:"links"`
}

type SplitOptions struct {
	AllowMissingID bool
}

type ValidateOptions struct {
	AllowMissingID bool
}

func New(kind, title, summary string, tags, sources, links []string) Document {
	return NewWithID(newIDBestEffort(), kind, title, summary, tags, sources, links)
}

func NewWithID(id, kind, title, summary string, tags, sources, links []string) Document {
	return Document{
		Title:   strings.TrimSpace(title),
		Summary: strings.TrimSpace(summary),
		Tags:    sortedUnique(tags),
		Lumbrera: LumbreraMeta{
			ID:      strings.TrimSpace(id),
			Schema:  Schema,
			Kind:    kind,
			Sources: sortedUnique(sources),
			Links:   sortedUnique(links),
		},
	}
}

func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "doc_" + hex.EncodeToString(b[:]), nil
}

func newIDBestEffort() string {
	id, err := NewID()
	if err == nil {
		return id
	}
	seed := fmt.Sprintf("%d:%d", time.Now().UnixNano(), os.Getpid())
	sum := sha256.Sum256([]byte(seed))
	return "doc_" + hex.EncodeToString(sum[:16])
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
	return SplitWithOptions(content, SplitOptions{})
}

func SplitWithOptions(content []byte, opts SplitOptions) (Document, string, bool, error) {
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
	if err := ValidateWithOptions(doc, ValidateOptions{AllowMissingID: opts.AllowMissingID}); err != nil {
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
	return ValidateWithOptions(doc, ValidateOptions{})
}

func ValidateWithOptions(doc Document, opts ValidateOptions) error {
	if strings.TrimSpace(doc.Title) == "" {
		return fmt.Errorf("frontmatter title is required")
	}
	id := strings.TrimSpace(doc.Lumbrera.ID)
	if id == "" && !opts.AllowMissingID {
		return fmt.Errorf("frontmatter lumbrera.id is required")
	}
	if id != "" && !idPattern.MatchString(id) {
		return fmt.Errorf("frontmatter lumbrera.id %q must match doc_<32 lowercase hex chars>", id)
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
	return textutil.UniqueSorted(values)
}

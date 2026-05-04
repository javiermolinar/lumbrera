package writecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

func inferOperation(kind string, exists bool, opts options) (operation, error) {
	if opts.AppendSet && opts.Delete {
		return "", fmt.Errorf("--append and --delete cannot be combined")
	}
	if opts.Delete {
		return opDelete, nil
	}
	if opts.AppendSet {
		return opAppend, nil
	}
	if kind == "source" {
		if exists {
			return "", fmt.Errorf("sources are immutable; refusing to update existing source")
		}
		return opSource, nil
	}
	if exists {
		return opUpdate, nil
	}
	return opCreate, nil
}

func applyMutation(repo, target, kind string, op operation, opts options, input []byte) error {
	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	switch op {
	case opDelete:
		return os.Remove(absTarget)
	case opSource:
		body := normalizeBody(input)
		return writeDocument(absTarget, target, kind, opts.Title, opts.Summary, opts.Tags, nil, body)
	case opCreate:
		body := normalizeBody(input)
		sources, err := mergeSourceCitations(target, body, normalizeSources(opts.Sources))
		if err != nil {
			return err
		}
		body = md.AppendSourcesSection(body, target, sources)
		return writeDocument(absTarget, target, kind, opts.Title, opts.Summary, opts.Tags, sources, body)
	case opUpdate:
		existingMeta, _, err := readExistingDocument(absTarget)
		if err != nil {
			return err
		}
		title := existingMeta.Title
		if strings.TrimSpace(opts.Title) != "" {
			title = opts.Title
		}
		summary := existingMeta.Summary
		if strings.TrimSpace(opts.Summary) != "" {
			summary = opts.Summary
		}
		tags := existingMeta.Tags
		if len(opts.Tags) > 0 {
			tags = opts.Tags
		}
		body := normalizeBody(input)
		sources, err := mergeSourceCitations(target, body, mergePaths(existingMeta.Lumbrera.Sources, normalizeSources(opts.Sources)))
		if err != nil {
			return err
		}
		body = md.AppendSourcesSection(body, target, sources)
		return writeDocument(absTarget, target, kind, title, summary, tags, sources, body)
	case opAppend:
		existingMeta, existingBody, err := readExistingDocument(absTarget)
		if err != nil {
			return err
		}
		body := md.RemoveSourcesSection(existingBody)
		body = md.AppendToSection(body, opts.Append, string(input))
		sources, err := mergeSourceCitations(target, body, mergePaths(existingMeta.Lumbrera.Sources, normalizeSources(opts.Sources)))
		if err != nil {
			return err
		}
		body = md.AppendSourcesSection(body, target, sources)
		return writeDocument(absTarget, target, kind, existingMeta.Title, existingMeta.Summary, existingMeta.Tags, sources, body)
	default:
		return fmt.Errorf("unsupported operation %q", op)
	}
}

func writeDocument(absTarget, target, kind, title, summary string, tags, sources []string, body string) error {
	analysis, err := md.AnalyzeWithOptions(target, body, md.AnalyzeOptions{SourceCitations: kind == "wiki"})
	if err != nil {
		return err
	}
	if analysis.FirstH1 != "" && strings.TrimSpace(title) != "" && analysis.FirstH1 != strings.TrimSpace(title) {
		return fmt.Errorf("first H1 %q must match --title %q", analysis.FirstH1, strings.TrimSpace(title))
	}
	links := filterWikiLinks(analysis.Links)
	if kind == "source" {
		sources = nil
	}
	meta := frontmatter.New(kind, title, summary, tags, sources, links)
	content, err := frontmatter.Attach(meta, body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absTarget, []byte(content), 0o644)
}

func readExistingDocument(absPath string) (frontmatter.Document, string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	if !has {
		return frontmatter.Document{}, "", fmt.Errorf("existing document %s has no Lumbrera-generated frontmatter", absPath)
	}
	return meta, body, nil
}

func mergeSourceCitations(target, body string, sources []string) ([]string, error) {
	analysis, err := md.Analyze(target, body)
	if err != nil {
		return nil, err
	}
	for _, citation := range analysis.SourceCitations {
		if !strings.HasPrefix(citation.Path, "sources/") {
			return nil, fmt.Errorf("%s source citation must link only to sources/, got %s", target, citation.String())
		}
	}
	return mergePaths(sources, referencePaths(analysis.SourceCitations)), nil
}

func normalizeSources(sources []string) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		normalized, _, err := normalizeTargetPath(source)
		if err == nil {
			out = append(out, normalized)
		}
	}
	return mergePaths(out, nil)
}

func normalizeBody(input []byte) string {
	body := strings.ReplaceAll(string(input), "\r\n", "\n")
	return strings.Trim(body, "\n") + "\n"
}

func hasSourcesSection(body string) bool {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return md.RemoveSourcesSection(body) != strings.TrimRight(body, "\n")
}

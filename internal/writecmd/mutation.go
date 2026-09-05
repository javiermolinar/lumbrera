package writecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

func inferOperation(policy brain.ContentPolicy, exists bool, opts options) (operation, error) {
	if opts.AppendSet && opts.Delete {
		return "", fmt.Errorf("--append and --delete cannot be combined")
	}
	if opts.Delete {
		return opDelete, nil
	}
	if opts.AppendSet {
		return opAppend, nil
	}

	switch policy.Storage {
	case brain.StorageRawMarkdown:
		if exists {
			return "", fmt.Errorf("%s documents are immutable; refusing to update existing document", policy.Kind)
		}
		return opSource, nil
	case brain.StorageBinary:
		if exists {
			return "", fmt.Errorf("%s documents are immutable; refusing to update existing document", policy.Kind)
		}
		return opAsset, nil
	case brain.StorageManagedMarkdown:
		if !policy.Mutable {
			return "", fmt.Errorf("%s documents are not mutable", policy.Kind)
		}
		if exists {
			return opUpdate, nil
		}
		return opCreate, nil
	default:
		return "", fmt.Errorf("unsupported storage policy for kind %q", policy.Kind)
	}
}

func applyMutation(repo, target string, policy brain.ContentPolicy, op operation, opts options, input []byte) error {
	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	switch op {
	case opDelete:
		return os.Remove(absTarget)
	case opSource:
		return writeRawFile(absTarget, input)
	case opAsset:
		content, err := os.ReadFile(opts.File)
		if err != nil {
			return fmt.Errorf("read --file %q: %w", opts.File, err)
		}
		return writeRawFile(absTarget, content)
	case opCreate, opUpdate, opAppend:
		if policy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("operation %q requires a managed Markdown policy, got %q", op, policy.Kind)
		}
		return applyManagedMutation(repo, absTarget, target, policy, op, opts, input)
	default:
		return fmt.Errorf("unsupported operation %q", op)
	}
}

func applyManagedMutation(repo, absTarget, target string, policy brain.ContentPolicy, op operation, opts options, input []byte) error {
	var (
		id               string
		title            string
		summary          string
		tags             []string
		body             string
		existingEvidence []string
	)

	switch op {
	case opCreate:
		title = opts.Title
		summary = opts.Summary
		tags = opts.Tags
		body = normalizeBody(input)
	case opUpdate:
		existingMeta, _, err := readExistingManagedDocument(absTarget, policy)
		if err != nil {
			return err
		}
		id = existingMeta.Lumbrera.ID
		title = existingMeta.Title
		if strings.TrimSpace(opts.Title) != "" {
			title = opts.Title
		}
		summary = existingMeta.Summary
		if strings.TrimSpace(opts.Summary) != "" {
			summary = opts.Summary
		}
		tags = existingMeta.Tags
		if len(opts.Tags) > 0 {
			tags = opts.Tags
		}
		existingEvidence = existingMeta.Lumbrera.Sources
		body = normalizeBody(input)
	case opAppend:
		existingMeta, existingBody, err := readExistingManagedDocument(absTarget, policy)
		if err != nil {
			return err
		}
		id = existingMeta.Lumbrera.ID
		title = existingMeta.Title
		summary = existingMeta.Summary
		tags = existingMeta.Tags
		existingEvidence = existingMeta.Lumbrera.Sources
		body = md.RemoveSourcesSection(existingBody)
		body = md.AppendToSection(body, opts.Append, string(input))
	default:
		return fmt.Errorf("unsupported managed document operation %q", op)
	}

	existingEvidence, err := normalizeAndValidateEvidencePaths(repo, target, policy, existingEvidence, "existing evidence")
	if err != nil {
		return err
	}
	suppliedEvidence, err := normalizeAndValidateEvidencePaths(repo, target, policy, opts.Sources, "--source")
	if err != nil {
		return err
	}

	analysis, err := md.AnalyzeWithOptions(target, body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(policy.Kind)})
	if err != nil {
		return err
	}
	citationEvidence := referencePaths(analysis.SourceCitations)
	citationEvidence, err = normalizeAndValidateEvidencePaths(repo, target, policy, citationEvidence, "source citation")
	if err != nil {
		return err
	}

	// Evidence is cumulative by design. Replacing body content does not imply
	// removing document-level provenance; removal requires an explicit graph mutation.
	evidence := mergePaths(existingEvidence, suppliedEvidence, citationEvidence)
	if !brain.AcceptsEvidence(policy.Kind) && len(evidence) > 0 {
		return fmt.Errorf("%s documents must not contain evidence", policy.Kind)
	}
	if policy.RequiresEvidence && len(evidence) == 0 {
		return fmt.Errorf("%s documents require at least one evidence reference", policy.Kind)
	}
	if len(evidence) > 0 {
		body = md.AppendSourcesSection(body, target, evidence)
	} else {
		body = strings.TrimRight(md.RemoveSourcesSection(body), "\n") + "\n"
	}

	return writeManagedDocument(absTarget, target, policy, id, title, summary, tags, evidence, body)
}

func writeRawFile(absTarget string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absTarget, content, 0o644)
}

func writeManagedDocument(absTarget, target string, policy brain.ContentPolicy, id, title, summary string, tags, evidence []string, body string) error {
	analysis, err := md.AnalyzeWithOptions(target, body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(policy.Kind)})
	if err != nil {
		return err
	}
	if analysis.FirstH1 != "" && strings.TrimSpace(title) != "" && analysis.FirstH1 != strings.TrimSpace(title) {
		return fmt.Errorf("first H1 %q must match --title %q", analysis.FirstH1, strings.TrimSpace(title))
	}
	links := filterManagedLinks(analysis.Links)
	meta := frontmatter.New(string(policy.Kind), title, summary, tags, evidence, links)
	if strings.TrimSpace(id) != "" {
		meta = frontmatter.NewWithID(id, string(policy.Kind), title, summary, tags, evidence, links)
	}
	meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	content, err := frontmatter.AttachForPolicy(meta, policy, body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absTarget, []byte(content), 0o644)
}

func readExistingManagedDocument(absPath string, policy brain.ContentPolicy) (frontmatter.Document, string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	meta, body, has, err := frontmatter.SplitForPolicy(content, policy)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	if !has {
		return frontmatter.Document{}, "", fmt.Errorf("existing document %s has no Lumbrera-generated frontmatter", absPath)
	}
	return meta, body, nil
}

func normalizeBody(input []byte) string {
	body := strings.ReplaceAll(string(input), "\r\n", "\n")
	return strings.Trim(body, "\n") + "\n"
}

func hasSourcesSection(body string) bool {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return md.RemoveSourcesSection(body) != strings.TrimRight(body, "\n")
}

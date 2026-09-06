package verify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/textutil"
)

func ValidateDocuments(repo string) error {
	return validateDocumentsForPolicies(repo, brain.ManagedPolicies(), true)
}

func validateDocumentsForPolicies(repo string, policies []brain.ContentPolicy, requireModifiedDate bool) error {
	roots := make([]string, 0, len(policies))
	byKind := make(map[brain.Kind]brain.ContentPolicy, len(policies))
	for _, policy := range policies {
		if policy.Storage != brain.StorageManagedMarkdown {
			continue
		}
		roots = append(roots, policy.Root)
		byKind[policy.Kind] = policy
	}
	seenIDs := map[string]string{}
	return brainfs.WalkMarkdown(repo, roots, func(file brainfs.MarkdownFile) error {
		pathPolicy, ok := brain.PolicyForPath(file.RelPath)
		if !ok || pathPolicy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("%s does not resolve to a managed content policy", file.RelPath)
		}
		policy, ok := byKind[pathPolicy.Kind]
		if !ok || policy.Root != pathPolicy.Root {
			return fmt.Errorf("%s is not valid for this brain version", file.RelPath)
		}
		id, err := validateManagedDocument(repo, file.AbsPath, file.RelPath, policy, requireModifiedDate)
		if err != nil {
			return err
		}
		if existing, ok := seenIDs[id]; ok {
			return fmt.Errorf("%s duplicates Lumbrera document id %s from %s", file.RelPath, id, existing)
		}
		seenIDs[id] = file.RelPath
		return nil
	})
}

func validateManagedDocument(repo, absPath, relPath string, policy brain.ContentPolicy, requireModifiedDate bool) (string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	meta, body, has, err := frontmatter.SplitForPolicy(content, policy)
	if err != nil {
		return "", fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !has {
		return "", fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if requireModifiedDate && strings.TrimSpace(meta.Lumbrera.ModifiedDate) == "" {
		return "", fmt.Errorf("%s is missing generated lumbrera.modified_date", relPath)
	}
	if lines := markdownLineCount(body); lines > brain.MaxManagedDocumentBodyLines {
		return "", fmt.Errorf("%s exceeds max %s page length: %d lines, max %d. Split it into smaller topic/task pages", relPath, policy.Kind, lines, brain.MaxManagedDocumentBodyLines)
	}
	acceptsEvidence := brain.AcceptsEvidence(policy.Kind)
	if !acceptsEvidence {
		citationAnalysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: true})
		if err != nil {
			return "", fmt.Errorf("%s must not contain source citations: %w", relPath, err)
		}
		if citationAnalysis.HasSourceCitationSyntax {
			return "", fmt.Errorf("%s must not contain source citations", relPath)
		}
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: acceptsEvidence})
	if err != nil {
		return "", fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	if analysis.FirstH1 != "" && analysis.FirstH1 != meta.Title {
		return "", fmt.Errorf("%s first H1 %q does not match generated title %q", relPath, analysis.FirstH1, meta.Title)
	}
	if err := validateInternalReferencesExist(repo, relPath, analysis.LinkReferences); err != nil {
		return "", err
	}
	if !sameStrings(meta.Lumbrera.Links, filterManagedLinks(analysis.Links)) {
		return "", fmt.Errorf("%s frontmatter links are stale; regenerate through lumbrera write", relPath)
	}
	if err := validateDocumentEvidence(repo, relPath, policy, meta, analysis); err != nil {
		return "", err
	}
	return meta.Lumbrera.ID, nil
}

func validateDocumentEvidence(repo, relPath string, policy brain.ContentPolicy, meta frontmatter.Document, analysis md.Analysis) error {
	if !brain.AcceptsEvidence(policy.Kind) {
		if len(meta.Lumbrera.Sources) > 0 {
			return fmt.Errorf("%s kind %q must not contain evidence metadata", relPath, policy.Kind)
		}
		if analysis.HasSourcesSection {
			return fmt.Errorf("%s kind %q must not contain a ## Sources section", relPath, policy.Kind)
		}
		return nil
	}

	if err := validateEvidenceReferences(repo, relPath, policy, "Sources section", analysis.SourceReferences); err != nil {
		return err
	}
	if err := validateEvidenceReferences(repo, relPath, policy, "source citation", analysis.SourceCitations); err != nil {
		return err
	}

	expectedEvidence := mergePaths(analysis.Sources, referencePaths(analysis.SourceCitations))
	if policy.RequiresEvidence && len(expectedEvidence) == 0 {
		return fmt.Errorf("%s is missing a ## Sources section with evidence links", relPath)
	}
	if len(expectedEvidence) > 0 && !analysis.HasSourcesSection {
		return fmt.Errorf("%s is missing a generated ## Sources section", relPath)
	}
	if !sameStrings(meta.Lumbrera.Sources, analysis.Sources) {
		return fmt.Errorf("%s generated Sources section is stale; regenerate through lumbrera write", relPath)
	}
	if !sameStrings(meta.Lumbrera.Sources, expectedEvidence) {
		return fmt.Errorf("%s frontmatter sources are stale; regenerate through lumbrera write", relPath)
	}
	return nil
}

func validateEvidenceReferences(repo, relPath string, documentPolicy brain.ContentPolicy, relationship string, refs []md.Reference) error {
	for _, ref := range refs {
		if ref.Path == relPath {
			return fmt.Errorf("%s %s must not reference the document itself: %s", relPath, relationship, ref.String())
		}
		evidencePolicy, ok := brain.PolicyForPath(ref.Path)
		if !ok || !brain.CanUseAsEvidence(documentPolicy.Kind, evidencePolicy.Kind) || !policyAllowsEvidenceKind(documentPolicy, evidencePolicy.Kind) {
			return fmt.Errorf("%s %s cannot use %s as evidence for kind %q", relPath, relationship, ref.String(), documentPolicy.Kind)
		}
	}
	return validateInternalReferencesExist(repo, relPath, refs)
}

func policyAllowsEvidenceKind(policy brain.ContentPolicy, evidenceKind brain.Kind) bool {
	for _, allowed := range policy.EvidenceKinds {
		if allowed == evidenceKind {
			return true
		}
	}
	return false
}

func markdownLineCount(body string) int {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return 0
	}
	return strings.Count(body, "\n") + 1
}

func validateInternalReferencesExist(repo, relPath string, refs []md.Reference) error {
	for _, ref := range refs {
		if ref.Path == "" {
			continue
		}
		if err := pathpolicy.EnsureSafeFilesystemTarget(repo, ref.Path); err != nil {
			return fmt.Errorf("%s links to unsafe path %s: %w", relPath, ref.String(), err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(ref.Path))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s links to missing file %s", relPath, ref.Path)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%s links to non-regular file %s", relPath, ref.Path)
		}
		if ref.Anchor == "" {
			continue
		}
		anchors, err := documentAnchors(repo, ref.Path)
		if err != nil {
			return err
		}
		if _, ok := anchors[ref.Anchor]; !ok {
			return fmt.Errorf("%s links to missing anchor #%s in %s", relPath, ref.Anchor, ref.Path)
		}
	}
	return nil
}

func documentAnchors(repo, relPath string) (map[string]struct{}, error) {
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, err
	}
	body := string(content)
	policy, ok := brain.PolicyForPath(relPath)
	analyzeOpts := md.AnalyzeOptions{IgnoreLinks: ok && policy.Storage == brain.StorageRawMarkdown}
	if ok && policy.Storage == brain.StorageManagedMarkdown {
		_, splitBody, has, err := frontmatter.SplitForPolicy(content, policy)
		if err != nil {
			return nil, fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
		}
		if has {
			body = splitBody
		}
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, analyzeOpts)
	if err != nil {
		return nil, fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	anchors := make(map[string]struct{}, len(analysis.Anchors))
	for _, anchor := range analysis.Anchors {
		anchors[anchor] = struct{}{}
	}
	return anchors, nil
}

func mergePaths(groups ...[]string) []string {
	return textutil.MergeStrings(groups...)
}

func referencePaths(refs []md.Reference) []string {
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	return mergePaths(paths)
}

func filterManagedLinks(links []string) []string {
	var out []string
	for _, link := range links {
		if brain.IsManagedPath(link) {
			out = append(out, link)
		}
	}
	return mergePaths(out)
}

func sameStrings(a, b []string) bool {
	return textutil.SameStringSet(a, b)
}

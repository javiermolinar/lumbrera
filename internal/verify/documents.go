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
	seenIDs := map[string]string{}
	return brainfs.WalkMarkdown(repo, []string{"wiki"}, func(file brainfs.MarkdownFile) error {
		id, err := validateWikiDocument(repo, file.AbsPath, file.RelPath)
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

func validateWikiDocument(repo, absPath, relPath string) (string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return "", fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !has {
		return "", fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if meta.Lumbrera.Kind != "wiki" {
		return "", fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, "wiki")
	}
	if lines := markdownLineCount(body); lines > brain.MaxWikiBodyLines {
		return "", fmt.Errorf("%s exceeds max wiki page length: %d lines, max %d. Split it into smaller topic/task pages", relPath, lines, brain.MaxWikiBodyLines)
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return "", fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	if analysis.FirstH1 != "" && analysis.FirstH1 != meta.Title {
		return "", fmt.Errorf("%s first H1 %q does not match generated title %q", relPath, analysis.FirstH1, meta.Title)
	}
	if err := validateInternalReferencesExist(repo, relPath, analysis.LinkReferences); err != nil {
		return "", err
	}
	if err := validateSourceCitations(repo, relPath, analysis.SourceCitations); err != nil {
		return "", err
	}
	if !sameStrings(meta.Lumbrera.Links, filterWikiLinks(analysis.Links)) {
		return "", fmt.Errorf("%s frontmatter links are stale; regenerate through lumbrera write", relPath)
	}
	if len(analysis.Sources) == 0 {
		return "", fmt.Errorf("%s is missing a ## Sources section with source links", relPath)
	}
	for _, source := range analysis.Sources {
		if !strings.HasPrefix(source, "sources/") {
			return "", fmt.Errorf("%s Sources section must link only to sources/, got %s", relPath, source)
		}
	}
	if err := validateInternalReferencesExist(repo, relPath, analysis.SourceReferences); err != nil {
		return "", err
	}
	expectedSources := mergePaths(analysis.Sources, referencePaths(analysis.SourceCitations))
	if !sameStrings(meta.Lumbrera.Sources, expectedSources) {
		return "", fmt.Errorf("%s frontmatter sources are stale; regenerate through lumbrera write", relPath)
	}
	return meta.Lumbrera.ID, nil
}

func markdownLineCount(body string) int {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return 0
	}
	return strings.Count(body, "\n") + 1
}

func validateSourceCitations(repo, relPath string, citations []md.Reference) error {
	for _, citation := range citations {
		if !strings.HasPrefix(citation.Path, "sources/") {
			return fmt.Errorf("%s source citation must link only to sources/, got %s", relPath, citation.String())
		}
	}
	return validateInternalReferencesExist(repo, relPath, citations)
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
	analyzeOpts := md.AnalyzeOptions{IgnoreLinks: strings.HasPrefix(relPath, "sources/")}
	if strings.HasPrefix(relPath, "wiki/") {
		_, splitBody, has, err := frontmatter.Split(content)
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

func filterWikiLinks(links []string) []string {
	var out []string
	for _, link := range links {
		if strings.HasPrefix(link, "wiki/") {
			out = append(out, link)
		}
	}
	return mergePaths(out)
}

func sameStrings(a, b []string) bool {
	return textutil.SameStringSet(a, b)
}

package verify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

var allowedRootFiles = map[string]struct{}{
	brain.IndexPath:     {},
	brain.ChangelogPath: {},
	brain.BrainSumPath:  {},
	"AGENTS.md":         {},
	"CLAUDE.md":         {},
	".gitignore":        {},
}

type Options struct{}

func Run(repo string, opts Options) error {
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}
	if err := ValidatePathPolicy(repo); err != nil {
		return err
	}
	if err := ValidateDocuments(repo); err != nil {
		return err
	}
	_ = opts
	return VerifyGeneratedFiles(repo)
}

func ValidatePathPolicy(repo string) error {
	return filepath.WalkDir(repo, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if absPath == repo {
			return nil
		}
		rel, err := filepath.Rel(repo, absPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".brain" || strings.HasPrefix(rel, ".brain/") || rel == ".agents" || strings.HasPrefix(rel, ".agents/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".claude" || strings.HasPrefix(rel, ".claude/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			if rel == "CLAUDE.md" || rel == ".claude" {
				return nil
			}
			return fmt.Errorf("path %s is a symlink; Lumbrera content paths must not use symlinks", rel)
		}

		if entry.IsDir() {
			if rel == "sources" || strings.HasPrefix(rel, "sources/") || rel == "wiki" || strings.HasPrefix(rel, "wiki/") {
				return nil
			}
			return fmt.Errorf("unexpected directory %s; Lumbrera content must live under sources/ or wiki/", rel)
		}

		if strings.HasPrefix(rel, "sources/") || strings.HasPrefix(rel, "wiki/") {
			if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				if _, _, err := pathpolicy.NormalizeTargetPath(rel); err != nil {
					return err
				}
			}
			return nil
		}

		if isAllowedRootFile(rel) {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return fmt.Errorf("unexpected Markdown file %s; Lumbrera content must live under sources/ or wiki/", rel)
		}
		return nil
	})
}

func isAllowedRootFile(rel string) bool {
	if _, ok := allowedRootFiles[rel]; ok {
		return true
	}
	switch strings.ToUpper(rel) {
	case "README", "README.MD", "LICENSE", "LICENSE.MD", "LICENSE.TXT", "COPYING", "COPYING.MD":
		return true
	default:
		return false
	}
}

func ValidateDocuments(repo string) error {
	root := filepath.Join(repo, "wiki")
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return filepath.WalkDir(root, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is not a regular Markdown file", absPath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular Markdown file", absPath)
		}
		rel, err := filepath.Rel(repo, absPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		return validateWikiDocument(repo, absPath, rel)
	})
}

func validateWikiDocument(repo, absPath, relPath string) error {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !has {
		return fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if meta.Lumbrera.Kind != "wiki" {
		return fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, "wiki")
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	if analysis.FirstH1 != "" && analysis.FirstH1 != meta.Title {
		return fmt.Errorf("%s first H1 %q does not match generated title %q", relPath, analysis.FirstH1, meta.Title)
	}
	if err := validateInternalReferencesExist(repo, relPath, analysis.LinkReferences); err != nil {
		return err
	}
	if err := validateSourceCitations(repo, relPath, analysis.SourceCitations); err != nil {
		return err
	}
	if !sameStrings(meta.Lumbrera.Links, filterWikiLinks(analysis.Links)) {
		return fmt.Errorf("%s frontmatter links are stale; regenerate through lumbrera write", relPath)
	}
	if len(analysis.Sources) == 0 {
		return fmt.Errorf("%s is missing a ## Sources section with source links", relPath)
	}
	for _, source := range analysis.Sources {
		if !strings.HasPrefix(source, "sources/") {
			return fmt.Errorf("%s Sources section must link only to sources/, got %s", relPath, source)
		}
	}
	if err := validateInternalReferencesExist(repo, relPath, analysis.SourceReferences); err != nil {
		return err
	}
	expectedSources := mergePaths(analysis.Sources, referencePaths(analysis.SourceCitations))
	if !sameStrings(meta.Lumbrera.Sources, expectedSources) {
		return fmt.Errorf("%s frontmatter sources are stale; regenerate through lumbrera write", relPath)
	}
	return nil
}

func VerifyGeneratedFiles(repo string) error {
	files, err := generate.FilesForRepo(repo)
	if err != nil {
		return err
	}
	checks := map[string]string{
		brain.IndexPath:     files.Index,
		brain.ChangelogPath: files.Changelog,
		brain.BrainSumPath:  files.BrainSum,
	}
	for rel, want := range checks {
		got, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("generated file %s is missing: %w", rel, err)
		}
		if string(got) != want {
			return fmt.Errorf("generated file %s is stale; regenerate through lumbrera write or restore generated metadata", rel)
		}
	}
	return nil
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
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
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
	}
	sort.Strings(out)
	return out
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
	a = mergePaths(a)
	b = mergePaths(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

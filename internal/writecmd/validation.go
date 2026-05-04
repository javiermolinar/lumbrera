package writecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

func validateDocuments(repo string) error {
	for _, dir := range []string{"sources", "wiki"} {
		root := filepath.Join(repo, dir)
		if _, err := os.Stat(root); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if err := filepath.WalkDir(root, func(absPath string, entry os.DirEntry, err error) error {
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
			return validateDocument(repo, absPath, rel, strings.TrimSuffix(dir, "s"))
		}); err != nil {
			return err
		}
	}
	return nil
}

func validateDocument(repo, absPath, relPath, wantKind string) error {
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
	if meta.Lumbrera.Kind != wantKind {
		return fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, wantKind)
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{SourceCitations: wantKind == "wiki"})
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
	if wantKind == "source" {
		if len(meta.Lumbrera.Sources) > 0 {
			return fmt.Errorf("%s source frontmatter must not list provenance sources", relPath)
		}
		return nil
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

func verifyGeneratedFiles(repo string, pending []generate.PendingChangelogEntry) error {
	files, err := generate.FilesForRepoWithPending(repo, pending)
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
			return fmt.Errorf("generated file %s is stale; run lumbrera sync first", rel)
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
		if err := ensureSafeFilesystemTarget(repo, ref.Path); err != nil {
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
	_, body, has, err := frontmatter.Split(content)
	if err != nil {
		return nil, fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !has {
		body = string(content)
	}
	analysis, err := md.AnalyzeWithOptions(relPath, body, md.AnalyzeOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	anchors := make(map[string]struct{}, len(analysis.Anchors))
	for _, anchor := range analysis.Anchors {
		anchors[anchor] = struct{}{}
	}
	return anchors, nil
}

func validateOptionsForOperation(repo, target, kind string, exists bool, op operation, opts options) error {
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	if op == opDelete {
		if !exists {
			return fmt.Errorf("cannot delete %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to delete existing source")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 || len(opts.Sources) > 0 {
			return fmt.Errorf("--delete cannot be combined with --title, --summary, --tag, or --source")
		}
		return nil
	}

	if kind == "source" {
		if len(opts.Sources) > 0 {
			return fmt.Errorf("source writes must not specify --source")
		}
		if op != opSource {
			return fmt.Errorf("sources are immutable; refusing to mutate existing source")
		}
	}

	if kind == "wiki" {
		if len(opts.Sources) == 0 {
			return fmt.Errorf("wiki writes require at least one --source")
		}
		if err := validateSourcePaths(repo, opts.Sources); err != nil {
			return err
		}
	}

	if (op == opSource || op == opCreate) && strings.TrimSpace(opts.Title) == "" {
		return fmt.Errorf("--title is required when creating a new file")
	}
	if op == opAppend {
		if !exists {
			return fmt.Errorf("cannot append to %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to append to existing source")
		}
		section := strings.TrimSpace(opts.Append)
		if section == "" {
			return fmt.Errorf("--append requires a non-empty section name")
		}
		if strings.EqualFold(section, "Sources") {
			return fmt.Errorf("--append cannot target the generated Sources section")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 {
			return fmt.Errorf("--append cannot change --title, --summary, or --tag in this version")
		}
	}
	return nil
}

func validateSourcePaths(repo string, sources []string) error {
	for _, source := range sources {
		normalized, kind, err := normalizeTargetPath(source)
		if err != nil {
			return fmt.Errorf("invalid --source %q: %w", source, err)
		}
		if kind != "source" {
			return fmt.Errorf("--source %q must be under sources/", source)
		}
		if err := ensureSafeFilesystemTarget(repo, normalized); err != nil {
			return fmt.Errorf("--source %q is unsafe: %w", source, err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(normalized))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("--source %q does not exist", source)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("--source %q must be a regular Markdown file", source)
		}
	}
	return nil
}

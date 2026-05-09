package movecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

// wikiRef holds a parsed wiki document with its path, frontmatter, and body.
type wikiRef struct {
	relPath string
	meta    frontmatter.Document
	body    string
}

// loadWikiRefs loads all wiki documents with valid frontmatter.
func loadWikiRefs(repo string) ([]wikiRef, error) {
	var refs []wikiRef
	err := brainfs.WalkMarkdown(repo, []string{"wiki"}, func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.Split(content)
		if err != nil || !has {
			return err
		}
		refs = append(refs, wikiRef{relPath: file.RelPath, meta: meta, body: body})
		return nil
	})
	return refs, err
}

// planMove determines which wiki pages need link rewriting after a move.
func planMove(from, to, kind string, allRefs []wikiRef) (map[string]wikiRef, error) {
	updates := make(map[string]wikiRef)

	for _, ref := range allRefs {
		if ref.relPath == from {
			continue // the moved file itself is handled separately
		}

		var updated wikiRef
		var changed bool
		var err error

		switch kind {
		case "wiki":
			updated, changed, err = rewriteWikiLink(ref, from, to)
		case "source":
			updated, changed, err = rewriteSourceRef(ref, from, to)
		case "asset":
			updated, changed, err = rewriteAssetRef(ref, from, to)
		default:
			return nil, fmt.Errorf("unsupported move kind %q", kind)
		}
		if err != nil {
			return nil, err
		}
		if changed {
			updates[updated.relPath] = updated
		}
	}
	return updates, nil
}

// rewriteWikiLink updates links to a moved wiki page in another wiki page.
func rewriteWikiLink(ref wikiRef, from, to string) (wikiRef, bool, error) {
	// Check frontmatter links.
	hasLink := false
	for _, link := range ref.meta.Lumbrera.Links {
		if link == from {
			hasLink = true
			break
		}
	}

	// Check body for markdown links.
	oldRelLink := md.RelativeLink(ref.relPath, from)
	bodyHasLink := strings.Contains(ref.body, oldRelLink) || strings.Contains(ref.body, from)

	if !hasLink && !bodyHasLink {
		return ref, false, nil
	}

	body := rewriteLinksInBody(ref.body, ref.relPath, from, to)

	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return ref, false, err
	}

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Links = filterWikiLinks(analysis.Links)
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, true, nil
}

// rewriteSourceRef updates source references in a wiki page after a source move.
func rewriteSourceRef(ref wikiRef, from, to string) (wikiRef, bool, error) {
	hasSource := false
	for _, src := range ref.meta.Lumbrera.Sources {
		if src == from {
			hasSource = true
			break
		}
	}

	oldRelLink := md.RelativeLink(ref.relPath, from)
	bodyHasRef := strings.Contains(ref.body, oldRelLink) || strings.Contains(ref.body, from)

	if !hasSource && !bodyHasRef {
		return ref, false, nil
	}

	// Update frontmatter sources.
	newSources := make([]string, 0, len(ref.meta.Lumbrera.Sources))
	for _, src := range ref.meta.Lumbrera.Sources {
		if src == from {
			newSources = append(newSources, to)
		} else {
			newSources = append(newSources, src)
		}
	}

	// Rewrite body: inline citations and ## Sources section links.
	body := rewriteLinksInBody(ref.body, ref.relPath, from, to)
	body = rewriteSourceCitations(body, ref.relPath, from, to)

	// Regenerate ## Sources section with updated paths.
	body = md.RemoveSourcesSection(body)
	if len(newSources) > 0 {
		body = md.AppendSourcesSection(body, ref.relPath, newSources)
	}

	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return ref, false, err
	}

	citationPaths := referencePaths(analysis.SourceCitations)
	allSources := mergePaths(newSources, citationPaths)

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Sources = allSources
	updated.meta.Lumbrera.Links = filterWikiLinks(analysis.Links)
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, true, nil
}

// rewriteAssetRef updates asset references in a wiki page after an asset move.
func rewriteAssetRef(ref wikiRef, from, to string) (wikiRef, bool, error) {
	oldRelLink := md.RelativeLink(ref.relPath, from)
	if !strings.Contains(ref.body, oldRelLink) && !strings.Contains(ref.body, from) {
		return ref, false, nil
	}

	body := rewriteLinksInBody(ref.body, ref.relPath, from, to)

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, true, nil
}

// rewriteLinksInBody replaces markdown links and images pointing to oldPath with newPath.
func rewriteLinksInBody(body, fromDoc, oldPath, newPath string) string {
	oldRelLink := md.RelativeLink(fromDoc, oldPath)
	newRelLink := md.RelativeLink(fromDoc, newPath)

	candidates := relVariants(oldRelLink, oldPath)
	for _, old := range candidates {
		escaped := regexp.QuoteMeta(old)
		newRepl := escapeRepl(newRelLink)
		// Rewrite [text](old) and [text](old#anchor) -- covers links and images.
		pattern := fmt.Sprintf(`(\]\()%s((?:#[^\)]*)?)\)`, escaped)
		re := regexp.MustCompile(pattern)
		body = re.ReplaceAllString(body, "${1}"+newRepl+"${2})")
	}
	return body
}

// rewriteSourceCitations replaces [source: old-path#anchor] with [source: new-path#anchor].
// relVariants returns all forms of a relative link that might appear in body text.
func relVariants(relLink, repoPath string) []string {
	v := []string{relLink, repoPath}
	if strings.HasPrefix(relLink, "./") {
		v = append(v, strings.TrimPrefix(relLink, "./"))
	}
	return v
}

func rewriteSourceCitations(body, fromDoc, oldPath, newPath string) string {
	oldRelLink := md.RelativeLink(fromDoc, oldPath)
	newRelLink := md.RelativeLink(fromDoc, newPath)

	candidates := relVariants(oldRelLink, oldPath)
	for _, old := range candidates {
		escaped := regexp.QuoteMeta(old)
		pattern := fmt.Sprintf(`(\[source:\s*)%s((?:#[^\]]*?)?\])`, escaped)
		re := regexp.MustCompile("(?i)" + pattern)
		body = re.ReplaceAllString(body, "${1}"+escapeRepl(newRelLink)+"${2}")
	}
	return body
}

// rewriteMovedWikiPage updates internal relative links and Sources section
// inside the moved wiki page itself (since relative paths change).
func rewriteMovedWikiPage(content []byte, from, to string) ([]byte, error) {
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return nil, fmt.Errorf("split frontmatter: %w", err)
	}
	if !has {
		// Non-wiki or raw file, just return as-is.
		return content, nil
	}

	// Rewrite relative links in body: they now originate from `to` instead of `from`.
	analysis, err := md.AnalyzeWithOptions(from, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return nil, err
	}

	// Collect all unique repo-relative paths that appear as relative links.
	// Build a single old→new replacement map to avoid chained replacements.
	replacements := map[string]string{}
	for _, link := range analysis.Links {
		oldRel := md.RelativeLink(from, link)
		newRel := md.RelativeLink(to, link)
		if oldRel != newRel {
			replacements[oldRel] = newRel
		}
	}
	for _, src := range analysis.Sources {
		oldRel := md.RelativeLink(from, src)
		newRel := md.RelativeLink(to, src)
		if oldRel != newRel {
			replacements[oldRel] = newRel
		}
	}
	for _, cite := range analysis.SourceCitations {
		oldRel := md.RelativeLink(from, cite.Path)
		newRel := md.RelativeLink(to, cite.Path)
		if oldRel != newRel {
			replacements[oldRel] = newRel
		}
	}

	// Apply replacements using regex to match only inside link/image/citation syntax,
	// not arbitrary substrings. Try both the ./prefixed and bare forms.
	for oldRel, newRel := range replacements {
		variants := []string{oldRel}
		if strings.HasPrefix(oldRel, "./") {
			variants = append(variants, strings.TrimPrefix(oldRel, "./"))
		}

		for _, variant := range variants {
			escaped := regexp.QuoteMeta(variant)
			newEscaped := escapeRepl(newRel)

			// Rewrite markdown links: [text](oldRel) and [text](oldRel#anchor)
			linkPat := fmt.Sprintf(`(\]\()%s((?:#[^\)]*)?)\)`, escaped)
			body = regexp.MustCompile(linkPat).ReplaceAllString(body, "${1}"+newEscaped+"${2})")

			// Rewrite source citations: [source: oldRel] and [source: oldRel#anchor]
			citePat := fmt.Sprintf(`(\[source:\s*)%s((?:#[^\]]*?)?)\]`, escaped)
			body = regexp.MustCompile("(?i)"+citePat).ReplaceAllString(body, "${1}"+newEscaped+"${2}]")
		}
	}

	// Regenerate ## Sources section with correct relative links from new location.
	body = md.RemoveSourcesSection(body)
	if len(meta.Lumbrera.Sources) > 0 {
		body = md.AppendSourcesSection(body, to, meta.Lumbrera.Sources)
	}

	// Re-analyze from the new path.
	newAnalysis, err := md.AnalyzeWithOptions(to, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return nil, err
	}

	meta.Lumbrera.Links = filterWikiLinks(newAnalysis.Links)
	meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")

	result, err := frontmatter.Attach(meta, body)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

// writeWikiRef writes the updated wiki ref back to disk with regenerated frontmatter.
func writeWikiRef(repo string, ref wikiRef) error {
	absPath := filepath.Join(repo, filepath.FromSlash(ref.relPath))

	analysis, err := md.AnalyzeWithOptions(ref.relPath, ref.body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return fmt.Errorf("re-analyze %s: %w", ref.relPath, err)
	}

	links := filterWikiLinks(analysis.Links)
	citationPaths := referencePaths(analysis.SourceCitations)
	sources := mergePaths(ref.meta.Lumbrera.Sources, citationPaths)

	meta := frontmatter.NewWithID(
		ref.meta.Lumbrera.ID,
		ref.meta.Lumbrera.Kind,
		ref.meta.Title,
		ref.meta.Summary,
		ref.meta.Tags,
		sources,
		links,
	)
	meta.Lumbrera.ModifiedDate = ref.meta.Lumbrera.ModifiedDate

	content, err := frontmatter.Attach(meta, ref.body)
	if err != nil {
		return fmt.Errorf("attach frontmatter %s: %w", ref.relPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// escapeRepl escapes $ in replacement strings for regexp.
func escapeRepl(s string) string {
	return strings.ReplaceAll(s, "$", "$$")
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

func mergePaths(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, p := range group {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	sortStrings(out)
	return out
}

func referencePaths(refs []md.Reference) []string {
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	return mergePaths(paths)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

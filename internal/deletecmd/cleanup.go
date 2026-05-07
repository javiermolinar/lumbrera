package deletecmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

// cleanSourceFromWiki removes a source from a wiki ref:
// 1. Strips inline [source: ...] citations referencing the source
// 2. Removes the source from frontmatter sources
// 3. Regenerates the ## Sources section
func cleanSourceFromWiki(ref wikiRef, sourcePath string) (wikiRef, error) {
	body := stripSourceCitations(ref.body, ref.relPath, sourcePath)
	sources := removeFromSlice(ref.meta.Lumbrera.Sources, sourcePath)

	body = md.RemoveSourcesSection(body)
	if len(sources) > 0 {
		body = md.AppendSourcesSection(body, ref.relPath, sources)
	}

	// Re-analyze to get updated links.
	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return ref, err
	}

	// Merge explicit sources with any remaining citations.
	citationPaths := referencePaths(analysis.SourceCitations)
	allSources := mergePaths(sources, citationPaths)

	links := filterWikiLinks(analysis.Links)

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Sources = allSources
	updated.meta.Lumbrera.Links = links
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, nil
}

// cleanWikiLinkFromWiki removes all markdown links pointing to wikiPath from
// the ref's body, unwrapping [text](link) → text.
func cleanWikiLinkFromWiki(ref wikiRef, wikiPath string) (wikiRef, error) {
	body := unwrapLinksToPath(ref.body, ref.relPath, wikiPath)

	// Re-analyze to get updated links.
	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: true})
	if err != nil {
		return ref, err
	}

	links := filterWikiLinks(analysis.Links)

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Links = links
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, nil
}

// stripSourceCitations removes [source: <relative-path-to-sourcePath>] and
// [source: <relative-path-to-sourcePath>#anchor] from body text.
// It matches all relative path forms that resolve to sourcePath from the
// wiki document's location.
func stripSourceCitations(body, fromPath, sourcePath string) string {
	// Build possible relative paths from the wiki doc to the source.
	relLink := md.RelativeLink(fromPath, sourcePath)
	candidates := []string{relLink, sourcePath}

	for _, candidate := range candidates {
		// Escape for regex and build pattern matching the citation with optional anchor.
		escaped := regexp.QuoteMeta(candidate)
		// Match [source: <path>] and [source: <path>#anchor]
		pattern := fmt.Sprintf(`\[source:\s*%s(?:#[^\]]*?)?\]`, escaped)
		re := regexp.MustCompile("(?i)" + pattern)
		body = re.ReplaceAllString(body, "")
	}

	// Clean up leftover double spaces from removed citations.
	body = collapseSpaces(body)
	return body
}

// unwrapLinksToPath replaces [text](relative-link-to-wikiPath) with just text.
func unwrapLinksToPath(body, fromPath, wikiPath string) string {
	relLink := md.RelativeLink(fromPath, wikiPath)
	candidates := []string{relLink, wikiPath}

	for _, candidate := range candidates {
		escaped := regexp.QuoteMeta(candidate)
		// Match [text](path) and [text](path#anchor)
		pattern := fmt.Sprintf(`\[([^\]]*)\]\(%s(?:#[^\)]*)?\)`, escaped)
		re := regexp.MustCompile(pattern)
		body = re.ReplaceAllString(body, "$1")
	}

	return body
}

// collapseSpaces replaces runs of multiple spaces with a single space, and
// trims trailing whitespace on each line.
func collapseSpaces(body string) string {
	lines := strings.Split(body, "\n")
	multiSpace := regexp.MustCompile(`  +`)
	for i, line := range lines {
		lines[i] = strings.TrimRight(multiSpace.ReplaceAllString(line, " "), " ")
	}
	return strings.Join(lines, "\n")
}

// filterWikiLinks returns only paths starting with "wiki/".
func filterWikiLinks(links []string) []string {
	var out []string
	for _, link := range links {
		if strings.HasPrefix(link, "wiki/") {
			out = append(out, link)
		}
	}
	return mergePaths(out)
}

// mergePaths deduplicates and sorts paths.
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
	// Sort for determinism.
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

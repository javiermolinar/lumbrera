package deletecmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

// cleanEvidenceFromManaged removes one evidence path, strips matching inline
// citations, and regenerates policy-controlled metadata and the Sources section.
func cleanEvidenceFromManaged(ref managedRef, evidencePath string) (managedRef, error) {
	body := stripEvidenceCitations(ref.body, ref.relPath, evidencePath)
	evidence := removeFromSlice(ref.meta.Lumbrera.Sources, evidencePath)
	body = md.RemoveSourcesSection(body)

	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(ref.policy.Kind)})
	if err != nil {
		return ref, err
	}
	evidence = mergePaths(evidence, referencePaths(analysis.SourceCitations))
	if len(evidence) > 0 {
		body = md.AppendSourcesSection(body, ref.relPath, evidence)
	} else {
		body = strings.TrimRight(body, "\n") + "\n"
	}

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Sources = evidence
	updated.meta.Lumbrera.Links = filterManagedLinks(analysis.Links)
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, nil
}

// cleanOrdinaryLinkFromManaged unwraps ordinary Markdown links to targetPath.
func cleanOrdinaryLinkFromManaged(ref managedRef, targetPath string) (managedRef, error) {
	body := unwrapLinksToPath(ref.body, ref.relPath, targetPath)
	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(ref.policy.Kind)})
	if err != nil {
		return ref, err
	}

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Links = filterManagedLinks(analysis.Links)
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, nil
}

// cleanAssetFromManaged removes image embeds and regular links to an asset.
// Removing an asset never removes the managed document itself.
func cleanAssetFromManaged(ref managedRef, assetPath string) (managedRef, error) {
	body := stripAssetReferences(ref.body, ref.relPath, assetPath)
	analysis, err := md.AnalyzeWithOptions(ref.relPath, body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(ref.policy.Kind)})
	if err != nil {
		return ref, err
	}

	updated := ref
	updated.body = body
	updated.meta.Lumbrera.Links = filterManagedLinks(analysis.Links)
	updated.meta.Lumbrera.ModifiedDate = time.Now().Format("2006-01-02")
	return updated, nil
}

func stripAssetReferences(body, fromPath, assetPath string) string {
	relLink := md.RelativeLink(fromPath, assetPath)
	for _, candidate := range []string{relLink, assetPath} {
		escaped := regexp.QuoteMeta(candidate)
		imagePattern := fmt.Sprintf(`!\[[^\]]*\]\(%s(?:#[^\)]*)?\.?\)`, escaped)
		body = regexp.MustCompile(imagePattern).ReplaceAllString(body, "")

		linkPattern := fmt.Sprintf(`\[[^\]]*\]\(%s(?:#[^\)]*)?\.?\)`, escaped)
		body = regexp.MustCompile(linkPattern).ReplaceAllString(body, "")
	}
	return collapseSpaces(body)
}

func stripEvidenceCitations(body, fromPath, evidencePath string) string {
	relLink := md.RelativeLink(fromPath, evidencePath)
	for _, candidate := range []string{relLink, evidencePath} {
		escaped := regexp.QuoteMeta(candidate)
		pattern := fmt.Sprintf(`\[source:\s*%s(?:#[^\]]*?)?\]`, escaped)
		body = regexp.MustCompile("(?i)"+pattern).ReplaceAllString(body, "")
	}
	return collapseSpaces(body)
}

func unwrapLinksToPath(body, fromPath, targetPath string) string {
	relLink := md.RelativeLink(fromPath, targetPath)
	for _, candidate := range []string{relLink, targetPath} {
		escaped := regexp.QuoteMeta(candidate)
		pattern := fmt.Sprintf(`\[([^\]]*)\]\(%s(?:#[^\)]*)?\)`, escaped)
		body = regexp.MustCompile(pattern).ReplaceAllString(body, "$1")
	}
	return body
}

func collapseSpaces(body string) string {
	lines := strings.Split(body, "\n")
	multiSpace := regexp.MustCompile(`  +`)
	for i, line := range lines {
		lines[i] = strings.TrimRight(multiSpace.ReplaceAllString(line, " "), " ")
	}
	return strings.Join(lines, "\n")
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

func mergePaths(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
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

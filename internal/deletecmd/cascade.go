package deletecmd

import (
	"os"
	"strings"

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

// wikiRefsWithSource returns wiki refs whose frontmatter sources include sourcePath.
func wikiRefsWithSource(refs []wikiRef, sourcePath string) []wikiRef {
	var out []wikiRef
	for _, ref := range refs {
		for _, s := range ref.meta.Lumbrera.Sources {
			if s == sourcePath {
				out = append(out, ref)
				break
			}
		}
	}
	return out
}

// wikiRefsLinkingTo returns wiki refs whose frontmatter links include wikiPath.
func wikiRefsLinkingTo(refs []wikiRef, wikiPath string) []wikiRef {
	var out []wikiRef
	for _, ref := range refs {
		for _, link := range ref.meta.Lumbrera.Links {
			if link == wikiPath {
				out = append(out, ref)
				break
			}
		}
	}
	return out
}

// removeFromSlice returns a copy of values without any entries matching target.
func removeFromSlice(values []string, target string) []string {
	var out []string
	for _, v := range values {
		if v != target {
			out = append(out, v)
		}
	}
	return out
}

// containsString returns true if values contains target.
func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// isDeletedPath returns true if path is in the deleted set.
func isDeletedPath(deleted map[string]struct{}, path string) bool {
	_, ok := deleted[path]
	return ok
}

// wikiRefsLinkingToAsset returns wiki refs whose body contains image/link
// references to the given asset path.
func wikiRefsLinkingToAsset(refs []wikiRef, assetPath string) []wikiRef {
	var out []wikiRef
	for _, ref := range refs {
		relLink := md.RelativeLink(ref.relPath, assetPath)
		if strings.Contains(ref.body, assetPath) || strings.Contains(ref.body, relLink) {
			out = append(out, ref)
		}
	}
	return out
}

// sourcePathsFromBody extracts source paths from a wiki body's ## Sources section links.
func sourcePathsFromBody(refs []wikiRef, relPath string) []string {
	for _, ref := range refs {
		if ref.relPath == relPath {
			return ref.meta.Lumbrera.Sources
		}
	}
	return nil
}

// planCascade determines which files to delete and which wiki pages to update.
// It returns:
//   - filesToDelete: ordered list of files to remove
//   - wikiUpdates: map of wikiPath -> updated wikiRef (body/meta cleaned)
func planCascade(repo string, targetPath, targetKind string, allRefs []wikiRef) (filesToDelete []string, wikiUpdates map[string]wikiRef, err error) {
	filesToDelete = []string{targetPath}
	wikiUpdates = make(map[string]wikiRef)
	deleted := map[string]struct{}{targetPath: {}}

	if targetKind == "asset" {
		// Find all wiki pages referencing this asset in their body.
		affected := wikiRefsLinkingToAsset(allRefs, targetPath)
		for _, ref := range affected {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			base := ref
			if prev, ok := wikiUpdates[ref.relPath]; ok {
				base = prev
			}
			updated, err := cleanAssetFromWiki(base, targetPath)
			if err != nil {
				return nil, nil, err
			}
			wikiUpdates[updated.relPath] = updated
		}
		// Assets never cascade-delete wiki pages.
	}

	if targetKind == "source" {
		// Find all wiki pages referencing this source.
		affected := wikiRefsWithSource(allRefs, targetPath)
		for _, ref := range affected {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			updated, err := cleanSourceFromWiki(ref, targetPath)
			if err != nil {
				return nil, nil, err
			}
			if len(updated.meta.Lumbrera.Sources) == 0 {
				// No sources left — cascade delete this wiki page.
				filesToDelete = append(filesToDelete, updated.relPath)
				deleted[updated.relPath] = struct{}{}
			} else {
				wikiUpdates[updated.relPath] = updated
			}
		}
	}

	// For every deleted wiki page, clean links from other wiki pages.
	for _, path := range filesToDelete {
		if !strings.HasPrefix(path, "wiki/") {
			continue
		}
		linkers := wikiRefsLinkingTo(allRefs, path)
		for _, ref := range linkers {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			// If already updated in a prior pass, use that version.
			base := ref
			if prev, ok := wikiUpdates[ref.relPath]; ok {
				base = prev
			}
			updated, err := cleanWikiLinkFromWiki(base, path)
			if err != nil {
				return nil, nil, err
			}
			wikiUpdates[updated.relPath] = updated
		}
	}

	return filesToDelete, wikiUpdates, nil
}

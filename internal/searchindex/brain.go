package searchindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

const (
	SearchIndexRelPath             = ".brain/search.sqlite"
	CurrentIndexerVersion          = 1
	CurrentMarkdownSectionsVersion = 1
	manifestHashMetaKey            = "manifest_hash"
	indexedPathsHashMetaKey        = "indexed_paths_hash"
	indexerVersionMetaKey          = "indexer_version"
	markdownSectionsVersionMetaKey = "markdown_sections_version"
	manifestDebugMetaKey           = "manifest_debug"
)

type indexedFile struct {
	Path string
	Hash string
	Size int
}

// RebuildBrain rebuilds the disposable SQLite search cache for a Lumbrera brain
// repository from canonical Markdown files.
func RebuildBrain(ctx context.Context, repo string) error {
	if _, err := validateIndexDirectory(repo, ".brain", true); err != nil {
		return err
	}
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}

	documents, sections, links, citations, tags, metadata, err := RecordsForRepoWithFacts(repo)
	if err != nil {
		return err
	}

	return rebuildBrainAtomically(ctx, repo, documents, sections, links, citations, tags, metadata)
}

func SearchIndexPath(repo string) string {
	return filepath.Join(repo, filepath.FromSlash(SearchIndexRelPath))
}

func rebuildBrainAtomically(ctx context.Context, repo string, documents []Document, sections []Section, links []DocumentLink, citations []DocumentCitation, tags []DocumentTag, metadata map[string]string) error {
	brainDir := filepath.Join(repo, ".brain")
	tmpFile, err := os.CreateTemp(brainDir, "search.sqlite-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary search index: %w", err)
	}
	tmpPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		removeSQLiteFiles(tmpPath)
		return fmt.Errorf("close temporary search index: %w", err)
	}
	defer removeSQLiteFiles(tmpPath)

	db, err := OpenSQLite(tmpPath)
	if err != nil {
		return err
	}
	rebuildErr := RebuildRecordsWithFacts(ctx, db, documents, sections, links, citations, tags, metadata)
	closeErr := db.Close()
	if rebuildErr != nil {
		return rebuildErr
	}
	if closeErr != nil {
		return fmt.Errorf("close rebuilt search index: %w", closeErr)
	}

	finalPath := SearchIndexPath(repo)
	removeSQLiteSidecars(finalPath)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("replace search index: %w", err)
	}
	return nil
}

// RecordsForRepo extracts deterministic search records and manifest metadata
// from all indexed Markdown files in a Lumbrera brain repository.
func RecordsForRepo(repo string) ([]Document, []Section, map[string]string, error) {
	documents, sections, _, _, _, metadata, err := RecordsForRepoWithFacts(repo)
	return documents, sections, metadata, err
}

// RecordsForRepoWithFacts extracts deterministic search records, relationship
// facts, and manifest metadata from all indexed Markdown files in a Lumbrera
// brain repository.
func RecordsForRepoWithFacts(repo string) ([]Document, []Section, []DocumentLink, []DocumentCitation, []DocumentTag, map[string]string, error) {
	paths, err := indexedMarkdownPaths(repo)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	documents := make([]Document, 0, len(paths))
	var sections []Section
	var links []DocumentLink
	var citations []DocumentCitation
	var tags []DocumentTag
	files := make([]indexedFile, 0, len(paths))
	for _, relPath := range paths {
		content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(relPath)))
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("read indexed Markdown file %s: %w", relPath, err)
		}
		doc, docSections, docLinks, docCitations, docTags, err := ExtractMarkdownRecordsWithFacts(relPath, content)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		documents = append(documents, doc)
		sections = append(sections, docSections...)
		links = append(links, docLinks...)
		citations = append(citations, docCitations...)
		tags = append(tags, docTags...)
		files = append(files, indexedFile{Path: relPath, Hash: contentHash(content), Size: len(content)})
	}
	metadata := manifestMetadata(files)
	return documents, sections, links, citations, tags, metadata, nil
}

func indexedMarkdownPaths(repo string) ([]string, error) {
	var paths []string
	for _, root := range []string{"sources", "wiki"} {
		exists, err := validateIndexDirectory(repo, root, false)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		absRoot := filepath.Join(repo, filepath.FromSlash(root))
		err = filepath.WalkDir(absRoot, func(absPath string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				rel, relErr := filepath.Rel(repo, absPath)
				if relErr != nil {
					return relErr
				}
				return fmt.Errorf("%s is not a regular Markdown file", filepath.ToSlash(rel))
			}
			rel, err := filepath.Rel(repo, absPath)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func manifestMetadata(files []indexedFile) map[string]string {
	manifest := canonicalManifest(files)
	pathsInput := canonicalIndexedPaths(files)
	return map[string]string{
		manifestHashMetaKey:            contentHash([]byte(manifest)),
		indexedPathsHashMetaKey:        contentHash([]byte(pathsInput)),
		indexerVersionMetaKey:          strconv.Itoa(CurrentIndexerVersion),
		markdownSectionsVersionMetaKey: strconv.Itoa(CurrentMarkdownSectionsVersion),
		manifestDebugMetaKey:           manifest,
	}
}

func canonicalManifest(files []indexedFile) string {
	files = sortedIndexedFiles(files)
	var b strings.Builder
	fmt.Fprintf(&b, "schema_version=%d\n", CurrentSchemaVersion)
	fmt.Fprintf(&b, "indexer_version=%d\n", CurrentIndexerVersion)
	fmt.Fprintf(&b, "markdown_sections_version=%d\n", CurrentMarkdownSectionsVersion)
	for _, file := range files {
		fmt.Fprintf(&b, "path=%s hash=%s size=%d\n", file.Path, file.Hash, file.Size)
	}
	return b.String()
}

func canonicalIndexedPaths(files []indexedFile) string {
	files = sortedIndexedFiles(files)
	var b strings.Builder
	for _, file := range files {
		b.WriteString(file.Path)
		b.WriteByte('\n')
	}
	return b.String()
}

func sortedIndexedFiles(files []indexedFile) []indexedFile {
	out := append([]indexedFile(nil), files...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func validateIndexDirectory(repo, rel string, required bool) (bool, error) {
	absPath := filepath.Join(repo, filepath.FromSlash(rel))
	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%s must be a real directory, not a symlink", rel)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s must be a directory", rel)
	}
	return true, nil
}

func removeSQLiteFiles(path string) {
	_ = os.Remove(path)
	removeSQLiteSidecars(path)
}

func removeSQLiteSidecars(path string) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

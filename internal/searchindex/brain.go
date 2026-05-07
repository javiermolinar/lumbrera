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
	"github.com/javiermolinar/lumbrera/internal/brainfs"
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
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		return fmt.Errorf("create .brain cache directory: %w", err)
	}
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
	markdownFiles, err := brainfs.ReadMarkdownFiles(repo, []string{"sources", "wiki"})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	documents := make([]Document, 0, len(markdownFiles))
	var sections []Section
	var links []DocumentLink
	var citations []DocumentCitation
	var tags []DocumentTag
	files := make([]indexedFile, 0, len(markdownFiles))
	for _, file := range markdownFiles {
		doc, docSections, docLinks, docCitations, docTags, err := ExtractMarkdownRecordsWithFacts(file.RelPath, file.Content)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		documents = append(documents, doc)
		sections = append(sections, docSections...)
		links = append(links, docLinks...)
		citations = append(citations, docCitations...)
		tags = append(tags, docTags...)
		files = append(files, indexedFile{Path: file.RelPath, Hash: contentHash(file.Content), Size: len(file.Content)})
	}
	metadata := manifestMetadata(files)
	return documents, sections, links, citations, tags, metadata, nil
}

func indexedMarkdownPaths(repo string) ([]string, error) {
	return brainfs.MarkdownPaths(repo, []string{"sources", "wiki"})
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

func removeSQLiteFiles(path string) {
	_ = os.Remove(path)
	removeSQLiteSidecars(path)
}

func removeSQLiteSidecars(path string) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

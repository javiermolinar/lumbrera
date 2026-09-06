package generate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/manifest"
)

type Files struct {
	Index        string
	SourcesIndex string
	NotesIndex   string
	AssetsIndex  string
	BrainSum     string
	Tags         string
}

func FilesForRepo(repo string) (Files, error) {
	index, err := IndexForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	sourcesIndex, err := SourcesIndexForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	notesIndex, err := NotesIndexForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	assetsIndex, err := AssetsIndexForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	brainSum, err := manifest.ForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	tags, err := TagsForRepo(repo)
	if err != nil {
		return Files{}, err
	}
	return Files{
		Index:        index,
		SourcesIndex: sourcesIndex,
		NotesIndex:   notesIndex,
		AssetsIndex:  assetsIndex,
		BrainSum:     brainSum,
		Tags:         tags,
	}, nil
}

func WriteFiles(repo string, files Files) error {
	catalogs := []struct {
		kind    brain.Kind
		content string
	}{
		{kind: brain.KindWiki, content: files.Index},
		{kind: brain.KindSource, content: files.SourcesIndex},
		{kind: brain.KindNote, content: files.NotesIndex},
		{kind: brain.KindAsset, content: files.AssetsIndex},
	}
	for _, catalog := range catalogs {
		path, ok := brain.CatalogPathForKind(catalog.kind)
		if !ok {
			return fmt.Errorf("content kind %q has no catalog path", catalog.kind)
		}
		if err := os.WriteFile(filepath.Join(repo, path), []byte(catalog.content), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(repo, brain.BrainSumPath), []byte(files.BrainSum), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo, brain.TagsPath), []byte(files.Tags), 0o644)
}

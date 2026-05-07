package generate

import (
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/manifest"
)

type Files struct {
	Index    string
	BrainSum string
	Tags     string
}

func FilesForRepo(repo string) (Files, error) {
	index, err := IndexForRepo(repo)
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
	return Files{Index: index, BrainSum: brainSum, Tags: tags}, nil
}

func WriteFiles(repo string, files Files) error {
	if err := os.WriteFile(filepath.Join(repo, brain.IndexPath), []byte(files.Index), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, brain.BrainSumPath), []byte(files.BrainSum), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo, brain.TagsPath), []byte(files.Tags), 0o644)
}

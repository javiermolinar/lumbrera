package verify

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/generate"
)

type Options struct{}

// Run performs the user-facing verify command behavior, including the legacy
// repair step for wiki documents that predate generated IDs.
func Run(repo string, opts Options) error {
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}
	if err := ValidatePathPolicy(repo); err != nil {
		return err
	}
	repaired, err := RepairMissingIDs(repo)
	if err != nil {
		return err
	}
	if repaired {
		files, err := generate.FilesForRepo(repo)
		if err != nil {
			return err
		}
		if err := generate.WriteFiles(repo, files); err != nil {
			return err
		}
	}
	return Check(repo, opts)
}

// Check validates deterministic brain integrity without mutating canonical
// files. Commands that only need a precondition should call Check instead of
// Run.
func Check(repo string, opts Options) error {
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

func VerifyGeneratedFiles(repo string) error {
	files, err := generate.FilesForRepo(repo)
	if err != nil {
		return err
	}
	checks := map[string]string{
		brain.IndexPath:     files.Index,
		brain.ChangelogPath: files.Changelog,
		brain.BrainSumPath:  files.BrainSum,
		brain.TagsPath:      files.Tags,
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

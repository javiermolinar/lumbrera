package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/generate"
)

type Options struct {
	Fix bool
}

// Run performs the user-facing verify command behavior, including the legacy
// repair step for wiki documents that predate generated IDs.
// When opts.Fix is true, stale generated files are regenerated in place.
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
	if repaired || opts.Fix {
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
		brain.IndexPath:    files.Index,
		brain.BrainSumPath: files.BrainSum,
		brain.TagsPath:     files.Tags,
	}
	for rel, want := range checks {
		got, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("generated file %s is missing: %w", rel, err)
		}
		if string(got) != want {
			diff := staleDiff(want, string(got), 5)
			return fmt.Errorf("generated file %s is stale:%s\nRegenerate through lumbrera write, or run lumbrera verify --fix", rel, diff)
		}
	}

	// CHANGELOG.md is the source of truth (append-only), not generated.
	// Verify it round-trips cleanly: parse → render must match on-disk.
	changelogWant, err := generate.ChangelogForRepo(repo)
	if err != nil {
		return fmt.Errorf("%s is malformed: %w", brain.ChangelogPath, err)
	}
	changelogGot, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(brain.ChangelogPath)))
	if err != nil {
		return fmt.Errorf("%s is missing: %w", brain.ChangelogPath, err)
	}
	if string(changelogGot) != changelogWant {
		diff := staleDiff(changelogWant, string(changelogGot), 5)
		return fmt.Errorf("%s has been hand-edited and does not match expected format:%s", brain.ChangelogPath, diff)
	}

	return nil
}

// staleDiff returns a short summary of the first line-level differences
// between the expected and actual content of a generated file.
func staleDiff(want, got string, max int) string {
	wantLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	n := len(wantLines)
	if len(gotLines) > n {
		n = len(gotLines)
	}

	var diffs []string
	for i := 0; i < n && len(diffs) < max; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w == g {
			continue
		}
		if g == "" {
			diffs = append(diffs, fmt.Sprintf("  line %d: missing %q", i+1, w))
		} else if w == "" {
			diffs = append(diffs, fmt.Sprintf("  line %d: unexpected %q", i+1, g))
		} else {
			diffs = append(diffs, fmt.Sprintf("  line %d: expected %q, got %q", i+1, w, g))
		}
	}

	if len(diffs) == 0 {
		return ""
	}
	return "\n" + strings.Join(diffs, "\n")
}

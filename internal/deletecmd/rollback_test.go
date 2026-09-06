package deletecmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/braintest"
)

func TestDeleteBackupRestoresEveryTouchedKind(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nEvidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Observation\n\nObserved behavior.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "An observed behavior.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")
	braintest.RunWrite(t, repo, "# Guidance\n\nGuidance.\n", "wiki/guidance.md",
		"--title", "Guidance", "--summary", "Canonical guidance.", "--tag", "operations", "--source", "notes/observation.md",
		"--reason", "Create guidance", "--actor", "test")
	assetSource := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(assetSource, []byte("asset bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	braintest.RunAssetWrite(t, repo, "assets/diagram.png", assetSource, "--reason", "Add diagram", "--actor", "test")

	refs, err := loadManagedRefs(repo)
	if err != nil {
		t.Fatal(err)
	}
	updates := map[string]managedRef{}
	for _, ref := range refs {
		updates[ref.relPath] = ref
	}
	contentPaths := []string{"sources/raw.md", "notes/observation.md", "wiki/guidance.md", "assets/diagram.png"}
	if err := os.Chmod(filepath.Join(repo, "assets", "diagram.png"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := newDeleteBackup(repo, contentPaths, updates)
	if err != nil {
		t.Fatal(err)
	}

	allPaths := append([]string{}, contentPaths...)
	allPaths = append(allPaths, brain.GeneratedFilePaths()...)
	allPaths = append(allPaths, brain.ChangelogPath)
	before := map[string][]byte{}
	beforeModes := map[string]os.FileMode{}
	for _, rel := range allPaths {
		abs := filepath.Join(repo, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("read %s before simulated failure: %v", rel, err)
		}
		before[rel] = content
		info, err := os.Stat(abs)
		if err != nil {
			t.Fatalf("stat %s before simulated failure: %v", rel, err)
		}
		beforeModes[rel] = info.Mode().Perm()
		if err := os.Remove(abs); err != nil {
			t.Fatalf("remove %s during simulated mutation: %v", rel, err)
		}
	}

	if err := backup.Restore(); err != nil {
		t.Fatalf("restore delete backup: %v", err)
	}
	for _, rel := range allPaths {
		got, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read restored %s: %v", rel, err)
		}
		if string(got) != string(before[rel]) {
			t.Fatalf("restored %s differs", rel)
		}
		info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("stat restored %s: %v", rel, err)
		}
		if info.Mode().Perm() != beforeModes[rel] {
			t.Fatalf("restored %s mode = %o, want %o", rel, info.Mode().Perm(), beforeModes[rel])
		}
	}
}

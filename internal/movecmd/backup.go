package movecmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

type fileBackup struct {
	path    string
	exists  bool
	content []byte
}

type moveBackup struct {
	files []fileBackup
}

func newMoveBackup(brainDir, from, to string, wikiUpdates map[string]wikiRef) (*moveBackup, error) {
	seen := map[string]struct{}{}
	var paths []string

	add := func(rel string) {
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		paths = append(paths, rel)
	}

	add(from)
	add(to)
	for p := range wikiUpdates {
		add(p)
	}
	for _, p := range brain.GeneratedFilePaths() {
		add(p)
	}
	add(brain.ChangelogPath)

	backup := &moveBackup{}
	for _, rel := range paths {
		abs := filepath.Join(brainDir, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err == nil {
			backup.files = append(backup.files, fileBackup{path: abs, exists: true, content: content})
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			backup.files = append(backup.files, fileBackup{path: abs})
			continue
		}
		return nil, err
	}
	return backup, nil
}

func (b *moveBackup) Restore() error {
	if b == nil {
		return nil
	}
	for _, file := range b.files {
		if file.exists {
			if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(file.path, file.content, 0o644); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

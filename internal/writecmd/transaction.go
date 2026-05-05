package writecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func preflight(brainDir string) error {
	if err := brain.ValidateRepo(brainDir); err != nil {
		return err
	}
	return verify.Run(brainDir, verify.Options{})
}

type fileBackup struct {
	path    string
	exists  bool
	content []byte
}

type writeBackup struct {
	files []fileBackup
}

func newWriteBackup(brainDir, target string) (*writeBackup, error) {
	paths := []string{
		target,
		brain.IndexPath,
		brain.ChangelogPath,
		brain.BrainSumPath,
		brain.TagsPath,
		ops.LogPath,
	}
	seen := map[string]struct{}{}
	backup := &writeBackup{}
	for _, rel := range paths {
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
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

func (b *writeBackup) Restore() error {
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

func defaultActor(brainDir string) (string, error) {
	_ = brainDir
	for _, key := range []string{"LUMBRERA_ACTOR", "USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return sanitizeActor(value), nil
		}
	}
	return "human", nil
}

func validateCommitSubject(actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.ContainsAny(reason, "\r\n") {
		return fmt.Errorf("--reason must be a single line")
	}
	if actor == "" {
		return nil
	}
	if strings.ContainsAny(actor, "]\r\n") {
		return fmt.Errorf("--actor must not contain ], carriage returns, or newlines")
	}
	return nil
}

func sanitizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	actor = strings.ReplaceAll(actor, "]", "")
	actor = strings.ReplaceAll(actor, "\n", " ")
	actor = strings.ReplaceAll(actor, "\r", " ")
	if actor == "" {
		return "human"
	}
	return actor
}

package migratecmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

type backupKind uint8

const (
	backupAbsent backupKind = iota
	backupRegular
	backupDirectory
	backupSymlink
)

type migrationBackupEntry struct {
	path       string
	kind       backupKind
	mode       os.FileMode
	content    []byte
	linkTarget string
}

type migrationBackup struct {
	files []migrationBackupEntry
	dirs  []migrationBackupEntry
}

func newMigrationBackup(repo string) (*migrationBackup, error) {
	filePaths := []string{
		brain.MarkerPath,
		brain.IndexPath,
		brain.SourcesIndexPath,
		brain.NotesIndexPath,
		brain.AssetsIndexPath,
		brain.BrainSumPath,
		brain.TagsPath,
		brain.ChangelogPath,
	}
	legacyManagedPaths, err := brainfs.MarkdownPaths(repo, []string{"wiki", "notes"})
	if err != nil {
		return nil, err
	}
	filePaths = append(filePaths, legacyManagedPaths...)
	for _, template := range initcmd.MigrationTemplates() {
		filePaths = append(filePaths, template.Path)
	}
	noteSkillPath, _ := initcmd.NoteSkillTemplate()
	filePaths = append(filePaths, noteSkillPath)
	indexPath := searchindex.SearchIndexPath(repo)
	for _, path := range []string{indexPath, indexPath + "-journal", indexPath + "-wal", indexPath + "-shm"} {
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return nil, err
		}
		filePaths = append(filePaths, filepath.ToSlash(rel))
	}

	dirPaths := []string{
		"assets",
		"notes",
		".agents",
		".agents/skills",
		filepath.ToSlash(filepath.Dir(noteSkillPath)),
	}

	backup := &migrationBackup{}
	seen := map[string]struct{}{}
	for _, rel := range filePaths {
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		entry, err := captureMigrationPath(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		backup.files = append(backup.files, entry)
	}
	for _, rel := range dirPaths {
		entry, err := captureMigrationPath(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		backup.dirs = append(backup.dirs, entry)
	}
	return backup, nil
}

func captureMigrationPath(path string) (migrationBackupEntry, error) {
	entry := migrationBackupEntry{path: path}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		entry.kind = backupAbsent
		return entry, nil
	}
	if err != nil {
		return migrationBackupEntry{}, err
	}
	entry.mode = info.Mode()
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		entry.kind = backupSymlink
		entry.linkTarget, err = os.Readlink(path)
		return entry, err
	case info.IsDir():
		entry.kind = backupDirectory
		return entry, nil
	case info.Mode().IsRegular():
		entry.kind = backupRegular
		entry.content, err = os.ReadFile(path)
		return entry, err
	default:
		return migrationBackupEntry{}, errors.New("migration cannot back up non-regular path " + path)
	}
}

func (backup *migrationBackup) Restore() error {
	if backup == nil {
		return nil
	}
	for i := len(backup.files) - 1; i >= 0; i-- {
		if err := restoreMigrationPath(backup.files[i]); err != nil {
			return err
		}
	}
	for i := len(backup.dirs) - 1; i >= 0; i-- {
		entry := backup.dirs[i]
		if entry.kind == backupAbsent {
			if err := os.RemoveAll(entry.path); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreMigrationPath(entry migrationBackupEntry) error {
	switch entry.kind {
	case backupAbsent:
		return os.RemoveAll(entry.path)
	case backupDirectory:
		return os.MkdirAll(entry.path, entry.mode.Perm())
	case backupRegular:
		if err := os.RemoveAll(entry.path); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(entry.path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(entry.path, entry.content, entry.mode.Perm())
	case backupSymlink:
		if err := os.RemoveAll(entry.path); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(entry.path), 0o755); err != nil {
			return err
		}
		return os.Symlink(entry.linkTarget, entry.path)
	default:
		return errors.New("migration backup has unknown path kind")
	}
}

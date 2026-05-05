package searchindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

type StatusState string

const (
	StatusFresh        StatusState = "fresh"
	StatusMissing      StatusState = "missing"
	StatusStale        StatusState = "stale"
	StatusIncompatible StatusState = "incompatible"
)

type Status struct {
	State           StatusState
	Path            string
	Exists          bool
	SchemaVersion   int
	ExpectedVersion int
	ManifestHash    string
	ExpectedHash    string
	Reason          string
	ManifestDebug   string
}

func CheckStatus(ctx context.Context, repo string) (Status, error) {
	status := Status{
		Path:            SearchIndexPath(repo),
		ExpectedVersion: CurrentSchemaVersion,
	}
	if err := validateBrainForStatus(repo); err != nil {
		return status, err
	}

	expectedMetadata, err := ManifestMetadataForRepo(repo)
	if err != nil {
		return status, err
	}
	status.ExpectedHash = expectedMetadata[manifestHashMetaKey]

	info, err := os.Lstat(status.Path)
	if err != nil {
		if os.IsNotExist(err) {
			status.State = StatusMissing
			status.Reason = "search index does not exist"
			return status, nil
		}
		return status, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		status.State = StatusIncompatible
		status.Exists = true
		status.Reason = "search index path is a symlink"
		return status, nil
	}
	if !info.Mode().IsRegular() {
		status.State = StatusIncompatible
		status.Exists = true
		status.Reason = "search index path is not a regular file"
		return status, nil
	}
	status.Exists = true

	db, err := OpenSQLite(status.Path)
	if err != nil {
		status.State = StatusIncompatible
		status.Reason = err.Error()
		return status, nil
	}
	defer db.Close()

	version, exists, err := ReadSchemaVersion(ctx, db)
	if err != nil {
		status.State = StatusIncompatible
		status.Reason = err.Error()
		return status, nil
	}
	if !exists {
		status.State = StatusIncompatible
		status.Reason = "search index schema version is missing"
		return status, nil
	}
	status.SchemaVersion = version
	if version != CurrentSchemaVersion {
		status.State = StatusIncompatible
		status.Reason = fmt.Sprintf("schema version %d does not match expected %d", version, CurrentSchemaVersion)
		return status, nil
	}

	meta, err := readMetaMap(ctx, db)
	if err != nil {
		status.State = StatusIncompatible
		status.Reason = err.Error()
		return status, nil
	}
	status.ManifestHash = meta[manifestHashMetaKey]
	status.ManifestDebug = meta[manifestDebugMetaKey]
	if reason := staleReason(meta, expectedMetadata); reason != "" {
		status.State = StatusStale
		status.Reason = reason
		return status, nil
	}

	status.State = StatusFresh
	status.Reason = "search index is fresh"
	return status, nil
}

func ManifestMetadataForRepo(repo string) (map[string]string, error) {
	files, err := indexedFilesForRepo(repo)
	if err != nil {
		return nil, err
	}
	return manifestMetadata(files), nil
}

func validateBrainForStatus(repo string) error {
	if _, err := validateIndexDirectory(repo, ".brain", true); err != nil {
		return err
	}
	return brain.ValidateRepo(repo)
}

func indexedFilesForRepo(repo string) ([]indexedFile, error) {
	paths, err := indexedMarkdownPaths(repo)
	if err != nil {
		return nil, err
	}
	files := make([]indexedFile, 0, len(paths))
	for _, relPath := range paths {
		content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(relPath)))
		if err != nil {
			return nil, fmt.Errorf("read indexed Markdown file %s: %w", relPath, err)
		}
		files = append(files, indexedFile{Path: relPath, Hash: contentHash(content), Size: len(content)})
	}
	return files, nil
}

func readMetaMap(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		if isMissingMetaTable(err) {
			return nil, errors.New("search index meta table is missing")
		}
		return nil, fmt.Errorf("read search index metadata: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan search index metadata: %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search index metadata: %w", err)
	}
	return out, nil
}

func staleReason(actual, expected map[string]string) string {
	for _, key := range []string{manifestHashMetaKey, indexedPathsHashMetaKey, indexerVersionMetaKey, markdownSectionsVersionMetaKey} {
		if actual[key] == "" {
			return fmt.Sprintf("metadata %s is missing", key)
		}
		if actual[key] != expected[key] {
			return fmt.Sprintf("metadata %s differs", key)
		}
	}
	if actualSchema := actual["schema_version"]; actualSchema != strconv.Itoa(CurrentSchemaVersion) {
		return "schema version metadata differs"
	}
	return ""
}

package searchindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

const modifiedDateLayout = "2006-01-02"

// RepairMissingModifiedDates adds generated wiki modified_date frontmatter to
// older wiki pages that predate the field. It intentionally repairs only the
// missing-field compatibility case; invalid frontmatter or invalid dates remain
// errors for the caller to surface.
func RepairMissingModifiedDates(repo, modifiedDate string) (bool, error) {
	if _, err := time.Parse(modifiedDateLayout, modifiedDate); err != nil {
		return false, fmt.Errorf("modified date %q must use YYYY-MM-DD: %w", modifiedDate, err)
	}

	root := filepath.Join(repo, "wiki")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	repaired := false
	err := filepath.WalkDir(root, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.Split(content)
		if err != nil {
			rel, relErr := filepath.Rel(repo, absPath)
			if relErr != nil {
				return relErr
			}
			return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", filepath.ToSlash(rel), err)
		}
		if !has || meta.Lumbrera.Kind != KindWiki || strings.TrimSpace(meta.Lumbrera.ModifiedDate) != "" {
			return nil
		}
		meta.Lumbrera.ModifiedDate = modifiedDate
		updated, err := frontmatter.Attach(meta, body)
		if err != nil {
			return err
		}
		if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
			return err
		}
		repaired = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return repaired, nil
}

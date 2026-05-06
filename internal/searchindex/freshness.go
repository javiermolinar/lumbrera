package searchindex

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brainfs"
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

	repaired := false
	err := brainfs.WalkMarkdown(repo, []string{"wiki"}, func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.Split(content)
		if err != nil {
			return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", file.RelPath, err)
		}
		if !has || meta.Lumbrera.Kind != KindWiki || strings.TrimSpace(meta.Lumbrera.ModifiedDate) != "" {
			return nil
		}
		meta.Lumbrera.ModifiedDate = modifiedDate
		updated, err := frontmatter.Attach(meta, body)
		if err != nil {
			return err
		}
		if err := os.WriteFile(file.AbsPath, []byte(updated), 0o644); err != nil {
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

package searchindex

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

const modifiedDateLayout = "2006-01-02"

// RepairMissingModifiedDates adds generated managed-document modified_date
// frontmatter to older pages that predate the field. It intentionally repairs only the
// missing-field compatibility case; invalid frontmatter or invalid dates remain
// errors for the caller to surface.
func RepairMissingModifiedDates(repo, modifiedDate string) (bool, error) {
	if _, err := time.Parse(modifiedDateLayout, modifiedDate); err != nil {
		return false, fmt.Errorf("modified date %q must use YYYY-MM-DD: %w", modifiedDate, err)
	}

	repaired := false
	err := brainfs.WalkMarkdown(repo, brain.ManagedRoots(), func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		policy, ok := brain.PolicyForPath(file.RelPath)
		if !ok || policy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("%s does not resolve to a managed content policy", file.RelPath)
		}
		meta, body, has, err := frontmatter.SplitForPolicy(content, policy)
		if err != nil {
			return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", file.RelPath, err)
		}
		if !has || strings.TrimSpace(meta.Lumbrera.ModifiedDate) != "" {
			return nil
		}
		meta.Lumbrera.ModifiedDate = modifiedDate
		updated, err := frontmatter.AttachForPolicy(meta, policy, body)
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

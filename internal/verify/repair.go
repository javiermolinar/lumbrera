package verify

import (
	"fmt"
	"os"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func RepairMissingIDs(repo string) (bool, error) {
	repaired := false
	err := brainfs.WalkMarkdown(repo, brain.ManagedRoots(), func(file brainfs.MarkdownFile) error {
		policy, ok := brain.PolicyForPath(file.RelPath)
		if !ok || policy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("%s does not resolve to a managed content policy", file.RelPath)
		}
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.SplitForPolicyWithOptions(content, policy, frontmatter.SplitOptions{AllowMissingID: true})
		if err != nil {
			return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", file.RelPath, err)
		}
		if !has || meta.Lumbrera.ID != "" {
			return nil
		}
		id, err := frontmatter.NewID()
		if err != nil {
			return err
		}
		meta.Lumbrera.ID = id
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

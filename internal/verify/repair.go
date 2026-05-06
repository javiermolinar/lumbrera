package verify

import (
	"fmt"
	"os"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func RepairMissingIDs(repo string) (bool, error) {
	repaired := false
	err := brainfs.WalkMarkdown(repo, []string{"wiki"}, func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.SplitWithOptions(content, frontmatter.SplitOptions{AllowMissingID: true})
		if err != nil {
			return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", file.RelPath, err)
		}
		if !has || meta.Lumbrera.Kind != "wiki" || strings.TrimSpace(meta.Lumbrera.ID) != "" {
			return nil
		}
		id, err := frontmatter.NewID()
		if err != nil {
			return err
		}
		meta.Lumbrera.ID = id
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

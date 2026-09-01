package generate

import (
	"fmt"
	"os"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func walkWikiMetadata(repo string, visit func(rel string, meta frontmatter.Document) error) error {
	policy, ok := brain.PolicyForKind(brain.KindWiki)
	if !ok {
		return fmt.Errorf("missing wiki content policy")
	}
	return brainfs.WalkMarkdown(repo, []string{policy.Root}, func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, _, hasFrontmatter, err := frontmatter.Split(content)
		if err != nil {
			return fmt.Errorf("%s has invalid frontmatter: %w", file.RelPath, err)
		}
		if !hasFrontmatter {
			return nil
		}
		return visit(file.RelPath, meta)
	})
}

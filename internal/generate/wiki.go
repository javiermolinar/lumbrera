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
	return walkManagedMetadata(repo, []brain.ContentPolicy{policy}, visit)
}

func walkManagedMetadata(repo string, policies []brain.ContentPolicy, visit func(rel string, meta frontmatter.Document) error) error {
	roots := make([]string, 0, len(policies))
	byRoot := make(map[string]brain.ContentPolicy, len(policies))
	for _, policy := range policies {
		if policy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("content policy %q is not managed Markdown", policy.Kind)
		}
		roots = append(roots, policy.Root)
		byRoot[policy.Root] = policy
	}
	return brainfs.WalkMarkdown(repo, roots, func(file brainfs.MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		policy, ok := brain.PolicyForPath(file.RelPath)
		if !ok {
			return fmt.Errorf("%s does not resolve to a content policy", file.RelPath)
		}
		if expected, exists := byRoot[policy.Root]; !exists || expected.Kind != policy.Kind {
			return fmt.Errorf("%s resolved outside the requested managed policies", file.RelPath)
		}
		meta, _, hasFrontmatter, err := frontmatter.SplitForPolicy(content, policy)
		if err != nil {
			return fmt.Errorf("%s has invalid frontmatter: %w", file.RelPath, err)
		}
		if !hasFrontmatter {
			return nil
		}
		return visit(file.RelPath, meta)
	})
}

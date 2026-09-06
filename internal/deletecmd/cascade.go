package deletecmd

import (
	"fmt"
	"os"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainfs"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

// managedRef holds one parsed managed document for cascade planning.
type managedRef struct {
	relPath string
	policy  brain.ContentPolicy
	meta    frontmatter.Document
	body    string
}

func loadManagedRefs(repo string) ([]managedRef, error) {
	var refs []managedRef
	err := brainfs.WalkMarkdown(repo, brain.ManagedRoots(), func(file brainfs.MarkdownFile) error {
		policy, ok := brain.PolicyForPath(file.RelPath)
		if !ok || policy.Storage != brain.StorageManagedMarkdown {
			return fmt.Errorf("%s does not resolve to a managed content policy", file.RelPath)
		}
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return err
		}
		meta, body, has, err := frontmatter.SplitForPolicy(content, policy)
		if err != nil {
			return fmt.Errorf("parse %s: %w", file.RelPath, err)
		}
		if !has {
			return fmt.Errorf("%s is missing Lumbrera-generated frontmatter", file.RelPath)
		}
		refs = append(refs, managedRef{relPath: file.RelPath, policy: policy, meta: meta, body: body})
		return nil
	})
	return refs, err
}

func refsUsingEvidence(refs []managedRef, evidencePath string) []managedRef {
	var out []managedRef
	for _, ref := range refs {
		if containsString(ref.meta.Lumbrera.Sources, evidencePath) {
			out = append(out, ref)
		}
	}
	return out
}

func refHasOrdinaryReference(ref managedRef, targetPath string) (bool, error) {
	analysis, err := md.AnalyzeWithOptions(ref.relPath, ref.body, md.AnalyzeOptions{SourceCitations: brain.AcceptsEvidence(ref.policy.Kind)})
	if err != nil {
		return false, err
	}
	for _, link := range analysis.LinkReferences {
		if link.Path == targetPath {
			return true, nil
		}
	}
	return false, nil
}

func removeFromSlice(values []string, target string) []string {
	var out []string
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isDeletedPath(deleted map[string]struct{}, path string) bool {
	_, ok := deleted[path]
	return ok
}

// planCascade calculates every delete and rewrite before the mutation starts.
func planCascade(repo string, targetPath, targetKind string, allRefs []managedRef) (filesToDelete []string, managedUpdates map[string]managedRef, err error) {
	_ = repo
	targetPolicy, ok := brain.PolicyForPath(targetPath)
	if !ok || string(targetPolicy.Kind) != targetKind {
		return nil, nil, fmt.Errorf("target path %s does not resolve to kind %q", targetPath, targetKind)
	}

	filesToDelete = []string{targetPath}
	managedUpdates = make(map[string]managedRef)
	deleted := map[string]struct{}{targetPath: {}}

	if targetPolicy.ProvidesEvidence {
		for _, ref := range refsUsingEvidence(allRefs, targetPath) {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			base := ref
			if previous, exists := managedUpdates[ref.relPath]; exists {
				base = previous
			}
			updated, cleanErr := cleanEvidenceFromManaged(base, targetPath)
			if cleanErr != nil {
				return nil, nil, cleanErr
			}
			if updated.policy.RequiresEvidence && len(updated.meta.Lumbrera.Sources) == 0 {
				filesToDelete = append(filesToDelete, updated.relPath)
				deleted[updated.relPath] = struct{}{}
				delete(managedUpdates, updated.relPath)
				continue
			}
			managedUpdates[updated.relPath] = updated
		}
	}

	if targetPolicy.Storage == brain.StorageBinary {
		for _, ref := range allRefs {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			base := ref
			if previous, exists := managedUpdates[ref.relPath]; exists {
				base = previous
			}
			references, analyzeErr := refHasOrdinaryReference(base, targetPath)
			if analyzeErr != nil {
				return nil, nil, analyzeErr
			}
			if !references {
				continue
			}
			updated, cleanErr := cleanAssetFromManaged(base, targetPath)
			if cleanErr != nil {
				return nil, nil, cleanErr
			}
			managedUpdates[updated.relPath] = updated
		}
		return filesToDelete, managedUpdates, nil
	}

	// Remove ordinary inbound links for the explicit target and every managed
	// document deleted by an evidence cascade.
	for _, deletedPath := range filesToDelete {
		for _, ref := range allRefs {
			if isDeletedPath(deleted, ref.relPath) {
				continue
			}
			base := ref
			if previous, exists := managedUpdates[ref.relPath]; exists {
				base = previous
			}
			references, analyzeErr := refHasOrdinaryReference(base, deletedPath)
			if analyzeErr != nil {
				return nil, nil, analyzeErr
			}
			if !references {
				continue
			}
			updated, cleanErr := cleanOrdinaryLinkFromManaged(base, deletedPath)
			if cleanErr != nil {
				return nil, nil, cleanErr
			}
			managedUpdates[updated.relPath] = updated
		}
	}

	return filesToDelete, managedUpdates, nil
}

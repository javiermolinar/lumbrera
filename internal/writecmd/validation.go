package writecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
)

func validateOptionsForOperation(repo, target string, policy brain.ContentPolicy, exists bool, op operation, opts options) error {
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	if op == opDelete {
		if !exists {
			return fmt.Errorf("cannot delete %s: file does not exist", target)
		}
		if policy.Storage == brain.StorageRawMarkdown {
			return fmt.Errorf("%s documents are immutable; refusing to delete existing document", policy.Kind)
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 || len(opts.Sources) > 0 {
			return fmt.Errorf("--delete cannot be combined with --title, --summary, --tag, or --source")
		}
		return nil
	}

	switch policy.Storage {
	case brain.StorageRawMarkdown:
		if len(opts.Sources) > 0 {
			return fmt.Errorf("%s writes must not specify --source", policy.Kind)
		}
		if op != opSource {
			return fmt.Errorf("%s documents are immutable; refusing to mutate existing document", policy.Kind)
		}
		return nil
	case brain.StorageBinary:
		if strings.TrimSpace(opts.File) == "" {
			return fmt.Errorf("%s writes require --file", policy.Kind)
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 || len(opts.Sources) > 0 || opts.AppendSet {
			return fmt.Errorf("%s writes cannot use --title, --summary, --tag, --source, or --append", policy.Kind)
		}
		if op != opAsset {
			return fmt.Errorf("%s documents are immutable; refusing to mutate existing document", policy.Kind)
		}
		return nil
	case brain.StorageManagedMarkdown:
		if !policy.Mutable {
			return fmt.Errorf("%s documents are not mutable", policy.Kind)
		}
	default:
		return fmt.Errorf("unsupported storage policy for kind %q", policy.Kind)
	}

	if len(opts.Sources) > 0 {
		if !brain.AcceptsEvidence(policy.Kind) {
			return fmt.Errorf("%s writes must not specify --source", policy.Kind)
		}
		if _, err := normalizeAndValidateEvidencePaths(repo, target, policy, opts.Sources, "--source"); err != nil {
			return err
		}
	}
	if len(opts.Tags) > 0 {
		if err := frontmatter.ValidateTagsForPolicy(opts.Tags, policy); err != nil {
			return err
		}
	}

	if op == opCreate {
		if strings.TrimSpace(opts.Title) == "" {
			return fmt.Errorf("--title is required when creating a new %s file", policy.Kind)
		}
		if strings.TrimSpace(opts.Summary) == "" {
			return fmt.Errorf("--summary is required when creating a new %s file", policy.Kind)
		}
		if len(opts.Tags) == 0 {
			return fmt.Errorf("at least one --tag is required when creating a new %s file", policy.Kind)
		}
	}
	if op == opAppend {
		if !exists {
			return fmt.Errorf("cannot append to %s: file does not exist", target)
		}
		section := strings.TrimSpace(opts.Append)
		if section == "" {
			return fmt.Errorf("--append requires a non-empty section name")
		}
		if strings.EqualFold(section, "Sources") {
			return fmt.Errorf("--append cannot target the managed Sources section")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 {
			return fmt.Errorf("--append cannot change --title, --summary, or --tag in this version")
		}
	}
	return nil
}

func normalizeAndValidateEvidencePaths(repo, target string, documentPolicy brain.ContentPolicy, evidencePaths []string, label string) ([]string, error) {
	normalizedPaths := make([]string, 0, len(evidencePaths))
	for _, evidencePath := range evidencePaths {
		normalized, kind, err := normalizeTargetPath(evidencePath)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", label, evidencePath, err)
		}
		evidencePolicy, ok := brain.PolicyForKind(brain.Kind(kind))
		if !ok || !brain.CanUseAsEvidence(documentPolicy.Kind, evidencePolicy.Kind) {
			return nil, fmt.Errorf("%s %q cannot be used as evidence for kind %q", label, evidencePath, documentPolicy.Kind)
		}
		if normalized == target {
			return nil, fmt.Errorf("%s %q must not reference the target document itself", label, evidencePath)
		}
		if err := ensureSafeFilesystemTarget(repo, normalized); err != nil {
			return nil, fmt.Errorf("%s %q is unsafe: %w", label, evidencePath, err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(normalized))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%s %q does not exist", label, evidencePath)
			}
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s %q must be a regular Markdown file", label, evidencePath)
		}
		normalizedPaths = append(normalizedPaths, normalized)
	}
	return mergePaths(normalizedPaths), nil
}

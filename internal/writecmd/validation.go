package writecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/verify"
)

func validateDocuments(repo string) error {
	return verify.ValidateDocuments(repo)
}

func validateOptionsForOperation(repo, target, kind string, exists bool, op operation, opts options) error {
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	if op == opDelete {
		if !exists {
			return fmt.Errorf("cannot delete %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to delete existing source")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 || len(opts.Sources) > 0 {
			return fmt.Errorf("--delete cannot be combined with --title, --summary, --tag, or --source")
		}
		return nil
	}

	if kind == "source" {
		if len(opts.Sources) > 0 {
			return fmt.Errorf("source writes must not specify --source")
		}
		if op != opSource {
			return fmt.Errorf("sources are immutable; refusing to mutate existing source")
		}
	}

	if kind == "wiki" {
		if len(opts.Sources) == 0 {
			return fmt.Errorf("wiki writes require at least one --source")
		}
		if err := validateSourcePaths(repo, opts.Sources); err != nil {
			return err
		}
	}

	if op == opCreate && strings.TrimSpace(opts.Title) == "" {
		return fmt.Errorf("--title is required when creating a new wiki file")
	}
	if op == opAppend {
		if !exists {
			return fmt.Errorf("cannot append to %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to append to existing source")
		}
		section := strings.TrimSpace(opts.Append)
		if section == "" {
			return fmt.Errorf("--append requires a non-empty section name")
		}
		if strings.EqualFold(section, "Sources") {
			return fmt.Errorf("--append cannot target the generated Sources section")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 {
			return fmt.Errorf("--append cannot change --title, --summary, or --tag in this version")
		}
	}
	return nil
}

func validateSourcePaths(repo string, sources []string) error {
	for _, source := range sources {
		normalized, kind, err := normalizeTargetPath(source)
		if err != nil {
			return fmt.Errorf("invalid --source %q: %w", source, err)
		}
		if kind != "source" {
			return fmt.Errorf("--source %q must be under sources/", source)
		}
		if err := ensureSafeFilesystemTarget(repo, normalized); err != nil {
			return fmt.Errorf("--source %q is unsafe: %w", source, err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(normalized))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("--source %q does not exist", source)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("--source %q must be a regular Markdown file", source)
		}
	}
	return nil
}

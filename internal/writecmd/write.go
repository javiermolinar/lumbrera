package writecmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/git"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Repo      string
	Target    string
	Reason    string
	Actor     string
	Title     string
	Summary   string
	Tags      []string
	Sources   []string
	Append    string
	AppendSet bool
	Delete    bool
	Help      bool
}

type operation string

const (
	opSource operation = "source"
	opCreate operation = "create"
	opUpdate operation = "update"
	opAppend operation = "append"
	opDelete operation = "delete"
)

func Run(args []string, stdin io.Reader) error {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp()
		return err
	}
	if opts.Help {
		printHelp()
		return nil
	}

	repo, err := resolveRepo(opts.Repo)
	if err != nil {
		return err
	}
	if err := preflight(repo); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor, err = defaultActor(repo)
		if err != nil {
			return err
		}
	}
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	target, kind, err := normalizeTargetPath(opts.Target)
	if err != nil {
		return err
	}
	if err := ensureSafeFilesystemTarget(repo, target); err != nil {
		return err
	}

	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	exists, err := fileExists(absTarget)
	if err != nil {
		return err
	}

	op, err := inferOperation(kind, exists, opts)
	if err != nil {
		return err
	}
	if err := validateOptionsForOperation(repo, target, kind, exists, op, opts); err != nil {
		return err
	}

	var input []byte
	if op != opDelete {
		input, err = io.ReadAll(stdin)
		if err != nil {
			return err
		}
		if len(input) == 0 {
			return fmt.Errorf("write requires Markdown content on stdin")
		}
		if frontmatter.StartsWithFrontmatter(input) {
			return fmt.Errorf("stdin must contain Markdown body only; Lumbrera generates frontmatter")
		}
		if kind == "wiki" && hasSourcesSection(string(input)) {
			return fmt.Errorf("stdin must not contain a ## Sources section; Lumbrera generates it")
		}
	}

	base, err := currentHead(repo)
	if err != nil {
		return err
	}
	commitTime := time.Now()
	commitSubject := fmt.Sprintf("[%s] [%s]: %s", op, opts.Actor, opts.Reason)

	mutated := false
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if mutated {
			if rollbackErr := rollbackWrite(repo, base); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}

	mutated = true
	if err := applyMutation(repo, target, kind, op, opts, input); err != nil {
		return fail(err)
	}
	files, err := generate.FilesForRepoWithPending(repo, []generate.PendingChangelogEntry{{Date: commitTime, Subject: commitSubject}})
	if err != nil {
		return fail(err)
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		return fail(err)
	}
	if err := verify.Run(repo, verify.Options{PendingChangelog: []generate.PendingChangelogEntry{{Date: commitTime, Subject: commitSubject}}}); err != nil {
		return fail(err)
	}

	if err := git.AddAll(repo); err != nil {
		return fail(err)
	}
	if err := git.Commit(repo, commitSubject); err != nil {
		return fail(err)
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return fail(err)
	}
	if !clean {
		return fail(fmt.Errorf("write committed but working tree is not clean"))
	}
	if err := git.Push(repo); err != nil {
		return fail(fmt.Errorf("failed to push Lumbrera write: %w", err))
	}
	fmt.Printf("Committed and pushed Lumbrera write: %s\n", commitSubject)
	return nil
}

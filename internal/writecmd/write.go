package writecmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/ops"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type options struct {
	Brain     string
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

func Run(args []string, stdin io.Reader) (err error) {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp()
		return err
	}
	if opts.Help {
		printHelp()
		return nil
	}

	brainDir, err := cliutil.ResolveBrain(opts.Brain)
	if err != nil {
		return err
	}
	if err := brain.ValidateRepo(brainDir); err != nil {
		return err
	}
	lock, err := brainlock.Acquire(brainDir, "write")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()
	if err := preflight(brainDir); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor, err = defaultActor(brainDir)
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
	if err := ensureSafeFilesystemTarget(brainDir, target); err != nil {
		return err
	}

	absTarget := filepath.Join(brainDir, filepath.FromSlash(target))
	exists, err := fileExists(absTarget)
	if err != nil {
		return err
	}

	op, err := inferOperation(kind, exists, opts)
	if err != nil {
		return err
	}
	if err := validateOptionsForOperation(brainDir, target, kind, exists, op, opts); err != nil {
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
		if kind == "wiki" && frontmatter.StartsWithFrontmatter(input) {
			return fmt.Errorf("stdin must contain Markdown body only; Lumbrera generates frontmatter")
		}
		if kind == "wiki" && hasSourcesSection(string(input)) {
			return fmt.Errorf("stdin must not contain a ## Sources section; Lumbrera generates it")
		}
	}

	backup, err := newWriteBackup(brainDir, target)
	if err != nil {
		return err
	}
	operationEntry := ops.NewEntry(string(op), opts.Actor, opts.Reason, time.Now())

	mutated := false
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if mutated {
			if rollbackErr := backup.Restore(); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}

	mutated = true
	if err := applyMutation(brainDir, target, kind, op, opts, input); err != nil {
		return fail(err)
	}
	if err := ops.Append(brainDir, operationEntry); err != nil {
		return fail(err)
	}
	files, err := generate.FilesForRepo(brainDir)
	if err != nil {
		return fail(err)
	}
	if err := generate.WriteFiles(brainDir, files); err != nil {
		return fail(err)
	}
	if err := verify.Run(brainDir, verify.Options{}); err != nil {
		return fail(err)
	}

	fmt.Printf("Applied Lumbrera write: [%s] [%s]: %s\n", op, opts.Actor, opts.Reason)
	return nil
}

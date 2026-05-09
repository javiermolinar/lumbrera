package movecmd

import (
	"fmt"
	"os"
	"strings"
)

type options struct {
	Brain  string
	From   string
	To     string
	Reason string
	Actor  string
	Help   bool
}

func parseArgs(args []string) (options, error) {
	var opts options
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" || arg == "help" {
			opts.Help = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		nextValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--brain", "--repo":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Brain = v
		case "--reason":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Reason = v
		case "--actor":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Actor = v
		default:
			return options{}, fmt.Errorf("unknown move option %s", name)
		}
	}
	if opts.Help {
		return opts, nil
	}
	if len(positional) != 2 {
		return options{}, fmt.Errorf("move requires exactly two positional arguments: <from> <to>")
	}
	opts.From = positional[0]
	opts.To = positional[1]
	if strings.TrimSpace(opts.Reason) == "" {
		return options{}, fmt.Errorf("move requires --reason")
	}
	return opts, nil
}

func defaultActor() string {
	for _, key := range []string{"LUMBRERA_ACTOR", "USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "human"
}

func validateCommitFields(actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.ContainsAny(reason, "\r\n") {
		return fmt.Errorf("--reason must be a single line")
	}
	if actor != "" && strings.ContainsAny(actor, "]\r\n") {
		return fmt.Errorf("--actor must not contain ], carriage returns, or newlines")
	}
	return nil
}

func printHelp() {
	fmt.Println(`Move a file within a Lumbrera brain, rewriting all references.

Usage:
  lumbrera move <from> <to> --reason <reason> [--actor <actor>] [--brain <path>]

Moves a source, wiki page, or asset and deterministically rewrites all
references in other wiki pages. Wiki page document IDs are preserved.

Behavior:
  - wiki move: rewrites [text](old-path) → [text](new-path) in all wiki pages,
    updates frontmatter links, regenerates Sources sections with new relative paths
  - source move: rewrites frontmatter sources, ## Sources sections, and inline
    [source: ...] citations in all wiki pages that reference the source
  - asset move: rewrites ![alt](old-path) and [text](old-path) in all wiki pages

Constraints:
  - source and destination must be under the same content root
  - destination must not already exist
  - the operation is atomic with rollback on failure

Options:
  --brain <path>      target brain directory, default current directory
  --reason <reason>   single-line changelog reason
  --actor <actor>     actor label for changelog`)
}

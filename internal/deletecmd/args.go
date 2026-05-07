package deletecmd

import (
	"fmt"
	"strings"
)

type options struct {
	Brain  string
	Target string
	Reason string
	Actor  string
	Help   bool
}

func parseArgs(args []string) (options, error) {
	var opts options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" || arg == "help" {
			opts.Help = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if opts.Target != "" {
				return options{}, fmt.Errorf("delete accepts exactly one target path")
			}
			opts.Target = arg
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
			return options{}, fmt.Errorf("unknown delete option %s", name)
		}
	}
	if opts.Help {
		return opts, nil
	}
	if strings.TrimSpace(opts.Target) == "" {
		return options{}, fmt.Errorf("delete requires a target path")
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return options{}, fmt.Errorf("delete requires --reason")
	}
	return opts, nil
}

func printHelp() {
	fmt.Println(`Usage:
  lumbrera delete <path> --reason <reason> [options]

Deletes a source or wiki file and cascades cleanup through referencing pages.

Required:
  <path>              repo-relative Markdown path under sources/ or wiki/
  --reason <reason>   single-line changelog reason

Options:
  --brain <path>      target brain directory, default current directory
  --repo <path>       deprecated alias for --brain
  --actor <actor>     actor label for changelog, default LUMBRERA_ACTOR, USER, USERNAME, or human

Behavior:
  Source deletion:
    - removes the source file
    - strips inline [source: ...] citations from referencing wiki pages
    - removes the source from wiki frontmatter and ## Sources sections
    - cascade-deletes wiki pages left with zero sources

  Wiki deletion:
    - removes the wiki file
    - removes links to the deleted page from other wiki pages
    - updates frontmatter links in referencing pages

  All deletions:
    - log a changelog entry per deleted file
    - regenerate INDEX.md, CHANGELOG.md, BRAIN.sum, and tags.md
    - the brain must pass lumbrera verify after the operation`)
}

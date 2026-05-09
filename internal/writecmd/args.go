package writecmd

import (
	"fmt"
	"strings"
)

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
				return options{}, fmt.Errorf("write accepts exactly one target path")
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
		case "--title":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Title = v
		case "--summary":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Summary = v
		case "--tag":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Tags = append(opts.Tags, v)
		case "--source":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Sources = append(opts.Sources, v)
		case "--append":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Append = v
			opts.AppendSet = true
		case "--file":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.File = v
		case "--delete":
			if hasValue {
				return options{}, fmt.Errorf("--delete does not accept a value")
			}
			opts.Delete = true
		default:
			return options{}, fmt.Errorf("unknown write option %s", name)
		}
	}
	if opts.Help {
		return opts, nil
	}
	if strings.TrimSpace(opts.Target) == "" {
		return options{}, fmt.Errorf("write requires a target path")
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return options{}, fmt.Errorf("write requires --reason")
	}
	return opts, nil
}

func printHelp() {
	fmt.Println(`Usage:
  lumbrera write <path> [options] < content.md

Performs one Lumbrera write transaction and regenerates local metadata.

Required:
  <path>              repo-relative path under sources/, wiki/, or assets/
  --reason <reason>   single-line changelog reason

Options:
  --brain <path>      target brain directory, default current directory
  --repo <path>       deprecated alias for --brain
  --actor <actor>     actor label for changelog, default LUMBRERA_ACTOR, USER, USERNAME, or human
  --title <title>     required when creating a new wiki file
  --summary <text>    required single-line generated wiki frontmatter summary for new wiki files
  --tag <tag>         required generated wiki frontmatter tag for new wiki files, repeatable up to 5
  --source <path>     provenance source for wiki writes, repeatable
  --append <section>  append stdin content to a named section in an existing wiki page
  --file <path>       local file to copy into assets/ (required for asset writes)
  --delete            delete an existing wiki page

Rules:
  - source writes preserve stdin as raw Markdown
  - wiki stdin must contain Markdown body only; Lumbrera generates wiki document IDs and frontmatter
  - asset writes require --file and copy the local file into the target path
  - assets are immutable after creation; .md files are not allowed under assets/
  - new wiki pages require single-line --summary, 1-5 lowercase slug --tag values, and at most 400 body lines
  - source files are immutable after creation
  - wiki writes require at least one --source
  - local Markdown links and heading anchors must resolve
  - optional inline claim citations use [source: ../sources/path.md#heading-anchor]
  - successful writes update INDEX.md, CHANGELOG.md, BRAIN.sum, and tags.md
  - Git, cloud sync, backup, and sharing are external to Lumbrera`)
}

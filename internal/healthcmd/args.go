package healthcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/cmdutil"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

type options struct {
	Brain      string
	Kind       string
	Limit      int
	PathPrefix string
	TargetPath string
	JSON       bool
	Help       bool
}

func parseArgs(args []string) (options, error) {
	var opts options
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inlineValue, hasInlineValue := cmdutil.SplitInlineFlag(arg)
		switch name {
		case "--help", "-h", "help":
			return options{Help: true}, nil
		case "--brain", "--repo":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Brain = value
			i = next
		case "--limit":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			limit, err := strconv.Atoi(value)
			if err != nil {
				return options{}, fmt.Errorf("invalid --limit %q", value)
			}
			opts.Limit = limit
			i = next
		case "--kind":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.Kind = value
			i = next
		case "--path":
			value, next, err := cmdutil.OptionValue(args, i, name, inlineValue, hasInlineValue)
			if err != nil {
				return options{}, err
			}
			opts.PathPrefix = value
			i = next
		case "--json":
			if hasInlineValue {
				return options{}, fmt.Errorf("--json does not accept a value")
			}
			opts.JSON = true
		default:
			if strings.HasPrefix(arg, "--") {
				return options{}, fmt.Errorf("unknown health option %q", arg)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) > 1 {
		return options{}, fmt.Errorf("health accepts at most one positional wiki/source path")
	}
	if len(positional) == 1 {
		if strings.TrimSpace(opts.PathPrefix) != "" {
			return options{}, fmt.Errorf("use either --path or a positional path, not both")
		}
		path, _, err := pathpolicy.NormalizeTargetPath(positional[0])
		if err != nil {
			return options{}, err
		}
		opts.TargetPath = path
	}
	if !isValidCandidateKind(opts.Kind) {
		return options{}, fmt.Errorf("invalid candidate kind %q", opts.Kind)
	}
	return opts, nil
}

func isValidCandidateKind(kind string) bool {
	switch kind {
	case "", searchindex.CandidateKindAll, searchindex.CandidateKindDuplicates, searchindex.CandidateKindLinks, searchindex.CandidateKindSources, searchindex.CandidateKindOrphans, searchindex.CandidateKindStubs, searchindex.CandidateKindTags:
		return true
	default:
		return false
	}
}

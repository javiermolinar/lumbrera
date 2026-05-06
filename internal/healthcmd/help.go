package healthcmd

import (
	"fmt"
	"io"
)

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Return deterministic Lumbrera health/consolidation review candidates.

Usage:
  lumbrera health [wiki/page.md|sources/source.md] [--brain <path>] [--path <prefix>] [--kind all|duplicates|links|sources|orphans] [--limit <n>] [--json]

Behavior:
  - candidates are deterministic review hints, not semantic drift diagnoses
  - output is read-only and does not mutate brain Markdown
  - missing or stale indexes are rebuilt automatically once
  - incompatible indexes require lumbrera index --rebuild

Options:
  --brain <path>      target brain directory, defaults to the current directory
  --repo <path>       deprecated alias for --brain
  --path <prefix>     restrict to a repo path prefix
  --kind <value>      restrict to all, duplicates, links, sources, or orphans; default all
  --limit <n>         max candidates, default 10, maximum 50
  --json              emit JSON instead of compact human output`)
}

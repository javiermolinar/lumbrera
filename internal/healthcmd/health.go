package healthcmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/javiermolinar/lumbrera/internal/cliutil"
	"github.com/javiermolinar/lumbrera/internal/indexruntime"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

func Run(args []string) error {
	return RunWithOutput(args, os.Stdout)
}

func RunWithOutput(args []string, out io.Writer) error {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp(out)
		return err
	}
	if opts.Help {
		printHelp(out)
		return nil
	}

	brainDir, err := cliutil.ResolveBrain(opts.Brain)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := indexruntime.EnsureFresh(ctx, brainDir); err != nil {
		return err
	}

	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(brainDir))
	if err != nil {
		return fmt.Errorf("open search index: %w; run lumbrera index --rebuild --brain %s", err, brainDir)
	}
	defer db.Close()

	pathPrefix := opts.PathPrefix
	if opts.TargetPath != "" {
		pathPrefix = opts.TargetPath
	}
	response, err := searchindex.HealthCandidates(ctx, db, searchindex.CandidateOptions{
		Limit:      opts.Limit,
		Kind:       opts.Kind,
		PathPrefix: pathPrefix,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, response)
	}
	return writeHuman(out, response)
}

package indexruntime

import (
	"context"
	"fmt"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brainlock"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

type RebuildOptions struct {
	LockName                   string
	RepairMissingModifiedDates bool
}

func EnsureFresh(ctx context.Context, brainDir string) error {
	status, err := searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	switch status.State {
	case searchindex.StatusFresh:
		return nil
	case searchindex.StatusMissing, searchindex.StatusStale:
		return autoRebuild(ctx, brainDir, status.State)
	case searchindex.StatusIncompatible:
		return fmt.Errorf("search index is incompatible: %s; run lumbrera index --rebuild --brain %s", status.Reason, brainDir)
	default:
		return fmt.Errorf("search index has unknown status %q; run lumbrera index --status --brain %s", status.State, brainDir)
	}
}

func Rebuild(ctx context.Context, brainDir string, opts RebuildOptions) error {
	lockName := opts.LockName
	if lockName == "" {
		lockName = "index"
	}
	lock, err := brainlock.Acquire(brainDir, lockName)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	return rebuildChecked(ctx, brainDir, opts)
}

func autoRebuild(ctx context.Context, brainDir string, state searchindex.StatusState) error {
	lock, err := brainlock.Acquire(brainDir, "search-index")
	if err != nil {
		return fmt.Errorf("search index is %s and automatic rebuild could not acquire lock: %w; run lumbrera index --rebuild --brain %s", state, err, brainDir)
	}
	defer func() { _ = lock.Release() }()

	status, err := searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	if status.State == searchindex.StatusFresh {
		return nil
	}
	if status.State == searchindex.StatusIncompatible {
		return fmt.Errorf("search index is incompatible: %s; run lumbrera index --rebuild --brain %s", status.Reason, brainDir)
	}
	if status.State != searchindex.StatusMissing && status.State != searchindex.StatusStale {
		return fmt.Errorf("search index has unknown status %q; run lumbrera index --status --brain %s", status.State, brainDir)
	}

	if err := verify.Check(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("cannot automatically rebuild search index because brain verification failed: %w; run lumbrera verify --brain %s", err, brainDir)
	}
	if err := searchindex.RebuildBrain(ctx, brainDir); err != nil {
		return fmt.Errorf("search index is %s and automatic rebuild failed: %w; run lumbrera index --rebuild --brain %s", state, err, brainDir)
	}
	status, err = searchindex.CheckStatus(ctx, brainDir)
	if err != nil {
		return err
	}
	if status.State != searchindex.StatusFresh {
		return fmt.Errorf("automatic rebuild completed but search index is %s: %s; run lumbrera index --rebuild --brain %s", status.State, status.Reason, brainDir)
	}
	return nil
}

func rebuildChecked(ctx context.Context, brainDir string, opts RebuildOptions) error {
	if err := verify.Check(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("cannot rebuild search index because brain verification failed: %w; run lumbrera verify --brain %s", err, brainDir)
	}
	if opts.RepairMissingModifiedDates {
		if err := repairMissingModifiedDates(brainDir); err != nil {
			return err
		}
	}
	return searchindex.RebuildBrain(ctx, brainDir)
}

func repairMissingModifiedDates(brainDir string) error {
	repaired, err := searchindex.RepairMissingModifiedDates(brainDir, time.Now().Format("2006-01-02"))
	if err != nil {
		return err
	}
	if !repaired {
		return nil
	}
	files, err := generate.FilesForRepo(brainDir)
	if err != nil {
		return err
	}
	if err := generate.WriteFiles(brainDir, files); err != nil {
		return err
	}
	if err := verify.Check(brainDir, verify.Options{}); err != nil {
		return fmt.Errorf("cannot rebuild search index after repairing modified dates because brain verification failed: %w; run lumbrera verify --brain %s", err, brainDir)
	}
	return nil
}

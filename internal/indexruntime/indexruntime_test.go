package indexruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

func TestEnsureFreshAutoRebuildsMissingIndex(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes mention runtimeunique.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody mentions runtimeunique.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")

	if err := EnsureFresh(context.Background(), repo); err != nil {
		t.Fatalf("EnsureFresh failed: %v", err)
	}
	status, err := searchindex.CheckStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}
	if status.State != searchindex.StatusFresh {
		t.Fatalf("status = %s, want fresh: %#v", status.State, status)
	}
}

func TestEnsureFreshRejectsVerifyDrift(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Raw source\n\nRaw notes.\n", "sources/raw.md", "--reason", "Preserve raw source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nBody.\n", "wiki/topic.md", "--title", "Topic", "--summary", "Topic summary.", "--tag", "topic", "--source", "sources/raw.md", "--reason", "Create topic", "--actor", "test")
	braintest.WriteFile(t, repo, "tags.md", braintest.ReadFile(t, repo, "tags.md")+"\nManual drift.\n")

	err := EnsureFresh(context.Background(), repo)
	if err == nil {
		t.Fatal("EnsureFresh succeeded with generated-file drift, want error")
	}
	if !strings.Contains(err.Error(), "verify") || !strings.Contains(err.Error(), "tags.md") {
		t.Fatalf("error = %v, want verify tags.md error", err)
	}
}

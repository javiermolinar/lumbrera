package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

func TestAppendReadAndFormatLine(t *testing.T) {
	repo := initChangelog(t)
	entry := NewEntry("create", "agent", "Create topic", time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC))
	if err := Append(repo, entry); err != nil {
		t.Fatalf("append: %v", err)
	}

	entries, err := Read(repo)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0] != entry {
		t.Fatalf("got %+v want %+v", entries[0], entry)
	}
	if got, want := FormatLine(entries[0]), "2026-05-04 [create] [agent]: Create topic"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// Placeholder should be gone.
	content, _ := os.ReadFile(filepath.Join(repo, brain.ChangelogPath))
	if strings.Contains(string(content), "No operations yet") {
		t.Fatal("placeholder should be removed after first append")
	}
}

func TestAppendMultipleEntries(t *testing.T) {
	repo := initChangelog(t)
	e1 := NewEntry("source", "agent", "Preserve source", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC))
	e2 := NewEntry("create", "agent", "Create topic", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC))
	if err := Append(repo, e1); err != nil {
		t.Fatal(err)
	}
	if err := Append(repo, e2); err != nil {
		t.Fatal(err)
	}
	entries, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0] != e1 || entries[1] != e2 {
		t.Fatalf("entries mismatch: %+v", entries)
	}
}

func TestReadRejectsInvalidEntry(t *testing.T) {
	repo := initChangelog(t)
	if err := Append(repo, NewEntry("source", "agent", "Preserve source", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	// Append a raw bad line.
	path := filepath.Join(repo, brain.ChangelogPath)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("2026-05-04 [bad] [agent]: Nope\n")
	f.Close()

	_, err = Read(repo)
	if err == nil {
		t.Fatal("expected invalid entry error")
	}
	if !strings.Contains(err.Error(), "operation") {
		t.Fatalf("expected operation error, got %v", err)
	}
}

func TestParseLine(t *testing.T) {
	entry, err := ParseLine("2026-05-04 [create] [agent]: Create topic")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Date != "2026-05-04" || entry.Operation != "create" || entry.Actor != "agent" || entry.Reason != "Create topic" {
		t.Fatalf("unexpected: %+v", entry)
	}
}

func TestParseLineRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"not a valid line",
		"2026-05-04 missing brackets",
		"2026-05-04 [create] missing actor",
		"05-04-2026 [create] [agent]: bad date",
	}
	for _, tc := range cases {
		if _, err := ParseLine(tc); err == nil {
			t.Fatalf("expected error for %q", tc)
		}
	}
}

func TestRender(t *testing.T) {
	empty := Render(nil)
	if !strings.Contains(empty, "No operations yet.") {
		t.Fatalf("expected empty placeholder, got:\n%s", empty)
	}

	entries := []Entry{
		{Date: "2026-05-04", Operation: "source", Actor: "agent", Reason: "Preserve source"},
		{Date: "2026-05-04", Operation: "create", Actor: "agent", Reason: "Create topic"},
	}
	got := Render(entries)
	if !strings.Contains(got, "# Lumbrera Changelog") {
		t.Fatalf("missing header:\n%s", got)
	}
	if !strings.Contains(got, "[source] [agent]: Preserve source") {
		t.Fatalf("missing source entry:\n%s", got)
	}
	if !strings.Contains(got, "[create] [agent]: Create topic") {
		t.Fatalf("missing create entry:\n%s", got)
	}
}

func TestValidateRejectsMalformedEntries(t *testing.T) {
	cases := []Entry{
		NewEntry("bogus", "agent", "Reason", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)),
		NewEntry("create", "", "Reason", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)),
		NewEntry("create", "agent", "", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)),
		{Date: "05-04-2026", Operation: "create", Actor: "agent", Reason: "Reason"},
		{Date: "2026-05-04", Operation: "create", Actor: "bad]actor", Reason: "Reason"},
		{Date: "2026-05-04", Operation: "create", Actor: "agent", Reason: "bad\nreason"},
	}
	for _, tc := range cases {
		if err := Validate(tc); err == nil {
			t.Fatalf("expected invalid entry to fail: %+v", tc)
		}
	}
}

// initChangelog creates a temp repo with a valid CHANGELOG.md.
func initChangelog(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	path := filepath.Join(repo, brain.ChangelogPath)
	if err := os.WriteFile(path, []byte(Render(nil)), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

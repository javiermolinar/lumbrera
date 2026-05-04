package ops

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestAppendReadAndChangelogLine(t *testing.T) {
	repo := t.TempDir()
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
	if got, want := ChangelogLine(entries[0]), "2026-05-04 [create] [agent]: Create topic"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReadRejectsInvalidEntry(t *testing.T) {
	repo := t.TempDir()
	if err := Append(repo, NewEntry("source", "agent", "Preserve source", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	path := repo + "/" + LogPath
	appendRaw(t, path, `{"date":"2026-05-04","operation":"bad","actor":"agent","reason":"Nope"}`+"\n")

	_, err := Read(repo)
	if err == nil {
		t.Fatal("expected invalid entry error")
	}
	if !strings.Contains(err.Error(), "operation") {
		t.Fatalf("expected operation error, got %v", err)
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

func appendRaw(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

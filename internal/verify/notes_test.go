package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func TestVerifyRejectsMissingNoteModifiedDate(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Observation\n\nDetail.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "A durable observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	path := filepath.Join(repo, "notes", "observation.md")
	content := testfs.ReadPath(t, path)
	content = removeLineContaining(content, "modified_date:")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	regenerateDerived(t, repo)

	err := verify.Check(repo, verify.Options{})
	if err == nil || !strings.Contains(err.Error(), "modified_date") {
		t.Fatalf("verify error = %v, want missing modified_date", err)
	}
}

func TestVerifyRejectsStaleOrInvalidNoteMetadata(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "links",
			mutate: func(content string) string {
				return strings.Replace(content, "notes/other.md", "notes/wrong.md", 1)
			},
			wantErr: "frontmatter links are stale",
		},
		{
			name: "tags",
			mutate: func(content string) string {
				return strings.Replace(content, "    - operations", "    - Bad Tag", 1)
			},
			wantErr: "frontmatter tag",
		},
		{
			name: "id",
			mutate: func(content string) string {
				return strings.Replace(content, "id: doc_", "id: invalid_", 1)
			},
			wantErr: "lumbrera.id",
		},
		{
			name: "modified date",
			mutate: func(content string) string {
				start := strings.Index(content, "modified_date:")
				if start < 0 {
					return content
				}
				end := strings.IndexByte(content[start:], '\n')
				return content[:start] + "modified_date: yesterday" + content[start+end:]
			},
			wantErr: "modified_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initBrain(t)
			runWrite(t, repo, "# Other\n\nOther detail.\n", "notes/other.md",
				"--title", "Other", "--summary", "Another durable note.", "--tag", "operations",
				"--reason", "Record other", "--actor", "test")
			runWrite(t, repo, "# Observation\n\nSee [Other](./other.md).\n", "notes/observation.md",
				"--title", "Observation", "--summary", "A durable observation.", "--tag", "operations",
				"--reason", "Record observation", "--actor", "test")
			path := filepath.Join(repo, "notes", "observation.md")
			before := testfs.ReadPath(t, path)
			after := tt.mutate(before)
			if after == before {
				t.Fatal("test mutation did not alter note")
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			err := verify.Check(repo, verify.Options{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("verify error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyRejectsSourceCitationInNote(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw\n\nEvidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	runWrite(t, repo, "# Observation\n\nDetail.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "A durable observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	path := filepath.Join(repo, "notes", "observation.md")
	content := testfs.ReadPath(t, path)
	content = strings.Replace(content, "Detail.", "Detail. [source: ../sources/raw.md]", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	regenerateDerived(t, repo)

	err := verify.Check(repo, verify.Options{})
	if err == nil || !strings.Contains(err.Error(), "must not contain source citations") {
		t.Fatalf("verify error = %v, want note citation rejection", err)
	}
}

func TestVerifyRejectsDuplicateIDsAcrossWikiAndNotes(t *testing.T) {
	repo := initBrain(t)
	runWrite(t, repo, "# Raw\n\nEvidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	runWrite(t, repo, "# Topic\n\nGuidance.\n", "wiki/topic.md",
		"--title", "Topic", "--summary", "Canonical guidance.", "--tag", "topic", "--source", "sources/raw.md",
		"--reason", "Create topic", "--actor", "test")
	runWrite(t, repo, "# Observation\n\nDetail.\n", "notes/observation.md",
		"--title", "Observation", "--summary", "A durable observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	wikiMeta, _, _, err := frontmatter.Split([]byte(testfs.ReadFile(t, repo, "wiki/topic.md")))
	if err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(repo, "notes", "observation.md")
	note := testfs.ReadPath(t, notePath)
	start := strings.Index(note, "id: doc_")
	if start < 0 {
		t.Fatal("note fixture has no ID")
	}
	end := strings.IndexByte(note[start:], '\n')
	if end < 0 {
		t.Fatal("note fixture ID line has no newline")
	}
	note = note[:start] + "id: " + wikiMeta.Lumbrera.ID + note[start+end:]
	if err := os.WriteFile(notePath, []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	regenerateDerived(t, repo)

	err = verify.Check(repo, verify.Options{})
	if err == nil || !strings.Contains(err.Error(), "duplicates Lumbrera document id") {
		t.Fatalf("verify error = %v, want duplicate managed ID", err)
	}
}

func regenerateDerived(t *testing.T, repo string) {
	t.Helper()
	files, err := generate.FilesForRepo(repo)
	if err != nil {
		t.Fatalf("generate derived files: %v", err)
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		t.Fatalf("write derived files: %v", err)
	}
}

func removeLineContaining(content, value string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, value) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

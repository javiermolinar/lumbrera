package pathpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTargetPath(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantPath string
		wantKind string
		wantErr  string
	}{
		{name: "wiki", raw: "./wiki/topic.md", wantPath: "wiki/topic.md", wantKind: "wiki"},
		{name: "source", raw: "sources/raw.md", wantPath: "sources/raw.md", wantKind: "source"},
		{name: "absolute", raw: filepath.Join(string(filepath.Separator), "wiki", "topic.md"), wantErr: "absolute"},
		{name: "parent", raw: "wiki/../sources/raw.md", wantErr: ".."},
		{name: "asset", raw: "assets/diagram.png", wantPath: "assets/diagram.png", wantKind: "asset"},
		{name: "asset nested", raw: "assets/diagrams/arch.png", wantPath: "assets/diagrams/arch.png", wantKind: "asset"},
		{name: "asset md rejected", raw: "assets/notes.md", wantErr: "Markdown files are not allowed"},
		{name: "wrong root", raw: "notes/topic.md", wantErr: "sources/, wiki/, or assets/"},
		{name: "not markdown", raw: "wiki/topic.txt", wantErr: "Markdown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotKind, err := NormalizeTargetPath(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NormalizeTargetPath(%q) error = %v, want containing %q", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTargetPath(%q): %v", tt.raw, err)
			}
			if gotPath != tt.wantPath || gotKind != tt.wantKind {
				t.Fatalf("NormalizeTargetPath(%q) = (%q, %q), want (%q, %q)", tt.raw, gotPath, gotKind, tt.wantPath, tt.wantKind)
			}
		})
	}
}

func TestEnsureSafeFilesystemTargetRejectsSymlinkTarget(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "real.md"), []byte("# Real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "real.md"), filepath.Join(repo, "wiki", "topic.md")); err != nil {
		t.Fatal(err)
	}

	err := EnsureSafeFilesystemTarget(repo, "wiki/topic.md")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("EnsureSafeFilesystemTarget symlink error = %v, want symlink rejection", err)
	}
}

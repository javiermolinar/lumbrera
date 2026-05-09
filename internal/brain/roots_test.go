package brain

import "testing"

func TestRootForPath(t *testing.T) {
	tests := []struct {
		path     string
		wantKind string
		wantOK   bool
	}{
		{"sources/raw.md", "source", true},
		{"sources/design/adr.md", "source", true},
		{"wiki/topic.md", "wiki", true},
		{"wiki/design/spec.md", "wiki", true},
		{"assets/diagram.png", "asset", true},
		{"assets/diagrams/arch.png", "asset", true},
		{"notes/topic.md", "", false},
		{"sources", "", false},
		{"wiki", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			root, ok := RootForPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("RootForPath(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			}
			if ok && root.Kind != tt.wantKind {
				t.Fatalf("RootForPath(%q) kind = %q, want %q", tt.path, root.Kind, tt.wantKind)
			}
		})
	}
}

func TestKindForPath(t *testing.T) {
	if got := KindForPath("sources/raw.md"); got != "source" {
		t.Fatalf("KindForPath sources = %q", got)
	}
	if got := KindForPath("wiki/topic.md"); got != "wiki" {
		t.Fatalf("KindForPath wiki = %q", got)
	}
	if got := KindForPath("assets/diagram.png"); got != "asset" {
		t.Fatalf("KindForPath assets = %q", got)
	}
	if got := KindForPath("notes/other.md"); got != "" {
		t.Fatalf("KindForPath unknown = %q", got)
	}
}

func TestContentDirList(t *testing.T) {
	got := ContentDirList()
	want := "sources/, wiki/, or assets/"
	if got != want {
		t.Fatalf("ContentDirList() = %q, want %q", got, want)
	}
}

func TestContentDirs(t *testing.T) {
	dirs := ContentDirs()
	if len(dirs) != 3 || dirs[0] != "sources" || dirs[1] != "wiki" || dirs[2] != "assets" {
		t.Fatalf("ContentDirs() = %v", dirs)
	}
}

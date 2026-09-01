package brain

import "testing"

func TestPolicyForPath(t *testing.T) {
	tests := []struct {
		path     string
		wantKind Kind
		wantOK   bool
	}{
		{"sources/raw.md", KindSource, true},
		{"sources/design/adr.md", KindSource, true},
		{"wiki/topic.md", KindWiki, true},
		{"wiki/design/spec.md", KindWiki, true},
		{"assets/diagram.png", KindAsset, true},
		{"assets/diagrams/arch.png", KindAsset, true},
		{"notes/topic.md", "", false},
		{"sources", "", false},
		{"wiki", "", false},
		{"wiki/", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			policy, ok := PolicyForPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("PolicyForPath(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			}
			if ok && policy.Kind != tt.wantKind {
				t.Fatalf("PolicyForPath(%q) kind = %q, want %q", tt.path, policy.Kind, tt.wantKind)
			}
		})
	}
}

func TestKindForPath(t *testing.T) {
	if got := KindForPath("sources/raw.md"); got != string(KindSource) {
		t.Fatalf("KindForPath sources = %q", got)
	}
	if got := KindForPath("wiki/topic.md"); got != string(KindWiki) {
		t.Fatalf("KindForPath wiki = %q", got)
	}
	if got := KindForPath("assets/diagram.png"); got != string(KindAsset) {
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

func TestEveryRootHasExactlyOnePolicy(t *testing.T) {
	seenRoots := map[string]Kind{}
	seenKinds := map[Kind]string{}
	for _, policy := range Policies() {
		if policy.Root == "" || policy.Kind == "" {
			t.Fatalf("policy has empty root or kind: %+v", policy)
		}
		if previous, ok := seenRoots[policy.Root]; ok {
			t.Fatalf("root %q maps to both %q and %q", policy.Root, previous, policy.Kind)
		}
		if previous, ok := seenKinds[policy.Kind]; ok {
			t.Fatalf("kind %q maps to both %q and %q", policy.Kind, previous, policy.Root)
		}
		seenRoots[policy.Root] = policy.Kind
		seenKinds[policy.Kind] = policy.Root

		byKind, ok := PolicyForKind(policy.Kind)
		if !ok || byKind.Root != policy.Root {
			t.Fatalf("PolicyForKind(%q) = (%+v, %v)", policy.Kind, byKind, ok)
		}
		byPath, ok := PolicyForPath(policy.Root + "/doc")
		if !ok || byPath.Kind != policy.Kind {
			t.Fatalf("PolicyForPath(%s/doc) = (%+v, %v)", policy.Root, byPath, ok)
		}
	}
	if len(seenRoots) != 3 {
		t.Fatalf("got %d policies, want 3", len(seenRoots))
	}
}

func TestManagedAndSearchRoots(t *testing.T) {
	if got := ManagedRoots(); len(got) != 1 || got[0] != "wiki" {
		t.Fatalf("ManagedRoots() = %v, want [wiki]", got)
	}
	if got := SearchRoots(); len(got) != 2 || got[0] != "sources" || got[1] != "wiki" {
		t.Fatalf("SearchRoots() = %v, want [sources wiki]", got)
	}

	managed := ManagedPolicies()
	if len(managed) != 1 || managed[0].Kind != KindWiki {
		t.Fatalf("ManagedPolicies() = %+v, want wiki", managed)
	}
	markdown := MarkdownPolicies()
	if len(markdown) != 2 || markdown[0].Kind != KindSource || markdown[1].Kind != KindWiki {
		t.Fatalf("MarkdownPolicies() = %+v, want source then wiki", markdown)
	}
}

func TestWikiEvidencePolicy(t *testing.T) {
	if !CanUseAsEvidence(KindWiki, KindSource) {
		t.Fatal("wiki should accept source evidence")
	}
	if CanUseAsEvidence(KindWiki, KindWiki) {
		t.Fatal("wiki should not accept wiki evidence")
	}
	if CanUseAsEvidence(KindWiki, KindAsset) {
		t.Fatal("wiki should not accept asset evidence")
	}
	if CanUseAsEvidence(KindSource, KindSource) {
		t.Fatal("source should not accept evidence")
	}
	if CanUseAsEvidence(KindAsset, KindSource) {
		t.Fatal("asset should not accept evidence")
	}
}

func TestIsManagedKind(t *testing.T) {
	if !IsManagedKind(KindWiki) {
		t.Fatal("wiki should be managed")
	}
	if IsManagedKind(KindSource) || IsManagedKind(KindAsset) || IsManagedKind("note") {
		t.Fatal("only wiki should be managed")
	}
}

func TestUnknownKindsAndRootsFailClosed(t *testing.T) {
	if _, ok := PolicyForKind("note"); ok {
		t.Fatal("unknown kind note should fail closed")
	}
	if _, ok := PolicyForKind(""); ok {
		t.Fatal("empty kind should fail closed")
	}
	if _, ok := PolicyForPath("notes/topic.md"); ok {
		t.Fatal("notes/ should fail closed")
	}
	if CanUseAsEvidence("note", KindSource) {
		t.Fatal("unknown document kind should not accept evidence")
	}
}

func TestPolicyCopiesDoNotMutateTable(t *testing.T) {
	policy, ok := PolicyForKind(KindWiki)
	if !ok {
		t.Fatal("missing wiki policy")
	}
	policy.EvidenceKinds[0] = KindAsset
	policy.Root = "mutated"

	again, ok := PolicyForKind(KindWiki)
	if !ok {
		t.Fatal("missing wiki policy after mutation")
	}
	if again.Root != "wiki" || len(again.EvidenceKinds) != 1 || again.EvidenceKinds[0] != KindSource {
		t.Fatalf("policy table mutated: %+v", again)
	}
}

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
		{"notes/topic.md", KindNote, true},
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
	if got := KindForPath("notes/other.md"); got != string(KindNote) {
		t.Fatalf("KindForPath notes = %q", got)
	}
}

func TestContentDirList(t *testing.T) {
	got := ContentDirList()
	want := "sources/, notes/, wiki/, or assets/"
	if got != want {
		t.Fatalf("ContentDirList() = %q, want %q", got, want)
	}
}

func TestContentDirs(t *testing.T) {
	dirs := ContentDirs()
	if len(dirs) != 4 || dirs[0] != "sources" || dirs[1] != "notes" || dirs[2] != "wiki" || dirs[3] != "assets" {
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
	if len(seenRoots) != 4 {
		t.Fatalf("got %d policies, want 4", len(seenRoots))
	}
}

func TestManagedAndSearchRoots(t *testing.T) {
	if got := ManagedRoots(); len(got) != 2 || got[0] != "notes" || got[1] != "wiki" {
		t.Fatalf("ManagedRoots() = %v, want [notes wiki]", got)
	}
	if got := SearchRoots(); len(got) != 3 || got[0] != "sources" || got[1] != "notes" || got[2] != "wiki" {
		t.Fatalf("SearchRoots() = %v, want [sources notes wiki]", got)
	}
	if got := RequiredRoots(); len(got) != 4 || got[0] != "sources" || got[1] != "notes" || got[2] != "wiki" || got[3] != "assets" {
		t.Fatalf("RequiredRoots() = %v", got)
	}
	for kind, want := range map[Kind]string{KindWiki: IndexPath, KindSource: SourcesIndexPath, KindNote: NotesIndexPath, KindAsset: AssetsIndexPath} {
		if got, ok := CatalogPathForKind(kind); !ok || got != want {
			t.Fatalf("CatalogPathForKind(%q) = (%q, %v), want %q", kind, got, ok, want)
		}
	}

	managed := ManagedPolicies()
	if len(managed) != 2 || managed[0].Kind != KindNote || managed[1].Kind != KindWiki {
		t.Fatalf("ManagedPolicies() = %+v, want note then wiki", managed)
	}
	markdown := MarkdownPolicies()
	if len(markdown) != 3 || markdown[0].Kind != KindSource || markdown[1].Kind != KindNote || markdown[2].Kind != KindWiki {
		t.Fatalf("MarkdownPolicies() = %+v, want source, note, then wiki", markdown)
	}
}

func TestEvidencePolicy(t *testing.T) {
	for _, tt := range []struct {
		kind Kind
		want bool
	}{
		{KindSource, false},
		{KindNote, false},
		{KindWiki, true},
		{KindAsset, false},
	} {
		policy, ok := PolicyForKind(tt.kind)
		if !ok {
			t.Fatalf("missing %q policy", tt.kind)
		}
		if policy.RequiresEvidence != tt.want {
			t.Errorf("PolicyForKind(%q).RequiresEvidence = %v, want %v", tt.kind, policy.RequiresEvidence, tt.want)
		}
	}

	if !CanUseAsEvidence(KindWiki, KindSource) {
		t.Fatal("wiki should accept source evidence")
	}
	if !CanUseAsEvidence(KindWiki, KindNote) {
		t.Fatal("wiki should accept note evidence")
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
	if CanUseAsEvidence(KindNote, KindSource) {
		t.Fatal("note should not accept evidence")
	}
	if CanUseAsEvidence(KindAsset, KindSource) {
		t.Fatal("asset should not accept evidence")
	}
}

func TestManagedAndEvidenceHelpers(t *testing.T) {
	if !IsManagedKind(KindWiki) || !IsManagedPath("wiki/topic.md") || !IsManagedKind(KindNote) || !IsManagedPath("notes/topic.md") {
		t.Fatal("wiki and notes should be managed")
	}
	if IsManagedKind(KindSource) || IsManagedKind(KindAsset) {
		t.Fatal("source and asset must not be managed")
	}
	if IsManagedPath("sources/raw.md") || IsManagedPath("assets/diagram.png") {
		t.Fatal("non-managed paths must fail IsManagedPath")
	}
	if !AcceptsEvidence(KindWiki) {
		t.Fatal("wiki should accept evidence")
	}
	if AcceptsEvidence(KindSource) || AcceptsEvidence(KindAsset) || AcceptsEvidence(KindNote) {
		t.Fatal("only wiki should accept evidence")
	}
}

func TestUnknownKindsAndRootsFailClosed(t *testing.T) {
	if _, ok := PolicyForKind("memo"); ok {
		t.Fatal("unknown kind memo should fail closed")
	}
	if _, ok := PolicyForKind(""); ok {
		t.Fatal("empty kind should fail closed")
	}
	if _, ok := PolicyForPath("memos/topic.md"); ok {
		t.Fatal("memos/ should fail closed")
	}
	if _, ok := CatalogPathForKind("memo"); ok {
		t.Fatal("unknown kind memo should not have a catalog")
	}
	if CanUseAsEvidence("memo", KindSource) {
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
	if again.Root != "wiki" || len(again.EvidenceKinds) != 2 || again.EvidenceKinds[0] != KindSource || again.EvidenceKinds[1] != KindNote {
		t.Fatalf("policy table mutated: %+v", again)
	}
}

package brain_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/verify"
	"github.com/javiermolinar/lumbrera/internal/writecmd"
)

func TestSecondManagedKindUsesWriteParseRepairAndVerify(t *testing.T) {
	const noteKind brain.Kind = "memo"

	policies := brain.Policies()
	wikiFound := false
	for i := range policies {
		if policies[i].Kind != brain.KindWiki {
			continue
		}
		policies[i].EvidenceKinds = append(policies[i].EvidenceKinds, noteKind)
		wikiFound = true
	}
	if !wikiFound {
		t.Fatal("missing wiki policy")
	}
	policies = append(policies, brain.ContentPolicy{
		Root:             "memos",
		Kind:             noteKind,
		Storage:          brain.StorageManagedMarkdown,
		RequiredRoot:     true,
		Mutable:          true,
		RequiresEvidence: false,
		ProvidesEvidence: true,
		CatalogPath:      "MEMOS.md",
	})
	brain.InstallPoliciesForTest(t, policies)

	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if info, err := os.Stat(filepath.Join(repo, "memos")); err != nil || !info.IsDir() {
		t.Fatalf("required memo root was not scaffolded: info=%v err=%v", info, err)
	}

	runPolicyWrite(t, repo, "# Observation\n\nInitial detail.\n", "memos/observation.md",
		"--title", "Observation", "--summary", "A first-party observation.", "--tag", "operations",
		"--reason", "Record observation", "--actor", "test")

	notePolicy, ok := brain.PolicyForKind(noteKind)
	if !ok {
		t.Fatal("test note policy was not registered")
	}
	noteMeta, noteBody := readManagedDocument(t, repo, "memos/observation.md", notePolicy)
	originalID := noteMeta.Lumbrera.ID
	if originalID == "" || noteMeta.Lumbrera.Kind != string(noteKind) {
		t.Fatalf("unexpected note identity: %+v", noteMeta.Lumbrera)
	}
	if len(noteMeta.Lumbrera.Sources) != 0 || strings.Contains(noteBody, "## Sources") {
		t.Fatalf("evidence-free policy produced sources: metadata=%v body=%q", noteMeta.Lumbrera.Sources, noteBody)
	}

	// The second managed kind can provide evidence to another policy through the
	// same write and verification path.
	runPolicyWrite(t, repo, "# Topic\n\nSynthesized from the observation.\n", "wiki/topic.md",
		"--title", "Topic", "--summary", "A synthesized topic.", "--tag", "topic",
		"--source", "memos/observation.md", "--reason", "Create topic", "--actor", "test")
	wikiPolicy, ok := brain.PolicyForKind(brain.KindWiki)
	if !ok {
		t.Fatal("missing wiki policy")
	}
	wikiMeta, wikiBody := readManagedDocument(t, repo, "wiki/topic.md", wikiPolicy)
	if len(wikiMeta.Lumbrera.Sources) != 1 || wikiMeta.Lumbrera.Sources[0] != "memos/observation.md" {
		t.Fatalf("wiki evidence = %v, want note path", wikiMeta.Lumbrera.Sources)
	}
	if !strings.Contains(wikiBody, "- [Observation](../memos/observation.md)") {
		t.Fatalf("wiki is missing generated note evidence:\n%s", wikiBody)
	}

	// Replacement and append both use the managed mutation path, preserve the ID,
	// and regenerate managed-link metadata without adding a Sources section.
	runPolicyWrite(t, repo, "# Observation\n\nSee [Topic](../wiki/topic.md).\n", "memos/observation.md",
		"--reason", "Connect observation", "--actor", "test")
	runPolicyWrite(t, repo, "Appended detail.\n", "memos/observation.md",
		"--append", "Details", "--reason", "Extend observation", "--actor", "test")
	noteMeta, noteBody = readManagedDocument(t, repo, "memos/observation.md", notePolicy)
	if noteMeta.Lumbrera.ID != originalID {
		t.Fatalf("managed updates changed ID: got %q, want %q", noteMeta.Lumbrera.ID, originalID)
	}
	if len(noteMeta.Lumbrera.Links) != 1 || noteMeta.Lumbrera.Links[0] != "wiki/topic.md" {
		t.Fatalf("managed links = %v, want wiki/topic.md", noteMeta.Lumbrera.Links)
	}
	if !strings.Contains(noteBody, "## Details\n\nAppended detail.") || strings.Contains(noteBody, "## Sources") {
		t.Fatalf("unexpected updated note body:\n%s", noteBody)
	}

	// The shared repair pass must discover the alternate managed root, restore a
	// missing ID, regenerate derived files, and leave the repository verifiable.
	notePath := filepath.Join(repo, "memos", "observation.md")
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	withoutID := removeDocumentID(string(content))
	if withoutID == string(content) {
		t.Fatal("test fixture did not contain a generated ID")
	}
	if err := os.WriteFile(notePath, []byte(withoutID), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify.Run(repo, verify.Options{}); err != nil {
		t.Fatalf("repair and verify alternate managed kind: %v", err)
	}
	repairedMeta, _ := readManagedDocument(t, repo, "memos/observation.md", notePolicy)
	if repairedMeta.Lumbrera.ID == "" {
		t.Fatal("repair did not restore the note ID")
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("final verification failed: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(repo, brain.BrainSumPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "memos/observation.md sha256:") {
		t.Fatalf("managed note is missing from %s:\n%s", brain.BrainSumPath, manifest)
	}
}

func runPolicyWrite(t *testing.T, repo, stdin, target string, args ...string) {
	t.Helper()
	fullArgs := append([]string{target, "--brain", repo}, args...)
	if err := writecmd.Run(fullArgs, strings.NewReader(stdin)); err != nil {
		t.Fatalf("write %v failed: %v", fullArgs, err)
	}
}

func readManagedDocument(t *testing.T, repo, rel string, policy brain.ContentPolicy) (frontmatter.Document, string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	meta, body, has, err := frontmatter.SplitForPolicy(content, policy)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	if !has {
		t.Fatalf("%s is missing managed frontmatter", rel)
	}
	return meta, body
}

func removeDocumentID(content string) string {
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "id: doc_") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

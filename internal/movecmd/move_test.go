package movecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func TestMoveWikiPageRewritesLinks(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source\n\nFacts.\n", "sources/raw.md", "--reason", "Add source", "--actor", "test")

	braintest.RunWrite(t, repo, "# Target\n\nContent.\n", "wiki/target.md",
		"--title", "Target", "--summary", "Target page.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create target", "--actor", "test")

	braintest.RunWrite(t, repo, "# Referrer\n\nSee [Target](./target.md) for details.\n", "wiki/referrer.md",
		"--title", "Referrer", "--summary", "Referrer page.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create referrer", "--actor", "test")

	runMove(t, repo, "wiki/target.md", "wiki/topics/target.md", "--reason", "Reorganize", "--actor", "test")

	assertMissing(t, repo, "wiki/target.md")
	assertExists(t, repo, "wiki/topics/target.md")

	// Referrer should now link to the new path.
	referrer := testfs.ReadFile(t, repo, "wiki/referrer.md")
	if strings.Contains(referrer, "./target.md") {
		t.Fatalf("referrer still has old link:\n%s", referrer)
	}
	if !strings.Contains(referrer, "topics/target.md") {
		t.Fatalf("referrer missing new link:\n%s", referrer)
	}

	// Frontmatter links should be updated.
	meta, _, _, err := frontmatter.Split([]byte(referrer))
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(meta.Lumbrera.Links, "wiki/topics/target.md") {
		t.Fatalf("frontmatter links not updated: %#v", meta.Lumbrera.Links)
	}

	// INDEX should show new path.
	assertFileContains(t, repo, "INDEX.md", "wiki/topics/target.md")
	assertFileNotContains(t, repo, "INDEX.md", "wiki/target.md)")

	assertFileContains(t, repo, "CHANGELOG.md", "[move]")
	assertVerify(t, repo)
}

func TestMoveWikiPagePreservesDocumentID(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source\n\nFacts.\n", "sources/raw.md", "--reason", "Add source", "--actor", "test")
	braintest.RunWrite(t, repo, "# Page\n\nContent.\n", "wiki/page.md",
		"--title", "Page", "--summary", "A page.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create page", "--actor", "test")

	oldMeta := readMeta(t, repo, "wiki/page.md")
	oldID := oldMeta.Lumbrera.ID

	runMove(t, repo, "wiki/page.md", "wiki/moved/page.md", "--reason", "Move", "--actor", "test")

	newMeta := readMeta(t, repo, "wiki/moved/page.md")
	if newMeta.Lumbrera.ID != oldID {
		t.Fatalf("document ID changed: %q → %q", oldID, newMeta.Lumbrera.ID)
	}
	assertVerify(t, repo)
}

func TestMoveSourceRewritesWikiReferences(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source A\n\n## Evidence\n\nKey fact.\n", "sources/a.md", "--reason", "Add source A", "--actor", "test")
	braintest.RunWrite(t, repo, "# Topic\n\nClaim. [source: ../sources/a.md#evidence]\n", "wiki/topic.md",
		"--title", "Topic", "--summary", "Topic page.", "--tag", "topic",
		"--source", "sources/a.md", "--reason", "Create topic", "--actor", "test")

	runMove(t, repo, "sources/a.md", "sources/design/a.md", "--reason", "Reclassify", "--actor", "test")

	assertMissing(t, repo, "sources/a.md")
	assertExists(t, repo, "sources/design/a.md")

	wiki := testfs.ReadFile(t, repo, "wiki/topic.md")
	meta, _, _, err := frontmatter.Split([]byte(wiki))
	if err != nil {
		t.Fatal(err)
	}
	if containsString(meta.Lumbrera.Sources, "sources/a.md") {
		t.Fatalf("wiki still references old source: %#v", meta.Lumbrera.Sources)
	}
	if !containsString(meta.Lumbrera.Sources, "sources/design/a.md") {
		t.Fatalf("wiki missing new source: %#v", meta.Lumbrera.Sources)
	}
	if strings.Contains(wiki, "sources/a.md") {
		t.Fatalf("wiki body still references old source:\n%s", wiki)
	}
	if !strings.Contains(wiki, "sources/design/a.md") {
		t.Fatalf("wiki body missing new source path:\n%s", wiki)
	}
	assertVerify(t, repo)
}

func TestMoveAssetRewritesWikiReferences(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source\n\nFacts.\n", "sources/raw.md", "--reason", "Add source", "--actor", "test")

	tmpFile := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(tmpFile, []byte("fake-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	braintest.RunAssetWrite(t, repo, "assets/diagram.png", tmpFile, "--reason", "Add diagram", "--actor", "test")

	braintest.RunWrite(t, repo, "# Page\n\n![Diagram](assets/diagram.png)\n", "wiki/page.md",
		"--title", "Page", "--summary", "A page.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create page", "--actor", "test")

	runMove(t, repo, "assets/diagram.png", "assets/diagrams/diagram.png", "--reason", "Organize assets", "--actor", "test")

	assertMissing(t, repo, "assets/diagram.png")
	assertExists(t, repo, "assets/diagrams/diagram.png")

	wiki := testfs.ReadFile(t, repo, "wiki/page.md")
	if strings.Contains(wiki, "assets/diagram.png") && !strings.Contains(wiki, "assets/diagrams/diagram.png") {
		t.Fatalf("wiki still references old asset path:\n%s", wiki)
	}
	if !strings.Contains(wiki, "assets/diagrams/diagram.png") {
		t.Fatalf("wiki missing new asset path:\n%s", wiki)
	}
	assertVerify(t, repo)
}

func TestMoveRejectsCrossRoot(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source\n\nFacts.\n", "sources/raw.md", "--reason", "Add source", "--actor", "test")

	err := Run([]string{"sources/raw.md", "wiki/raw.md", "--brain", repo, "--reason", "Bad move", "--actor", "test"})
	if err == nil {
		t.Fatal("expected cross-root move to be rejected")
	}
	if !strings.Contains(err.Error(), "cannot move across") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMoveRejectsNonExistent(t *testing.T) {
	repo := braintest.InitBrain(t)
	err := Run([]string{"wiki/nope.md", "wiki/dest.md", "--brain", repo, "--reason", "Bad", "--actor", "test"})
	if err == nil {
		t.Fatal("expected non-existent source to be rejected")
	}
}

func TestMoveRejectsExistingDest(t *testing.T) {
	repo := braintest.InitBrain(t)
	braintest.RunWrite(t, repo, "# Source\n\nFacts.\n", "sources/raw.md", "--reason", "Add source", "--actor", "test")
	braintest.RunWrite(t, repo, "# A\n\nContent.\n", "wiki/a.md",
		"--title", "A", "--summary", "Page A.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create A", "--actor", "test")
	braintest.RunWrite(t, repo, "# B\n\nContent.\n", "wiki/b.md",
		"--title", "B", "--summary", "Page B.", "--tag", "topic",
		"--source", "sources/raw.md", "--reason", "Create B", "--actor", "test")

	err := Run([]string{"wiki/a.md", "wiki/b.md", "--brain", repo, "--reason", "Overwrite", "--actor", "test"})
	if err == nil {
		t.Fatal("expected existing destination to be rejected")
	}
}

// --- helpers ---

func runMove(t *testing.T, repo, from, to string, args ...string) {
	t.Helper()
	fullArgs := append([]string{from, to, "--brain", repo}, args...)
	if err := Run(fullArgs); err != nil {
		t.Fatalf("move %v failed: %v", fullArgs, err)
	}
}

func readMeta(t *testing.T, repo, rel string) frontmatter.Document {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	meta, _, _, err := frontmatter.Split(content)
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func assertExists(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func assertMissing(t *testing.T, repo, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", rel, err)
	}
}

func assertFileContains(t *testing.T, repo, rel, want string) {
	t.Helper()
	got := testfs.ReadFile(t, repo, rel)
	if !strings.Contains(got, want) {
		t.Fatalf("expected %s to contain %q, got:\n%s", rel, want, got)
	}
}

func assertFileNotContains(t *testing.T, repo, rel, unwanted string) {
	t.Helper()
	got := testfs.ReadFile(t, repo, rel)
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected %s not to contain %q, got:\n%s", rel, unwanted, got)
	}
}

func assertVerify(t *testing.T, repo string) {
	t.Helper()
	if err := verify.Run(repo, verify.Options{}); err != nil {
		t.Fatalf("verify failed after move: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

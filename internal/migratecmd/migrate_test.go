package migratecmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/braintest"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/indexruntime"
	"github.com/javiermolinar/lumbrera/internal/initcmd"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
	"github.com/javiermolinar/lumbrera/internal/testfs"
	"github.com/javiermolinar/lumbrera/internal/verify"
)

func TestMigrateV2ToV3(t *testing.T) {
	repo := newCurrentBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nMigration evidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Existing topic\n\nExisting v2 knowledge.\n", "wiki/existing.md",
		"--title", "Existing topic", "--summary", "Existing v2 knowledge.", "--tag", "migration", "--source", "sources/raw.md",
		"--reason", "Create existing topic", "--actor", "test")
	downgradeFixture(t, repo, brain.VersionV2)
	wikiPath := filepath.Join(repo, "wiki/existing.md")
	if err := os.WriteFile(wikiPath, []byte(removeLineContaining(testfs.ReadPath(t, wikiPath), "modified_date:")), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGeneratedVersion(t, repo, brain.VersionV2)
	if err := verify.CheckVersion(repo, brain.VersionV2, verify.Options{}); err != nil {
		t.Fatalf("legacy v2 fixture with no modified date should remain valid: %v", err)
	}
	cache := searchindex.SearchIndexPath(repo)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("disposable-v3-cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo, "--actor", "test"}); err != nil {
		t.Fatalf("migrate v2 to v3: %v", err)
	}

	assertMarker(t, repo, brain.Version)
	assertPathIsDir(t, repo, "notes")
	assertPathIsFile(t, repo, brain.NotesIndexPath)
	assertPathIsFile(t, repo, ".agents/skills/lumbrera-note/SKILL.md")
	assertPathIsFile(t, repo, "sources/raw.md")
	assertPathIsFile(t, repo, "wiki/existing.md")
	if !strings.Contains(testfs.ReadPath(t, wikiPath), "modified_date:") {
		t.Fatal("migration did not repair legacy wiki modified_date")
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("search cache survived migration: %v", err)
	}
	changelog := testfs.ReadFile(t, repo, brain.ChangelogPath)
	if strings.Count(changelog, "[migrate] [test]") != 1 || !strings.Contains(changelog, brain.VersionV2+" → "+brain.Version) {
		t.Fatalf("unexpected migration changelog:\n%s", changelog)
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("verify migrated v3 brain: %v", err)
	}
	if err := indexruntime.EnsureFresh(context.Background(), repo); err != nil {
		t.Fatalf("rebuild migrated search index: %v", err)
	}
	db, err := searchindex.OpenSQLite(searchindex.SearchIndexPath(repo))
	if err != nil {
		t.Fatalf("open rebuilt search index: %v", err)
	}
	defer db.Close()
	version, exists, err := searchindex.ReadSchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("read rebuilt schema version: %v", err)
	}
	if !exists || version != searchindex.CurrentSchemaVersion {
		t.Fatalf("rebuilt schema version = %d (exists %v), want %d", version, exists, searchindex.CurrentSchemaVersion)
	}
}

func TestMigrateV1ChainsThroughV2AndV3(t *testing.T) {
	repo := newCurrentBrain(t)
	downgradeFixture(t, repo, brain.VersionV1)

	if err := Run([]string{"--brain", repo, "--actor", "test"}); err != nil {
		t.Fatalf("migrate v1 to v3: %v", err)
	}

	assertMarker(t, repo, brain.Version)
	assertPathIsDir(t, repo, "assets")
	assertPathIsDir(t, repo, "notes")
	changelog := testfs.ReadFile(t, repo, brain.ChangelogPath)
	if strings.Count(changelog, "[migrate] [test]") != 2 {
		t.Fatalf("chained migration should append two ordered entries:\n%s", changelog)
	}
	if !strings.Contains(changelog, brain.VersionV1+" → "+brain.VersionV2) || !strings.Contains(changelog, brain.VersionV2+" → "+brain.Version) {
		t.Fatalf("missing chained migration steps:\n%s", changelog)
	}
	if err := verify.Check(repo, verify.Options{}); err != nil {
		t.Fatalf("verify chained v3 brain: %v", err)
	}
}

func TestUpdateScaffoldedAgentFileUsesKnownHashesOnly(t *testing.T) {
	repo := t.TempDir()
	const oldContent = "old scaffold\n"
	const newContent = "new scaffold\n"
	sum := sha256.Sum256([]byte(oldContent))
	template := initcmd.MigrationTemplate{
		Path:         "AGENTS.md",
		Content:      newContent,
		LegacySHA256: []string{fmt.Sprintf("%x", sum)},
	}
	if err := os.WriteFile(filepath.Join(repo, template.Path), []byte(oldContent), 0o640); err != nil {
		t.Fatal(err)
	}
	manual, err := updateScaffoldedAgentFile(repo, template)
	if err != nil {
		t.Fatal(err)
	}
	if manual || testfs.ReadFile(t, repo, template.Path) != newContent {
		t.Fatalf("known scaffold was not updated: manual=%v content=%q", manual, testfs.ReadFile(t, repo, template.Path))
	}
	info, err := os.Stat(filepath.Join(repo, template.Path))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("updated scaffold mode = %o, want 640", info.Mode().Perm())
	}

	const custom = "customized\n"
	if err := os.WriteFile(filepath.Join(repo, template.Path), []byte(custom), 0o640); err != nil {
		t.Fatal(err)
	}
	manual, err = updateScaffoldedAgentFile(repo, template)
	if err != nil {
		t.Fatal(err)
	}
	if !manual || testfs.ReadFile(t, repo, template.Path) != custom {
		t.Fatalf("custom scaffold was not preserved: manual=%v content=%q", manual, testfs.ReadFile(t, repo, template.Path))
	}
}

func TestMigrateLeavesCustomizedAgentFileUntouched(t *testing.T) {
	repo := newCurrentBrain(t)
	custom := "# Custom agent instructions\n"
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	downgradeFixture(t, repo, brain.VersionV2)
	noteSkillPath, _ := initcmd.NoteSkillTemplate()
	customNoteSkill := "custom note workflow\n"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, noteSkillPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, noteSkillPath), []byte(customNoteSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo, "--actor", "test"}); err != nil {
		t.Fatalf("migrate customized v2 brain: %v", err)
	}
	if got := testfs.ReadFile(t, repo, "AGENTS.md"); got != custom {
		t.Fatalf("migration overwrote customized AGENTS.md:\n%s", got)
	}
	if got := testfs.ReadFile(t, repo, noteSkillPath); got != customNoteSkill {
		t.Fatalf("migration overwrote customized note skill:\n%s", got)
	}
}

func TestMigrateRollsBackEveryTouchedFileOnFailure(t *testing.T) {
	repo := newCurrentBrain(t)
	braintest.RunWrite(t, repo, "# Raw\n\nEvidence.\n", "sources/raw.md", "--reason", "Preserve raw", "--actor", "test")
	braintest.RunWrite(t, repo, "# Existing\n\nKnowledge.\n", "wiki/existing.md",
		"--title", "Existing", "--summary", "Existing knowledge.", "--tag", "migration", "--source", "sources/raw.md",
		"--reason", "Create existing", "--actor", "test")
	downgradeFixture(t, repo, brain.VersionV2)
	wikiPath := filepath.Join(repo, "wiki/existing.md")
	wikiWithoutDate := removeLineContaining(testfs.ReadPath(t, wikiPath), "modified_date:")
	if err := os.WriteFile(wikiPath, []byte(wikiWithoutDate), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGeneratedVersion(t, repo, brain.VersionV2)
	beforeIndex := testfs.ReadFile(t, repo, brain.IndexPath)
	beforeTags := testfs.ReadFile(t, repo, brain.TagsPath)
	beforeWiki := testfs.ReadPath(t, wikiPath)
	cacheDir := searchindex.SearchIndexPath(repo)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheSentinel := filepath.Join(cacheDir, "preserve")
	if err := os.WriteFile(cacheSentinel, []byte("old cache state"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run([]string{"--brain", repo, "--actor", "test"})
	if err == nil {
		t.Fatal("migration unexpectedly succeeded with a non-empty search cache directory")
	}
	assertMarker(t, repo, brain.VersionV2)
	if _, err := os.Stat(filepath.Join(repo, "notes")); !os.IsNotExist(err) {
		t.Fatalf("new notes root was not rolled back: %v", err)
	}
	if got := testfs.ReadFile(t, repo, brain.IndexPath); got != beforeIndex {
		t.Fatalf("INDEX.md was not restored:\n%s", got)
	}
	if got := testfs.ReadFile(t, repo, brain.TagsPath); got != beforeTags {
		t.Fatalf("tags.md was not restored:\n%s", got)
	}
	if got := testfs.ReadPath(t, wikiPath); got != beforeWiki {
		t.Fatalf("legacy wiki modified_date repair was not rolled back:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(repo, brain.NotesIndexPath)); !os.IsNotExist(err) {
		t.Fatalf("new NOTES.md was not rolled back: %v", err)
	}
	noteSkillPath, _ := initcmd.NoteSkillTemplate()
	if _, err := os.Stat(filepath.Join(repo, noteSkillPath)); !os.IsNotExist(err) {
		t.Fatalf("installed note skill was not rolled back: %v", err)
	}
	if got := testfs.ReadPath(t, cacheSentinel); got != "old cache state" {
		t.Fatalf("old cache state was not preserved: %q", got)
	}
	if err := verify.CheckVersion(repo, brain.VersionV2, verify.Options{}); err != nil {
		t.Fatalf("rolled-back v2 brain is invalid: %v", err)
	}
}

func TestMigrateRollbackRestoresPreexistingNoteContent(t *testing.T) {
	repo := newCurrentBrain(t)
	downgradeFixture(t, repo, brain.VersionV2)
	if err := os.MkdirAll(filepath.Join(repo, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePolicy, ok := brain.PolicyForKind(brain.KindNote)
	if !ok {
		t.Fatal("missing note policy")
	}
	meta := frontmatter.New(string(brain.KindNote), "Preexisting note", "A note present before migration.", []string{"migration"}, nil, nil)
	content, err := frontmatter.AttachForPolicy(meta, notePolicy, "# Preexisting note\n\nOriginal content.\n")
	if err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(repo, "notes", "preexisting.md")
	if err := os.WriteFile(notePath, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	before := testfs.ReadPath(t, notePath)

	cacheDir := searchindex.SearchIndexPath(repo)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "preserve"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--brain", repo, "--actor", "test"}); err == nil {
		t.Fatal("migration unexpectedly succeeded with a non-empty search cache directory")
	}
	if got := testfs.ReadPath(t, notePath); got != before {
		t.Fatalf("preexisting note was not restored exactly:\n%s", got)
	}
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("restored note mode = %o, want 640", info.Mode().Perm())
	}
	assertMarker(t, repo, brain.VersionV2)
}

func TestCurrentCommandsRequestMigrationForV2(t *testing.T) {
	repo := newCurrentBrain(t)
	downgradeFixture(t, repo, brain.VersionV2)
	err := verify.Run(repo, verify.Options{})
	if err == nil || !strings.Contains(err.Error(), "lumbrera migrate") || strings.Contains(err.Error(), "stale") {
		t.Fatalf("verify error = %v, want migration instruction without stale claim", err)
	}
	if _, err := searchindex.CheckStatus(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "lumbrera migrate") {
		t.Fatalf("search status error = %v, want migration instruction", err)
	}
	if err := searchindex.RebuildBrain(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "lumbrera migrate") {
		t.Fatalf("search rebuild error = %v, want migration instruction", err)
	}
}

func newCurrentBrain(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "brain")
	if err := initcmd.Run([]string{repo}); err != nil {
		t.Fatalf("init fixture: %v", err)
	}
	return repo
}

func downgradeFixture(t *testing.T, repo, version string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(repo, "notes")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, brain.NotesIndexPath)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(repo, ".agents/skills/lumbrera-note")); err != nil {
		t.Fatal(err)
	}
	if version == brain.VersionV1 {
		if err := os.RemoveAll(filepath.Join(repo, "assets")); err != nil {
			t.Fatal(err)
		}
		for _, rel := range []string{brain.SourcesIndexPath, brain.AssetsIndexPath} {
			if err := os.Remove(filepath.Join(repo, rel)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	files, err := generate.FilesForRepoVersion(repo, version)
	if err != nil {
		t.Fatalf("generate %s fixture: %v", version, err)
	}
	if err := generate.WriteFilesForVersion(repo, version, files); err != nil {
		t.Fatalf("write %s fixture: %v", version, err)
	}
	if err := os.WriteFile(filepath.Join(repo, brain.MarkerPath), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify.CheckVersion(repo, version, verify.Options{}); err != nil {
		t.Fatalf("invalid %s fixture: %v", version, err)
	}
}

func writeGeneratedVersion(t *testing.T, repo, version string) {
	t.Helper()
	files, err := generate.FilesForRepoVersion(repo, version)
	if err != nil {
		t.Fatalf("generate %s files: %v", version, err)
	}
	if err := generate.WriteFilesForVersion(repo, version, files); err != nil {
		t.Fatalf("write %s files: %v", version, err)
	}
}

func removeLineContaining(content, value string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, value) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func assertMarker(t *testing.T, repo, want string) {
	t.Helper()
	got := strings.TrimSpace(testfs.ReadFile(t, repo, brain.MarkerPath))
	if got != want {
		t.Fatalf("VERSION = %q, want %q", got, want)
	}
}

func assertPathIsDir(t *testing.T, repo, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil || !info.IsDir() {
		t.Fatalf("%s is not a directory: info=%v err=%v", rel, info, err)
	}
}

func assertPathIsFile(t *testing.T, repo, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file: info=%v err=%v", rel, info, err)
	}
}

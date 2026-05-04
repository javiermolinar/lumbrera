package writecmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/generate"
	"github.com/javiermolinar/lumbrera/internal/git"
	md "github.com/javiermolinar/lumbrera/internal/markdown"
)

type options struct {
	Repo      string
	Target    string
	Reason    string
	Actor     string
	Title     string
	Summary   string
	Tags      []string
	Sources   []string
	Append    string
	AppendSet bool
	Delete    bool
	Help      bool
}

type operation string

const (
	opSource operation = "source"
	opCreate operation = "create"
	opUpdate operation = "update"
	opAppend operation = "append"
	opDelete operation = "delete"
)

func Run(args []string, stdin io.Reader) error {
	opts, err := parseArgs(args)
	if err != nil {
		printHelp()
		return err
	}
	if opts.Help {
		printHelp()
		return nil
	}

	repo, err := resolveRepo(opts.Repo)
	if err != nil {
		return err
	}
	if err := preflight(repo); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor, err = defaultActor(repo)
		if err != nil {
			return err
		}
	}
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	target, kind, err := normalizeTargetPath(opts.Target)
	if err != nil {
		return err
	}
	if err := ensureSafeFilesystemTarget(repo, target); err != nil {
		return err
	}

	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	exists, err := fileExists(absTarget)
	if err != nil {
		return err
	}

	op, err := inferOperation(kind, exists, opts)
	if err != nil {
		return err
	}
	if err := validateOptionsForOperation(repo, target, kind, exists, op, opts); err != nil {
		return err
	}

	var input []byte
	if op != opDelete {
		input, err = io.ReadAll(stdin)
		if err != nil {
			return err
		}
		if len(input) == 0 {
			return fmt.Errorf("write requires Markdown content on stdin")
		}
		if frontmatter.StartsWithFrontmatter(input) {
			return fmt.Errorf("stdin must contain Markdown body only; Lumbrera generates frontmatter")
		}
		if kind == "wiki" && hasSourcesSection(string(input)) {
			return fmt.Errorf("stdin must not contain a ## Sources section; Lumbrera generates it")
		}
	}

	base, err := currentHead(repo)
	if err != nil {
		return err
	}
	commitTime := time.Now()
	commitSubject := fmt.Sprintf("[%s] [%s]: %s", op, opts.Actor, opts.Reason)

	mutated := false
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if mutated {
			if rollbackErr := rollbackWrite(repo, base); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}

	mutated = true
	if err := applyMutation(repo, target, kind, op, opts, input); err != nil {
		return fail(err)
	}
	files, err := generate.FilesForRepoWithPending(repo, []generate.PendingChangelogEntry{{Date: commitTime, Subject: commitSubject}})
	if err != nil {
		return fail(err)
	}
	if err := generate.WriteFiles(repo, files); err != nil {
		return fail(err)
	}
	if err := validateDocuments(repo); err != nil {
		return fail(err)
	}
	if err := verifyGeneratedFiles(repo, []generate.PendingChangelogEntry{{Date: commitTime, Subject: commitSubject}}); err != nil {
		return fail(err)
	}

	if err := git.AddAll(repo); err != nil {
		return fail(err)
	}
	if err := git.Commit(repo, commitSubject); err != nil {
		return fail(err)
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return fail(err)
	}
	if !clean {
		return fail(fmt.Errorf("write committed but working tree is not clean"))
	}
	fmt.Printf("Committed Lumbrera write: %s\n", commitSubject)
	return nil
}

func parseArgs(args []string) (options, error) {
	var opts options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" || arg == "help" {
			opts.Help = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if opts.Target != "" {
				return options{}, fmt.Errorf("write accepts exactly one target path")
			}
			opts.Target = arg
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		nextValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--repo":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Repo = v
		case "--reason":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Reason = v
		case "--actor":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Actor = v
		case "--title":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Title = v
		case "--summary":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Summary = v
		case "--tag":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Tags = append(opts.Tags, v)
		case "--source":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Sources = append(opts.Sources, v)
		case "--append":
			v, err := nextValue()
			if err != nil {
				return options{}, err
			}
			opts.Append = v
			opts.AppendSet = true
		case "--delete":
			if hasValue {
				return options{}, fmt.Errorf("--delete does not accept a value")
			}
			opts.Delete = true
		default:
			return options{}, fmt.Errorf("unknown write option %s", name)
		}
	}
	if opts.Help {
		return opts, nil
	}
	if strings.TrimSpace(opts.Target) == "" {
		return options{}, fmt.Errorf("write requires a target path")
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return options{}, fmt.Errorf("write requires --reason")
	}
	return opts, nil
}

func resolveRepo(repo string) (string, error) {
	if strings.TrimSpace(repo) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root, err := git.WorkTreeRoot(cwd)
		if err == nil {
			repo = root
		} else {
			repo = cwd
		}
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func currentHead(repo string) (string, error) {
	result, err := git.Run(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(result.Stdout)
	if head == "" {
		return "", fmt.Errorf("git HEAD is empty")
	}
	return head, nil
}

func rollbackWrite(repo, base string) error {
	if strings.TrimSpace(base) == "" {
		return fmt.Errorf("cannot rollback write without a base commit")
	}
	if _, err := git.Run(repo, "reset", "--hard", base); err != nil {
		return err
	}
	_, err := git.Run(repo, "clean", "-fd", "--", "sources", "wiki", brain.IndexPath, brain.ChangelogPath, brain.BrainSumPath)
	return err
}

func preflight(repo string) error {
	if err := git.EnsureAvailable(); err != nil {
		return err
	}
	if !git.IsRepo(repo) {
		return fmt.Errorf("%s is not a Git worktree root", repo)
	}
	if err := brain.ValidateRepo(repo); err != nil {
		return err
	}
	clean, err := git.IsClean(repo)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree is not clean; run lumbrera sync or commit/revert unrelated changes before write")
	}
	if err := validateDocuments(repo); err != nil {
		return err
	}
	return verifyGeneratedFiles(repo, nil)
}

func validateDocuments(repo string) error {
	for _, dir := range []string{"sources", "wiki"} {
		root := filepath.Join(repo, dir)
		if _, err := os.Stat(root); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if err := filepath.WalkDir(root, func(absPath string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("%s is not a regular Markdown file", absPath)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular Markdown file", absPath)
			}
			rel, err := filepath.Rel(repo, absPath)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			return validateDocument(repo, absPath, rel, strings.TrimSuffix(dir, "s"))
		}); err != nil {
			return err
		}
	}
	return nil
}

func validateDocument(repo, absPath, relPath, wantKind string) error {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return fmt.Errorf("%s has invalid Lumbrera frontmatter: %w", relPath, err)
	}
	if !has {
		return fmt.Errorf("%s is missing Lumbrera-generated frontmatter", relPath)
	}
	if meta.Lumbrera.Kind != wantKind {
		return fmt.Errorf("%s frontmatter kind is %q; expected %q", relPath, meta.Lumbrera.Kind, wantKind)
	}
	analysis, err := md.Analyze(relPath, body)
	if err != nil {
		return fmt.Errorf("%s has invalid Markdown links: %w", relPath, err)
	}
	if analysis.FirstH1 != "" && analysis.FirstH1 != meta.Title {
		return fmt.Errorf("%s first H1 %q does not match generated title %q", relPath, analysis.FirstH1, meta.Title)
	}
	if err := validateInternalLinksExist(repo, relPath, analysis.Links); err != nil {
		return err
	}
	if !sameStrings(meta.Lumbrera.Links, filterWikiLinks(analysis.Links)) {
		return fmt.Errorf("%s frontmatter links are stale; regenerate through lumbrera write", relPath)
	}
	if wantKind == "source" {
		if len(meta.Lumbrera.Sources) > 0 {
			return fmt.Errorf("%s source frontmatter must not list provenance sources", relPath)
		}
		return nil
	}
	if len(analysis.Sources) == 0 {
		return fmt.Errorf("%s is missing a ## Sources section with source links", relPath)
	}
	for _, source := range analysis.Sources {
		if !strings.HasPrefix(source, "sources/") {
			return fmt.Errorf("%s Sources section must link only to sources/, got %s", relPath, source)
		}
	}
	if err := validateInternalLinksExist(repo, relPath, analysis.Sources); err != nil {
		return err
	}
	if !sameStrings(meta.Lumbrera.Sources, mergePaths(analysis.Sources)) {
		return fmt.Errorf("%s frontmatter sources are stale; regenerate through lumbrera write", relPath)
	}
	return nil
}

func verifyGeneratedFiles(repo string, pending []generate.PendingChangelogEntry) error {
	files, err := generate.FilesForRepoWithPending(repo, pending)
	if err != nil {
		return err
	}
	checks := map[string]string{
		brain.IndexPath:     files.Index,
		brain.ChangelogPath: files.Changelog,
		brain.BrainSumPath:  files.BrainSum,
	}
	for rel, want := range checks {
		got, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("generated file %s is missing: %w", rel, err)
		}
		if string(got) != want {
			return fmt.Errorf("generated file %s is stale; run lumbrera sync first", rel)
		}
	}
	return nil
}

func validateInternalLinksExist(repo, relPath string, links []string) error {
	for _, link := range mergePaths(links) {
		if link == "" {
			continue
		}
		if err := ensureSafeFilesystemTarget(repo, link); err != nil {
			return fmt.Errorf("%s links to unsafe path %s: %w", relPath, link, err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(link))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s links to missing file %s", relPath, link)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%s links to non-regular file %s", relPath, link)
		}
	}
	return nil
}

func normalizeTargetPath(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("target path is required")
	}
	raw = filepath.ToSlash(raw)
	if filepath.IsAbs(raw) || path.IsAbs(raw) {
		return "", "", fmt.Errorf("absolute target paths are not allowed")
	}
	if hasParentSegment(raw) {
		return "", "", fmt.Errorf("target path %q must not contain ..", raw)
	}
	if strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	clean := path.Clean(raw)
	if clean == "." || clean == "" {
		return "", "", fmt.Errorf("invalid target path %q", raw)
	}
	if strings.Contains(clean, "\\") {
		return "", "", fmt.Errorf("target path %q must use repo-relative POSIX separators", raw)
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return "", "", fmt.Errorf("target path %q must be a Markdown file", raw)
	}
	if strings.HasPrefix(clean, "sources/") && clean != "sources" {
		return clean, "source", nil
	}
	if strings.HasPrefix(clean, "wiki/") && clean != "wiki" {
		return clean, "wiki", nil
	}
	return "", "", fmt.Errorf("target path %q must be under sources/ or wiki/", raw)
}

func ensureSafeFilesystemTarget(repo, target string) error {
	repoResolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return err
	}
	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	if info, err := os.Lstat(absTarget); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink %s", target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target path %s is not a regular file", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ancestor := filepath.Dir(absTarget)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if errors.Is(err, os.ErrNotExist) {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				return fmt.Errorf("could not find existing parent for %s", target)
			}
			ancestor = parent
		} else {
			return err
		}
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return err
	}
	if !pathInside(repoResolved, resolvedAncestor) {
		return fmt.Errorf("target path %s resolves outside repo", target)
	}
	return nil
}

func inferOperation(kind string, exists bool, opts options) (operation, error) {
	if opts.AppendSet && opts.Delete {
		return "", fmt.Errorf("--append and --delete cannot be combined")
	}
	if opts.Delete {
		return opDelete, nil
	}
	if opts.AppendSet {
		return opAppend, nil
	}
	if kind == "source" {
		if exists {
			return "", fmt.Errorf("sources are immutable; refusing to update existing source")
		}
		return opSource, nil
	}
	if exists {
		return opUpdate, nil
	}
	return opCreate, nil
}

func validateOptionsForOperation(repo, target, kind string, exists bool, op operation, opts options) error {
	if err := validateCommitSubject(opts.Actor, opts.Reason); err != nil {
		return err
	}

	if op == opDelete {
		if !exists {
			return fmt.Errorf("cannot delete %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to delete existing source")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 || len(opts.Sources) > 0 {
			return fmt.Errorf("--delete cannot be combined with --title, --summary, --tag, or --source")
		}
		return nil
	}

	if kind == "source" {
		if len(opts.Sources) > 0 {
			return fmt.Errorf("source writes must not specify --source")
		}
		if op != opSource {
			return fmt.Errorf("sources are immutable; refusing to mutate existing source")
		}
	}

	if kind == "wiki" {
		if len(opts.Sources) == 0 {
			return fmt.Errorf("wiki writes require at least one --source")
		}
		if err := validateSourcePaths(repo, opts.Sources); err != nil {
			return err
		}
	}

	if (op == opSource || op == opCreate) && strings.TrimSpace(opts.Title) == "" {
		return fmt.Errorf("--title is required when creating a new file")
	}
	if op == opAppend {
		if !exists {
			return fmt.Errorf("cannot append to %s: file does not exist", target)
		}
		if kind == "source" {
			return fmt.Errorf("sources are immutable; refusing to append to existing source")
		}
		section := strings.TrimSpace(opts.Append)
		if section == "" {
			return fmt.Errorf("--append requires a non-empty section name")
		}
		if strings.EqualFold(section, "Sources") {
			return fmt.Errorf("--append cannot target the generated Sources section")
		}
		if opts.Title != "" || opts.Summary != "" || len(opts.Tags) > 0 {
			return fmt.Errorf("--append cannot change --title, --summary, or --tag in this version")
		}
	}
	return nil
}

func applyMutation(repo, target, kind string, op operation, opts options, input []byte) error {
	absTarget := filepath.Join(repo, filepath.FromSlash(target))
	switch op {
	case opDelete:
		return os.Remove(absTarget)
	case opSource:
		body := normalizeBody(input)
		return writeDocument(absTarget, target, kind, opts.Title, opts.Summary, opts.Tags, nil, body)
	case opCreate:
		body := normalizeBody(input)
		body = md.AppendSourcesSection(body, target, normalizeSources(opts.Sources))
		return writeDocument(absTarget, target, kind, opts.Title, opts.Summary, opts.Tags, normalizeSources(opts.Sources), body)
	case opUpdate:
		existingMeta, _, err := readExistingDocument(absTarget)
		if err != nil {
			return err
		}
		title := existingMeta.Title
		if strings.TrimSpace(opts.Title) != "" {
			title = opts.Title
		}
		summary := existingMeta.Summary
		if strings.TrimSpace(opts.Summary) != "" {
			summary = opts.Summary
		}
		tags := existingMeta.Tags
		if len(opts.Tags) > 0 {
			tags = opts.Tags
		}
		sources := mergePaths(existingMeta.Lumbrera.Sources, normalizeSources(opts.Sources))
		body := normalizeBody(input)
		body = md.AppendSourcesSection(body, target, sources)
		return writeDocument(absTarget, target, kind, title, summary, tags, sources, body)
	case opAppend:
		existingMeta, existingBody, err := readExistingDocument(absTarget)
		if err != nil {
			return err
		}
		sources := mergePaths(existingMeta.Lumbrera.Sources, normalizeSources(opts.Sources))
		body := md.RemoveSourcesSection(existingBody)
		body = md.AppendToSection(body, opts.Append, string(input))
		body = md.AppendSourcesSection(body, target, sources)
		return writeDocument(absTarget, target, kind, existingMeta.Title, existingMeta.Summary, existingMeta.Tags, sources, body)
	default:
		return fmt.Errorf("unsupported operation %q", op)
	}
}

func writeDocument(absTarget, target, kind, title, summary string, tags, sources []string, body string) error {
	analysis, err := md.Analyze(target, body)
	if err != nil {
		return err
	}
	if analysis.FirstH1 != "" && strings.TrimSpace(title) != "" && analysis.FirstH1 != strings.TrimSpace(title) {
		return fmt.Errorf("first H1 %q must match --title %q", analysis.FirstH1, strings.TrimSpace(title))
	}
	links := filterWikiLinks(analysis.Links)
	if kind == "source" {
		sources = nil
	}
	meta := frontmatter.New(kind, title, summary, tags, sources, links)
	content, err := frontmatter.Attach(meta, body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absTarget, []byte(content), 0o644)
}

func readExistingDocument(absPath string) (frontmatter.Document, string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	meta, body, has, err := frontmatter.Split(content)
	if err != nil {
		return frontmatter.Document{}, "", err
	}
	if !has {
		return frontmatter.Document{}, "", fmt.Errorf("existing document %s has no Lumbrera-generated frontmatter", absPath)
	}
	return meta, body, nil
}

func validateSourcePaths(repo string, sources []string) error {
	for _, source := range sources {
		normalized, kind, err := normalizeTargetPath(source)
		if err != nil {
			return fmt.Errorf("invalid --source %q: %w", source, err)
		}
		if kind != "source" {
			return fmt.Errorf("--source %q must be under sources/", source)
		}
		if err := ensureSafeFilesystemTarget(repo, normalized); err != nil {
			return fmt.Errorf("--source %q is unsafe: %w", source, err)
		}
		abs := filepath.Join(repo, filepath.FromSlash(normalized))
		info, err := os.Lstat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("--source %q does not exist", source)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("--source %q must be a regular Markdown file", source)
		}
	}
	return nil
}

func normalizeSources(sources []string) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		normalized, _, err := normalizeTargetPath(source)
		if err == nil {
			out = append(out, normalized)
		}
	}
	return mergePaths(out, nil)
}

func normalizeBody(input []byte) string {
	body := strings.ReplaceAll(string(input), "\r\n", "\n")
	return strings.Trim(body, "\n") + "\n"
}

func hasSourcesSection(body string) bool {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return md.RemoveSourcesSection(body) != strings.TrimRight(body, "\n")
}

func defaultActor(repo string) (string, error) {
	result, err := git.Run(repo, "config", "user.name")
	if err == nil {
		name := strings.TrimSpace(result.Stdout)
		if name != "" {
			return sanitizeActor(name), nil
		}
	}
	return "human", nil
}

func validateCommitSubject(actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.ContainsAny(reason, "\r\n") {
		return fmt.Errorf("--reason must be a single line")
	}
	if actor == "" {
		return nil
	}
	if strings.ContainsAny(actor, "]\r\n") {
		return fmt.Errorf("--actor must not contain ], carriage returns, or newlines")
	}
	return nil
}

func sanitizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	actor = strings.ReplaceAll(actor, "]", "")
	actor = strings.ReplaceAll(actor, "\n", " ")
	actor = strings.ReplaceAll(actor, "\r", " ")
	if actor == "" {
		return "human"
	}
	return actor
}

func fileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func pathInside(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hasParentSegment(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func mergePaths(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func filterWikiLinks(links []string) []string {
	var out []string
	for _, link := range links {
		if strings.HasPrefix(link, "wiki/") {
			out = append(out, link)
		}
	}
	return mergePaths(out)
}

func sameStrings(a, b []string) bool {
	a = mergePaths(a)
	b = mergePaths(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func printHelp() {
	fmt.Println(`Usage:
  lumbrera write <path> [options] < content.md

Performs one local Lumbrera write transaction and commits all content and generated metadata changes.

Required:
  <path>              repo-relative Markdown path under sources/ or wiki/
  --reason <reason>   single-line commit/changelog reason

Options:
  --repo <path>       target brain repo, default current Git worktree root
  --actor <actor>     actor label for commit subject, default Git user name or human
  --title <title>     required when creating a new file
  --summary <text>    optional generated frontmatter summary
  --tag <tag>         optional generated frontmatter tag, repeatable
  --source <path>     provenance source for wiki writes, repeatable
  --append <section>  append stdin content to a named section in an existing wiki page
  --delete            delete an existing wiki page

Rules:
  - stdin must contain Markdown body only; Lumbrera generates frontmatter
  - source files are immutable after creation
  - wiki writes require at least one --source
  - successful writes create exactly one Git commit
  - push/sync behavior is not implemented in this command yet`)
}

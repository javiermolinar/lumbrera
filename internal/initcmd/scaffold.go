package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	brainVersion        = "lumbrera-brain-v1"
	markerPath          = ".brain/VERSION"
	agentsPath          = "AGENTS.md"
	claudePath          = "CLAUDE.md"
	claudeSymlinkTarget = agentsPath
	agentsDir           = ".agents"
	claudeDir           = ".claude"
	claudeDirTarget     = agentsDir
)

var scaffoldDirs = []string{
	"sources",
	"wiki",
	".brain",
	".agents/skills/lumbrera-ingest",
	".agents/skills/lumbrera-query",
	".agents/skills/lumbrera-lint",
}

var scaffoldFiles = map[string]string{
	markerPath:       brainVersion + "\n",
	"INDEX.md":       indexContent,
	"CHANGELOG.md":   changelogContent,
	"BRAIN.sum":      brainSumContent,
	"tags.md":        tagsContent,
	".brain/ops.log": "",
	agentsPath:       agentsContent,
	".agents/skills/lumbrera-ingest/SKILL.md": ingestSkillContent,
	".agents/skills/lumbrera-query/SKILL.md":  querySkillContent,
	".agents/skills/lumbrera-lint/SKILL.md":   lintSkillContent,
}

var partialDirs = map[string]struct{}{
	".agents":                        {},
	".agents/skills":                 {},
	".agents/skills/lumbrera-ingest": {},
	".agents/skills/lumbrera-query":  {},
	".agents/skills/lumbrera-lint":   {},
	".brain":                         {},
	"sources":                        {},
	"wiki":                           {},
}

var partialFiles = map[string]struct{}{
	markerPath: {},
	".agents/skills/lumbrera-ingest/SKILL.md": {},
	".agents/skills/lumbrera-query/SKILL.md":  {},
	".agents/skills/lumbrera-lint/SKILL.md":   {},
	".brain/ops.log":                          {},
	agentsPath:                                {},
	claudeDir:                                 {},
	claudePath:                                {},
	"BRAIN.sum":                               {},
	"tags.md":                                 {},
	"CHANGELOG.md":                            {},
	"INDEX.md":                                {},
}

func ensureScaffold(repo string) error {
	for _, rel := range scaffoldDirs {
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(rel)), 0o755); err != nil {
			return err
		}
	}
	for rel, content := range scaffoldFiles {
		if err := writeExpectedFile(filepath.Join(repo, filepath.FromSlash(rel)), content); err != nil {
			return err
		}
	}
	if err := ensureSymlink(filepath.Join(repo, claudePath), claudeSymlinkTarget); err != nil {
		return err
	}
	return ensureSymlink(filepath.Join(repo, claudeDir), claudeDirTarget)
}

func writeExpectedFile(path, content string) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) != content {
			return fmt.Errorf("refusing to overwrite existing file %s with different content", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func ensureSymlink(path, target string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to overwrite existing file %s; expected symlink to %s", path, target)
		}
		current, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if current != target {
			return fmt.Errorf("refusing to replace existing symlink %s -> %s; expected %s", path, current, target)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Symlink(target, path)
}

func validateFreshBoilerplate(repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" {
			continue
		}
		if entry.IsDir() {
			return fmt.Errorf("refusing to initialize %s: existing directory %q is not Lumbrera boilerplate", repo, name)
		}
		if !isAllowedBoilerplateFile(name) {
			return fmt.Errorf("refusing to initialize %s: existing file %q is not Lumbrera boilerplate", repo, name)
		}
	}
	return nil
}

func validateEmptyDirectory(repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	entry := entries[0]
	kind := "file"
	if entry.IsDir() {
		kind = "directory"
	}
	return fmt.Errorf("refusing to initialize %s: existing %s %q is not in an empty directory or common boilerplate directory", repo, kind, entry.Name())
}

func validatePartialScaffold(repo string) error {
	return filepath.WalkDir(repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == repo {
			return nil
		}

		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if _, ok := partialDirs[rel]; ok {
				return nil
			}
			return fmt.Errorf("refusing to resume initialization in %s: existing directory %q is not part of a partial Lumbrera scaffold", repo, rel)
		}
		if _, ok := partialFiles[rel]; ok {
			switch rel {
			case markerPath:
				return validateMarkerFile(repo, path)
			case claudePath:
				return validateSymlink(repo, path, claudeSymlinkTarget)
			case claudeDir:
				return validateSymlink(repo, path, claudeDirTarget)
			default:
				return nil
			}
		}
		if !strings.Contains(rel, "/") && isAllowedBoilerplateFile(rel) {
			return nil
		}
		return fmt.Errorf("refusing to resume initialization in %s: existing path %q is not part of a partial Lumbrera scaffold", repo, rel)
	})
}

func validateMarkerFile(repo, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	marker := strings.TrimSpace(string(content))
	if marker != brainVersion {
		return fmt.Errorf("refusing to resume initialization in %s: unsupported Lumbrera marker %q", repo, marker)
	}
	return nil
}

func validateSymlink(repo, path, target string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to resume initialization in %s: %q is not a symlink", repo, filepath.Base(path))
	}
	current, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if current != target {
		return fmt.Errorf("refusing to resume initialization in %s: %q points to %q, expected %q", repo, filepath.Base(path), current, target)
	}
	return nil
}

func statusOnlyInitScaffold(status string) bool {
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if line == "" {
			continue
		}
		path := statusPath(line)
		if path == "" || !isInitStatusPath(path) {
			return false
		}
	}
	return true
}

func statusPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	path := line[3:]
	if i := strings.Index(path, " -> "); i >= 0 {
		path = path[i+4:]
	}
	return strings.TrimSuffix(filepath.ToSlash(path), "/")
}

func isInitStatusPath(path string) bool {
	if _, ok := partialFiles[path]; ok {
		return true
	}
	_, ok := partialDirs[path]
	return ok
}

func isAllowedBoilerplateFile(name string) bool {
	if name == ".gitignore" {
		return true
	}
	switch strings.ToUpper(name) {
	case "README", "README.MD", "LICENSE", "LICENSE.MD", "LICENSE.TXT", "COPYING", "COPYING.MD":
		return true
	default:
		return false
	}
}

const indexContent = `# Lumbrera Index

Generated by Lumbrera.

## Sources

No sources yet.

## Wiki

No wiki pages yet.
`

const changelogContent = `# Lumbrera Changelog

Generated from the Lumbrera operation log.

No Lumbrera writes yet.
`

const brainSumContent = `lumbrera-sum-v1 sha256
`

const tagsContent = `# Lumbrera Tags

Generated by Lumbrera from wiki frontmatter. Agents must not edit this file.

Wiki pages require 1-5 lowercase slug tags. Reuse existing tags when they fit.

No tags yet.
`

const agentsContent = `# Lumbrera Brain Agent Guide

This is a Lumbrera brain: a managed Markdown knowledge base for humans and LLM agents.

## Read

- You may read Markdown files directly.
- Use INDEX.md and tags.md for navigation, not as source evidence.
- Use .agents/skills/lumbrera-query/SKILL.md when answering questions.

## Write

- Do not create, edit, move, delete, or overwrite files directly.
- All mutations to sources/ and wiki/ must use lumbrera write.
- Do not modify existing files under sources/; sources are immutable.
- Do not edit generated files: INDEX.md, CHANGELOG.md, BRAIN.sum, or tags.md.
- Do not edit Lumbrera internals under .brain/, .agents/, or .claude.

## Wiki rules

- Wiki pages require a title, single-line summary, 1-5 lowercase slug tags, source references, and at most 400 Markdown body lines.
- Lumbrera generates document IDs, frontmatter, source sections, index, changelog, checksums, and the tag registry.
- Read tags.md before wiki writes and reuse existing tags when they fit.

## Commands

~~~sh
lumbrera verify --brain .
lumbrera write sources/<path>.md --reason "Preserve source" < source.md
lumbrera write wiki/<path>.md --title "Title" --summary "Summary" --tag tag --source sources/<path>.md --reason "Distill source" < page.md
~~~

## Skills

- Ingest sources into wiki pages: .agents/skills/lumbrera-ingest/SKILL.md
- Answer questions from the brain: .agents/skills/lumbrera-query/SKILL.md
- Check semantic health: .agents/skills/lumbrera-lint/SKILL.md
`

const ingestSkillContent = `---
name: lumbrera-ingest
description: Ingest a referenced Markdown source into a Lumbrera LLM Wiki by preserving the raw resource and adding distilled wiki knowledge through lumbrera write.
---

# Lumbrera Ingest

Use when the user asks to ingest, process, summarize, or integrate a raw source.

## Workflow

- Read the raw resource referenced by the user.
- Do not alter the raw resource.
- Preserve source granularity when possible; avoid aggregating unrelated source documents into one huge source unless necessary.
- If a combined source is already provided, preserve it as-is but write wiki pages by durable topic or task.
- Read INDEX.md, tags.md, and relevant existing wiki/ pages before writing, so new synthesis updates or complements existing knowledge instead of duplicating it.
- Inspect the source heading structure and identify durable topics.
- Create or update distilled Markdown documents under wiki/ with durable knowledge from the source.
- A good wiki page is atomic, source-grounded, useful without reopening the source, small, searchable, and linked when the relationship is real.
- Wiki pages have a hard maximum of 400 Markdown body lines. Split pages before they exceed the limit.
- Chunk large sources only for reading. Write wiki pages by durable topic, not by source chunk number. Do not create pages named like part-1 or part-2 unless the source itself is sequential knowledge.
- For large sources, produce a short ingest plan before writing: target wiki paths, create/update decisions, source sections covered, proposed title, mandatory single-line summary, and 1-5 tags.
- Prefer task-oriented synthesis pages when useful, not only topic summaries.
- Split when a draft covers multiple independent concepts, has sections that can be read independently, needs more than 5 tags, or becomes a grab bag.
- Avoid pages that are just "everything from source X", chunk dumps, giant mixed-topic notes, filler tags, unsupported synthesis, or duplicates of existing pages.
- For operational content, mark whether synthesized knowledge is public, internal, or mixed in the wiki body when that distinction is relevant.
- When ingesting troubleshooting or runbook material, prefer symptom → cause → fix tables where durable.
- Every wiki page needs a single-line --summary and 1-5 --tag values. Reuse existing lowercase slug tags from tags.md when they fit; create a new stable tag only when existing tags are clearly wrong. Do not invent filler tags.
- Prefer page-level provenance through --source. For large combined sources, add inline [source: ../sources/path.md#heading-anchor] citations to exact headings for important, surprising, version-sensitive, numeric, operational-limit, destructive, customer-impacting, or easily disputed claims.
- Provide wiki Markdown body content only. Do not create wiki document IDs, frontmatter, tag registry entries, index entries, changelog entries, checksums, or other generated metadata. Lumbrera owns those for wiki pages.
- Use lumbrera write to add the distilled document. For a new wiki file, pass --title, --summary, and 1-5 --tag flags. For wiki writes, pass --source. Always pass --reason.
- After writing, run lumbrera verify and report coverage: created or updated pages, covered source sections, skipped sections, uncertainties, and recommended follow-up pages.

## Required wiki linking pass

Before writing wiki content:

- Read INDEX.md and tags.md.
- Identify 3-7 existing wiki pages that may overlap, depend on this page, or act as prerequisites.
- Read the relevant existing wiki pages, or at least their title, summary, tags, and headings.
- Add contextual wiki-to-wiki links where the relationship is real: prerequisite, related task, deeper reference, operational follow-up, or contrasting behavior.
- Prefer contextual inline links over a large link dump.
- If contextual inline links would be awkward, add a short '## Related pages' section.
- Every new wiki page should have at least one wiki link unless it is genuinely standalone.
- If no wiki links are added, explain why in the final report.
- Do not add unrelated links just to satisfy the rule.
`

const querySkillContent = `---
name: lumbrera-query
description: Answer questions from a Lumbrera LLM Wiki by using the maintained wiki first and checking preserved Markdown sources when needed.
---

# Lumbrera Query

Use when the user asks a question about knowledge in the brain.

## Workflow

- Start with INDEX.md and tags.md to find candidate wiki/ pages. Use them for navigation, not evidence.
- If a user term is ambiguous, state the likely interpretations and either ask for clarification or answer with the assumed scope.
- Read the relevant wiki/ pages first.
- Check preserved sources/ documents when claims need verification.
- Do not infer frequency, priority, popularity, or prevalence unless the wiki/source explicitly supports it.
- When using internal/private operational sources, label the answer as internal-sourced and avoid presenting it as public documentation.
- Answer with citations to the wiki pages or source documents used.
- When asked, list the specific wiki/source files used.
- If the answer is durable knowledge worth keeping, ask whether to save it.
- Save only through lumbrera write. Do not create wiki frontmatter, tags.md entries, or generated metadata.
`

const lintSkillContent = `---
name: lumbrera-lint
description: Semantically health-check a Lumbrera LLM Wiki for stale synthesis, contradictions, unsupported claims, duplicated concepts, and useful follow-up questions.
---

# Lumbrera Lint

Use when the user asks for a semantic health check of the wiki.

Lumbrera handles deterministic consistency for managed wiki content: wiki document IDs, frontmatter, tag registry, index, changelog, checksums, source sections, broken links, heading anchors, path policy, and generated files. Do not spend LLM linting effort on those.

## Workflow

- Read the relevant wiki/ pages and their preserved sources/ documents.
- Look for semantic drift: stale claims, contradictions, synthesis that no longer matches sources, or claims not actually supported by cited sources.
- Identify high-risk claims that need claim-level citations: limits, breaking changes, destructive procedures, security/auth behavior, and internal operational workflows.
- Check whether internal-only knowledge is clearly marked and not presented as public documentation.
- Look for duplicated or fragmented concepts that should be merged or clarified.
- Identify important open questions or data gaps that need new sources.
- Report task-navigation gaps, such as missing troubleshooting quick references, FAQ-style pages, or symptom → cause → fix runbooks.
- Report findings with affected paths, evidence, and suggested next actions.
- If asked to fix semantic issues, use lumbrera write. Do not edit files directly or create wiki generated metadata, including tags.md entries.

## Semantic link health

- Read INDEX.md as the wiki map.
- Identify orphan or weakly connected wiki pages.
- Check whether related concepts are linked contextually.
- Look for pages that duplicate or depend on another page without linking to it.
- Suggest inline links or a short 'Related pages' section where useful.
- Do not report lack of links as a problem unless there is a clear semantic relationship.
`

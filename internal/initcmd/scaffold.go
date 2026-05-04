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

This repository is a Lumbrera brain: a backendless, Markdown-native implementation of Andrej Karpathy's LLM Wiki pattern.

The model:
- sources/ contains immutable raw Markdown source material.
- wiki/ contains LLM-maintained Markdown synthesis.
- Lumbrera owns the write boundary and deterministic bookkeeping.
- tags.md is a generated read-only tag registry derived from wiki frontmatter.

The goal is not one-shot RAG. The wiki is a persistent, compounding artifact that agents keep current by ingesting sources, answering questions, and checking semantic drift over time.

## Bundled skills

Use the matching bundled skill for the operation:

- .agents/skills/lumbrera-ingest/SKILL.md for ingesting a raw source into distilled wiki knowledge.
- .agents/skills/lumbrera-query/SKILL.md for answering questions from the maintained wiki and preserved sources.
- .agents/skills/lumbrera-lint/SKILL.md for semantic health checks: stale synthesis, contradictions, unsupported claims, duplicated concepts, and useful follow-up questions.

## Command protocol

Lumbrera is agent-driven. Prefer the bundled skills over inventing ad hoc shell workflows. The CLI commands are narrow protocol boundaries, not a broad human note-taking API:

- lumbrera write is the only mutation boundary for sources/ and wiki/ content.
- lumbrera verify is for deterministic diagnostics; it proves mechanical consistency, not semantic truth.
- lumbrera init is a human/admin setup command.

## What Lumbrera handles

Lumbrera handles deterministic and protocol work:

- wiki frontmatter and protocol metadata,
- required source references from write flags,
- INDEX.md, CHANGELOG.md, BRAIN.sum, and tags.md,
- wiki checksums, tag registry, and generated metadata,
- deterministic consistency such as paths, generated files, source immutability, broken links, and broken heading anchors.

## What agents handle

Agents handle semantic work:

- read Markdown sources and wiki pages,
- distill raw sources into durable wiki knowledge,
- synthesize answers from the maintained wiki,
- identify semantic drift between wiki pages and source material,
- suggest new questions or missing sources.

## Common workflow

1. Source arrives: the user adds or references a new Markdown source. Treat the raw source as immutable.
2. Ingest: use the ingest skill to read that source, distill durable knowledge into wiki/ content, and add it through lumbrera write. Lumbrera handles wiki frontmatter, source sections, tag registry, index, changelog, and wiki checksums.
3. Query: when the user asks questions, use the query skill. Start with INDEX.md as a map and tags.md as the tag registry, then read relevant wiki/ pages, then check sources/ when evidence is needed.
4. Lint periodically: use the lint skill from time to time to look for semantic drift only: stale synthesis, contradictions, unsupported claims, duplicated concepts, and missing source material.

## What to do

- Assume paths are relative to this repository root.
- For wiki writes, use Markdown body content only; let Lumbrera add protocol metadata and frontmatter.
- Read tags.md before wiki writes and reuse existing tags when they fit. Wiki pages require a single-line summary and 1-5 lowercase slug tags.
- For claim-level provenance, optionally add inline citations as [source: ../sources/path.md#heading-anchor]. Lumbrera validates the target file and heading anchor.
- Use lumbrera write for every mutation, supplying title, summary, tags, source path for wiki writes, and reason through CLI flags.
- Run lumbrera verify when deterministic consistency is in doubt.

## What not to do

- Do not create, edit, move, delete, or overwrite files directly.
- Do not modify existing source files under sources/.
- Do not create or maintain wiki frontmatter, INDEX.md, CHANGELOG.md, BRAIN.sum, tags.md, wiki checksums, tag registry, or generated metadata manually.
- Do not edit Lumbrera internals under .brain/, .agents/, or .claude.
- Do not rely on Git commands for Lumbrera knowledge bookkeeping.
- Do not spend LLM linting effort on deterministic consistency; Lumbrera handles that.

Git, cloud sync, backup, and sharing are external to Lumbrera. Use them only when explicitly instructed.

## Optional external guardrails

Humans may configure external tools such as Git hooks, GitHub Actions, CI jobs, backup checks, or sync services to run:

~~~sh
lumbrera verify --brain .
~~~

Those guardrails are defense-in-depth only. They do not replace lumbrera write, and they do not make direct edits safe. Agents must not create, edit, disable, or bypass external guardrails unless the user explicitly asks.

If generated files drift because someone edited managed wiki files directly, stop and report the verification error. The usual recovery is to restore the generated files or the whole brain from the user's external versioning/backup system, then retry through lumbrera write.
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
- Chunk large sources only for reading. Write wiki pages by durable topic, not by source chunk number. Do not create pages named like part-1 or part-2 unless the source itself is sequential knowledge.
- For large sources, produce a short ingest plan before writing: target wiki paths, create/update decisions, source sections covered, proposed title, mandatory single-line summary, and 1-5 tags.
- Prefer task-oriented synthesis pages when useful, not only topic summaries.
- For operational content, mark whether synthesized knowledge is public, internal, or mixed in the wiki body when that distinction is relevant.
- When ingesting troubleshooting or runbook material, prefer symptom → cause → fix tables where durable.
- Every wiki page needs a single-line --summary and 1-5 --tag values. Reuse existing lowercase slug tags from tags.md when they fit; create a new stable tag only when existing tags are clearly wrong. Do not invent filler tags.
- Prefer page-level provenance through --source. Add inline [source: ../sources/path.md#heading-anchor] citations only for important, surprising, version-sensitive, numeric, operational-limit, or easily disputed claims.
- Provide wiki Markdown body content only. Do not create wiki frontmatter, tag registry entries, index entries, changelog entries, checksums, or other generated metadata. Lumbrera owns those for wiki pages.
- Use lumbrera write to add the distilled document. For a new wiki file, pass --title, --summary, and 1-5 --tag flags. For wiki writes, pass --source. Always pass --reason.
- After writing, run lumbrera verify and report coverage: created or updated pages, covered source sections, skipped sections, uncertainties, and recommended follow-up pages.

## Wiki linking pass

Before writing wiki content:

- Read INDEX.md and tags.md.
- Identify existing wiki pages that overlap with the new source.
- Read the relevant existing wiki pages or at least their headings/summaries.
- Add wiki-to-wiki links where they help navigation or explain prerequisites.
- Prefer contextual inline links over a large link dump.
- If the page is mostly standalone, add a short '## Related pages' section.
- Do not force unrelated links.
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

Lumbrera handles deterministic consistency for managed wiki content: wiki frontmatter, tag registry, index, changelog, checksums, source sections, broken links, heading anchors, path policy, and generated files. Do not spend LLM linting effort on those.

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

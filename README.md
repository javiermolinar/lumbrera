# Lumbrera

Lumbrera is a backendless, Markdown-native second brain for humans and LLM agents.

It is inspired by the Karpathy [LLM Wiki pattern](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f): preserve raw source material, distill it into a durable human-readable Markdown wiki, and let agents help maintain that knowledge base over time.

<img width="1983" height="793" alt="0f75f597-5cc1-432e-a61a-ebc581baed22" src="https://github.com/user-attachments/assets/d7903531-149b-481e-b4c5-68004aa115dd" />


## What problem does it solve?

LLM agents are useful for summarizing, organizing, and updating knowledge, but they need a safe way to write durable shared memory. Plain folders of Markdown are easy to read and edit, but direct edits by humans and agents can drift, lose provenance, or silently overwrite important context. Hosted knowledge tools solve some of this with backends and product-specific workflows, but they can make the data less portable and harder to audit.

Lumbrera keeps the data as ordinary files and makes the CLI the mutation boundary. Agents may read Markdown directly, but durable changes go through `lumbrera write`, which applies path/provenance rules, regenerates metadata, and updates an internal operation log.


## Install

```sh
go install github.com/javiermolinar/lumbrera/cmd/lumbrera@main
```

The module root is not an installable command package; use `/cmd/lumbrera`.


## How to use it

Start by initializing a new brain:

```sh
lumbrera init ./brain
```

Then drop new markdown content into the sources folder. You can convert almost anything to markdown these days.
Ask your LLM to ingest it using the skill:

```
/skill:lumbrera-ingest @sources/whatever.md
```

Start asking questions using the skill:

```
/skill:lumbrera-query how can I do X or Y?
```

The query skill starts with the local SQLite search index:

```sh
lumbrera search "how can I do X or Y?" --brain ./brain --json
```

`lumbrera search` automatically rebuilds a missing or stale local index. To inspect or force the disposable cache explicitly:

```sh
lumbrera index --status --brain ./brain
lumbrera index --rebuild --brain ./brain
```

From time to time, run the health skill to review semantic maintenance candidates:

```
/skill:lumbrera-health
```


## Source tiers

Not all sources are equal. Lumbrera infers a tier from the path and uses it to rank search results:

| Tier | Path prefix | Ranking | Use for |
|---|---|---|---|
| canonical | `sources/` `wiki/` | default (1.0) | Current product docs, operations, reference |
| design | `sources/design/` `wiki/design/` | demoted (0.45 penalty) | Proposals, ADRs, specs not yet implemented |
| reference | `sources/reference/` | demoted (0.60 penalty) | Historical docs, competition, meeting notes |

Canonical content ranks first in search. Design and reference content is still findable but structurally deprioritized. Use `--tier` to filter:

```sh
lumbrera search "querier batching" --tier design --json
```

When ingesting a design doc, preserve under `sources/design/` and create wiki pages under `wiki/design/`. The LLM sees the tier label in search results and naturally prefers canonical answers for operational questions.

## Goals

The goal is simple: a way to summarize content so both humans and agents can benefit from it.
Lumbrera is not trying to be a new chat UI or a full knowledge-management app. It is a small protocol and CLI boundary for maintaining source-grounded Markdown knowledge safely in local files. Git, cloud sync, backups, and sharing are external choices.

## Commands

Agents use the generated `AGENTS.md` and bundled skills. The core protocol is intentionally small:

- `lumbrera search "<query>" --brain <path> --json` searches wiki synthesis and preserved Markdown sources with a deterministic local SQLite/FTS5 index. Optional `--tag <tag>`, `--source <sources/path.md>`, and `--tier <canonical|design|reference>` filters narrow results. Output treats `recommended_sections` as the primary agent read plan, with section reasons, `agent_instructions`, entity `coverage`, ranked raw hits, snippets, tags, tier, sources, links, `recommended_read_order`, and a stop rule.
- `lumbrera health --brain <path> --json` returns deterministic health/consolidation review candidates for LLM review. Candidates are not conclusions; they identify pages or sources worth reading for possible links, consolidation, stale-risk, orphan pages, or source coverage gaps.
- `lumbrera index --status --brain <path>` reports whether `.brain/search.sqlite` is missing, fresh, stale, or incompatible without mutating files.
- `lumbrera index --rebuild --brain <path>` verifies the brain and rebuilds `.brain/search.sqlite` as a disposable cache.
- `lumbrera write ...` is the only supported mutation boundary for `sources/`, `wiki/`, and generated metadata such as `tags.md`.
- `lumbrera verify --brain <path>` repairs missing wiki document IDs for backward compatibility, then checks deterministic integrity for managed wiki content: provenance, links, generated files, and checksums. Raw files under `sources/` are not required to have Lumbrera frontmatter.
- `lumbrera init <path>` creates a brain scaffold and a `.gitignore` entry for `.brain/search.sqlite*`. It does not initialize Git, install hooks, commit, or push.

## Guardrails

Lumbrera does not own Git or GitHub. If a brain is stored in Git, humans can optionally add hooks, GitHub Actions, CI checks, or branch protection that run:

```sh
lumbrera verify --brain .
```

These guardrails are defense-in-depth. They catch drift in managed wiki content and generated metadata, but they do not replace `lumbrera write`. If generated files drift, restore them from your external versioning or backup system, then retry the Lumbrera operation.

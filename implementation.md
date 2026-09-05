# Policy-first notes implementation

Status: draft

## Goal

Add `notes/` as a first-class Markdown content type for durable first-party knowledge.

A note does not need an upstream source because the note is evidence itself. A wiki page may use either an imported source or a note as provenance. The implementation should preserve Lumbrera's deterministic write, delete, verify, and search guarantees without duplicating the wiki implementation.

This design starts from the stable pre-notes behavior. It does not use the experimental notes implementation as a base. The implementation should be derived from the policy and acceptance criteria in this document.

## Design principles

1. **Policy before branches.** Content behavior comes from one typed policy table, not repeated string comparisons across packages.
2. **One managed-document path.** Wiki pages and notes share frontmatter, body validation, links, tags, writes, and verification.
3. **Evidence is broader than imported sources.** A wiki may be backed by `sources/` or `notes/`.
4. **Canonical and evidentiary content remain distinct.** Wiki pages are synthesized answers; notes are first-party evidence.
5. **`all` means all.** Default search includes every searchable content kind and ranks canonical wiki content first.
6. **Every successful mutation leaves a valid brain.** Link cleanup, evidence cleanup, generated files, and search freshness must agree.
7. **Version closed contracts.** Brain and SQLite schema versions change when their accepted kinds or generated contracts change.
8. **Prefer the smallest complete behavior.** No note-specific command, graph UI, promotion workflow, or semantic freshness engine in the first release.

## Content model

| Root | Kind | Storage | Mutable | Required upstream evidence | Can back a wiki | Searchable | Generated catalog |
|---|---|---|---:|---:|---:|---:|---|
| `sources/` | `source` | raw Markdown | no | none | yes | yes | `SOURCES.md` |
| `notes/` | `note` | managed Markdown | yes | none | yes | yes | `NOTES.md` |
| `wiki/` | `wiki` | managed Markdown | yes | at least one source or note | no | yes | `INDEX.md` |
| `assets/` | `asset` | binary | no | none | no | no | `ASSETS.md` |

### Source

An imported or preserved external document. Sources remain raw and immutable.

### Note

A concise first-party document containing a durable observation, decision, failed experiment, local convention, or other knowledge that has no external document to ingest.

A note:

- has generated frontmatter, title, summary, tags, ID, and modified date;
- requires no `--source`;
- must not contain a generated `## Sources` section;
- may link normally to notes, wiki pages, sources, and assets;
- may be used as evidence for a wiki page;
- is mutable through the same managed-document update and append paths as a wiki page.

A wiki reference points to the current note, not a historical note revision. Git and `CHANGELOG.md` retain change history. Version-pinned note evidence and automatic stale-wiki detection are deferred.

### Wiki

Canonical synthesized knowledge. The resulting wiki document must always have at least one evidence reference. Accepted evidence kinds are:

- `source`
- `note`

A wiki page cannot use another wiki page as provenance. Wiki-to-wiki relationships remain ordinary Markdown links.

### Asset

Unchanged by this feature.

## Terminology and compatibility

Use **evidence** internally for anything that can support a wiki page. Preserve the existing public and serialized names for compatibility:

- CLI flag: `--source`
- frontmatter field: `lumbrera.sources`
- generated section: `## Sources`

After this change, those locations may contain paths under either `sources/` or `notes/`.

Example:

```sh
lumbrera write wiki/downscaling.md \
  --title "Downscaling" \
  --summary "Observed downscaling behavior." \
  --tag operations \
  --source notes/failed-downscale.md \
  --reason "Synthesize first-party evidence" < wiki.md
```

Renaming the public contract to `--evidence` and `lumbrera.evidence` would require a larger migration without improving the initial capability. Do not include that rename in this implementation.

## Central content policy

Create one dependency-light policy definition under `internal/brain`.

The exact Go representation may vary, but it must express only behavior used by the implementation:

```go
type Kind string

const (
    KindSource Kind = "source"
    KindNote   Kind = "note"
    KindWiki   Kind = "wiki"
    KindAsset  Kind = "asset"
)

type StorageMode uint8

const (
    StorageRawMarkdown StorageMode = iota
    StorageManagedMarkdown
    StorageBinary
)

type ContentPolicy struct {
    Root             string
    Kind             Kind
    Storage          StorageMode
    RequiredRoot     bool
    Mutable          bool
    RequiresEvidence bool
    EvidenceKinds    []Kind
    ProvidesEvidence bool
    CatalogPath      string
}
```

Required helpers:

```go
func PolicyForKind(Kind) (ContentPolicy, bool)
func PolicyForPath(string) (ContentPolicy, bool)
func ManagedPolicies() []ContentPolicy
func MarkdownPolicies() []ContentPolicy
func ManagedRoots() []string
func SearchRoots() []string
func IsManagedKind(Kind) bool
func CanUseAsEvidence(documentKind, evidenceKind Kind) bool
```

Rules:

- `source`: raw Markdown, immutable, provides evidence.
- `note`: managed Markdown, mutable, zero evidence, provides evidence.
- `wiki`: managed Markdown, mutable, requires evidence, accepts source and note.
- `asset`: binary, immutable.

Do not add speculative policy flags. Add a policy field only when at least two callers derive behavior from it.

All packages should consume typed kinds or policy helpers. Avoid adding new checks shaped like:

```go
kind == "wiki" || kind == "note"
```

## Managed-document contract

Wiki pages and notes use one managed-document implementation.

Shared validation:

- generated document ID is present and globally unique across wiki and notes;
- frontmatter kind matches the path policy;
- title is present;
- summary is a non-empty single line;
- one to five valid tags are present;
- modified date is present and valid;
- first H1, when present, matches the title;
- body is at most 400 lines;
- local paths and anchors resolve;
- generated managed-document links are current;
- evidence count and evidence kinds satisfy policy.

Policy differences:

- a note has no evidence and no `## Sources` section;
- a wiki has at least one source or note and a generated `## Sources` section.

Implement one verifier parameterized by `ContentPolicy`. Do not create parallel `validateWikiDocument` and `validateNoteDocument` functions containing the same mechanics.

## Write behavior

Use the existing `lumbrera write` command. Do not add a `lumbrera note` command.

### Create a note

```sh
lumbrera write notes/failed-downscale.md \
  --title "Failed downscale while ring retained ownership" \
  --summary "Downscaling failed while the ingester ring still owned the tenant." \
  --tag operations \
  --reason "Record first-party downscale observation" < note.md
```

Rules:

- `--title`, `--summary`, and at least one `--tag` are required on create;
- `--source` is rejected for notes;
- input contains body Markdown only;
- frontmatter is generated;
- a `## Sources` section is rejected;
- update and append use the shared managed-document path.

### Create or update a wiki using a note

```sh
lumbrera write wiki/downscaling.md \
  --title "Downscaling" \
  --summary "How tenant ownership affects ingester downscaling." \
  --tag operations \
  --source notes/failed-downscale.md \
  --reason "Create guidance from first-party evidence" < wiki.md
```

Rules:

- wiki evidence paths may resolve to `source` or `note`;
- direct self-reference is rejected;
- creation requires at least one evidence path;
- evidence retention is cumulative by design: updates, appends, and full-body replacements retain every existing evidence path without repeating `--source`, even when the new body no longer cites it;
- body replacement is not evidence removal; the write workflow has no evidence-removal operation in this phase;
- supplying `--source` adds normalized evidence to the existing set;
- the final document, rather than the argument list, is validated against the policy's evidence requirement;
- `## Sources` is generated from the resulting evidence set.

### Mutation implementation

Use one managed-document mutation flow:

1. Load existing metadata when updating.
2. Resolve the policy from the target path.
3. Merge title, summary, tags, links, and evidence according to the operation.
4. Validate evidence with `CanUseAsEvidence`.
5. Generate or remove the Sources section according to policy.
6. Generate frontmatter.
7. Write through the existing transaction and rollback boundary.

Do not add note branches separately to create, update, and append.

## Links and references

Distinguish two relationships:

1. **Evidence reference** — stored in `lumbrera.sources` and the generated `## Sources` section.
2. **Ordinary link** — a Markdown link between content documents.

Generated `lumbrera.links` should include managed-document destinations under both `wiki/` and `notes/`. Rename helpers such as `filterWikiLinks` to describe managed links and derive allowed roots from policy.

Verification must validate ordinary local links from both wiki pages and notes to any managed content path or asset.

The search relationship index must record note links with kind `note`.

## Delete behavior

Deletion must plan against all managed documents, not wiki pages alone.

Replace wiki-specific reference structures with a managed-document representation containing path, policy, metadata, and body.

### Delete a note

1. Find wiki pages that use the note as evidence.
2. Remove the note from their generated evidence metadata and Sources section.
3. Delete a dependent wiki page if it has no remaining evidence.
4. Remove ordinary links to the note from surviving wiki pages and notes.
5. Delete the note.
6. Regenerate derived files and verify atomically.

### Delete a source

Existing source deletion semantics remain, except evidence consumers are selected by policy. Notes cannot use sources as evidence in this initial model, so only wiki evidence is affected.

### Delete a wiki page

Remove ordinary inbound links from both wiki pages and notes before deleting it.

### Delete an asset

Scrub asset references from both wiki pages and notes. Asset deletion never cascade-deletes a managed document.

### Generic cascade rule

After removing evidence from a managed document:

```text
if policy.RequiresEvidence and remaining evidence count is zero:
    cascade-delete the document
else:
    rewrite it with regenerated metadata
```

Only wiki currently requires evidence, but the algorithm must use policy rather than a wiki string check.

The entire delete plan must be calculated before mutation so the backup includes every touched file.

## Search behavior

Notes are indexed in the same SQLite database as sources and wiki pages.

Required behavior:

- add `note` to the document, section, and link kind schema constraints;
- increment the SQLite schema version;
- index `sources/`, `notes/`, and `wiki/` from policy-derived roots;
- support `--kind note`;
- preserve `--kind all` as all searchable kinds;
- default search includes notes;
- rank wiki results before notes, and notes before raw sources when relevance is otherwise comparable;
- when no wiki result exists but a note does, recommend note sections rather than incorrectly falling back to source recommendations;
- note results expose empty `sources` and normal tags and links.

The SQLite index is disposable. Migration should invalidate the old cache so the next search creates the new schema.

## Health behavior

Health checks should follow semantic capabilities rather than hard-coded kinds.

Initial scope:

- broken-link checks include wiki and note documents;
- duplicate checks may compare wiki and notes so an already-canonicalized note can be identified;
- tag and stub checks include wiki and notes;
- source-coverage checks remain about imported sources and wiki coverage;
- zero-evidence orphan checks apply only to policies that require evidence;
- asset checks remain unchanged.

Automatic detection that a wiki may be stale after a referenced note changes is deferred.

## Generated files

Generated navigation remains separated by role:

- `INDEX.md` — canonical wiki pages;
- `SOURCES.md` — imported sources;
- `NOTES.md` — first-party notes;
- `ASSETS.md` — assets.

`NOTES.md` should use the same deterministic tree generation infrastructure as the other catalogs and include note title and summary. Do not copy the index generator into a note-specific implementation.

Other generated behavior:

- `BRAIN.sum` includes managed wiki and note documents;
- `tags.md` counts tags from wiki and note documents;
- generated-file backup and verification include `NOTES.md`;
- static generated text must be versioned through brain migration rather than making existing brains unexpectedly stale.

## Brain and schema versions

This feature changes accepted content roots, frontmatter kinds, generated files, and SQLite constraints. Treat it as a format change.

- bump the brain marker from `lumbrera-brain-v2` to `lumbrera-brain-v3`;
- bump the SQLite search schema from version 3 to version 4;
- keep the current frontmatter shape and `document-v1` marker because fields do not change; the brain marker gates support for the new kind;
- old binaries must reject v3 clearly instead of partially processing notes.

## Migration

Extend `lumbrera migrate` to migrate any supported older brain to the current version through ordered steps.

The v2 to v3 migration must:

1. acquire the brain lock;
2. verify the v2 brain before mutation;
3. create `notes/`;
4. create `NOTES.md`;
5. regenerate `INDEX.md`, `SOURCES.md`, `NOTES.md`, `ASSETS.md`, `BRAIN.sum`, and `tags.md`;
6. update `VERSION` to v3;
7. install the note skill when the target path is absent;
8. update scaffolded agent instructions only when the existing file exactly matches the known v2 template;
9. leave user-modified agent files untouched and report the required manual update;
10. remove the disposable SQLite cache;
11. append one migration changelog entry;
12. run v3 verification;
13. roll back every touched file on failure.

A v1 brain should migrate through v2 and v3 steps in one command.

Mutating commands must reject older brain versions with a migration instruction. Read-only compatibility should not silently claim support for content contracts the binary does not understand.

## Init and product-facing instructions

New brains initialize directly as v3 with:

- `notes/`;
- `NOTES.md`;
- `.agents/skills/lumbrera-note/SKILL.md`;
- updated `AGENTS.md` and query/delete skills.

### Note skill

The note skill should:

1. identify one durable first-party fact;
2. search all content for existing coverage;
3. read only recommended sections;
4. skip when wiki or an existing note already covers the fact;
5. create or update one focused note;
6. verify the brain;
7. report what was created, updated, or skipped.

Guardrails:

- no transcript dumps or session recaps;
- no source ingestion through the note skill;
- no generated frontmatter or Sources section in input;
- use ordinary links to connect related notes;
- prefer zero notes over low-value notes.

### Ingest/query/delete skills

- Ingest documents into `sources/` and wiki as before.
- Query searches notes by default but prioritizes wiki recommendations.
- Query may suggest synthesizing durable notes into a wiki page backed by those notes.
- Delete explains note evidence cascade and managed-link cleanup.

## Implementation phases

### Phase 1: Policy refactor with no behavior change — Implemented

- Add typed kinds and `ContentPolicy` under `internal/brain`.
- Represent the existing source, wiki, and asset behavior.
- Replace duplicated path and kind classification with policy helpers.
- Derive managed and searchable roots from policy.
- Keep all existing tests green and generated output byte-for-byte unchanged.

This phase must land independently. It proves the policy describes existing behavior before adding notes.

### Phase 2: Generic managed documents and evidence — Implemented

- Parameterize frontmatter validation by policy.
- Replace wiki-only document verification with managed-document verification.
- Replace repeated create/update/append branches with one managed mutation flow.
- Validate final evidence cardinality instead of requiring flags mechanically.
- Generalize managed links and evidence path validation.
- Preserve existing wiki behavior.

### Phase 3: Add the note policy end to end

- Add `notes/` and kind `note` to policy.
- Implement note write, update, append, verify, manifest, tags, and `NOTES.md`.
- Allow wiki evidence under `notes/`.
- Add v3 init behavior and generated templates.

Do not merge this phase until a note can be created and a wiki backed only by that note passes verification.

### Phase 4: Reference graph and deletion

- Load every managed document for cascade planning.
- Track ordinary links to wiki and notes.
- Treat source and note paths as valid wiki evidence.
- Implement note evidence deletion and managed-link cleanup.
- Extend asset cleanup to notes.
- Add rollback tests covering every touched kind.

### Phase 5: Search and health

- Bump the SQLite schema.
- Index notes and support note filtering.
- Make default/all search include notes with wiki-first ranking.
- Generalize recommendation fallback by available kind.
- Extend applicable health checks to managed notes.

### Phase 6: Migration, skills, and documentation

- Implement v2 to v3 migration and chained v1 migration.
- Safely update scaffolded files.
- Add the note skill and update query/delete instructions.
- Update CLI help, README, format descriptions, and examples.
- Remove stale wording that describes `BRAIN.sum`, tags, search, or verification as wiki-only where it now covers managed documents.

## Minimality guardrails

Do not introduce:

- a separate `lumbrera note` command;
- a note-only database or search command;
- copied note versions of wiki validation or mutation functions;
- hidden notes excluded from default `all` search;
- a second provenance field alongside `lumbrera.sources`;
- automatic note-to-wiki promotion;
- global graph visualization;
- semantic stale-content automation;
- direct repository edits outside the Lumbrera mutation boundary.

Do not claim that one policy row automatically implements a new kind. The policy centralizes semantics; each subsystem still needs explicit tests proving it honors those semantics.

## Acceptance tests

### Policy

- Every recognized root resolves to exactly one policy.
- Managed roots are wiki and notes.
- Search roots are sources, wiki, and notes.
- Wiki requires evidence and accepts source and note evidence.
- Note neither requires nor accepts evidence.
- Unknown kinds and roots fail closed.

### Write and verify

- Create a note without `--source`; verify passes.
- Reject a note with `--source`.
- Reject a note containing `## Sources`.
- Update and append to an existing note; ID remains stable and modified date updates.
- Create a wiki backed only by a note; verify passes.
- Create a wiki backed by a source and a note; verify passes.
- Reject a wiki backed by a wiki path.
- Reject missing or unsafe evidence paths.
- Reject stale note frontmatter links, tags, IDs, and modified dates.
- Reject duplicate IDs across wiki and notes.

### Delete

- Delete an unreferenced note.
- Delete a note used as one of several wiki evidence paths; wiki survives and is regenerated.
- Delete a note used as the only wiki evidence; wiki cascade-deletes.
- Delete a note linked from another note; the inbound link is cleaned.
- Delete a wiki linked from a note; the note is cleaned.
- Delete an asset referenced by a note; the note is cleaned but survives.
- Any post-mutation failure restores notes, dependent documents, generated files, and changelog.

### Search and health

- Default search returns matching notes.
- Explicit `--kind all` returns matching wiki, note, and source documents.
- `--kind note` returns only notes.
- Wiki ranks ahead of a comparably relevant note; note ranks ahead of a raw source.
- A note-only result produces note recommendations.
- Index freshness changes after note create, update, or delete.
- Applicable duplicate, link, tag, and stub checks include notes.

### Generated files and migration

- `NOTES.md` is deterministic and note-only.
- `INDEX.md` remains wiki-only.
- `BRAIN.sum` includes wiki and notes.
- `tags.md` includes wiki and note tags.
- A v2 fixture migrates to v3 and verifies.
- A v1 fixture migrates through v2 to v3 and verifies.
- Migration does not overwrite user-modified agent files.
- An old search cache is invalidated and rebuilt with schema version 4.
- A pre-migration v2 brain does not become stale merely by running the new binary; mutating commands instead request migration.

## Completion criteria

The implementation is complete when:

1. all acceptance tests pass;
2. `go test ./...`, `go vet ./...`, and `git diff --check` pass;
3. a temporary v3 brain supports this complete workflow:

   ```text
   init
   → write note without source
   → write wiki using note as source
   → search and find both, wiki first
   → update note
   → delete note
   → cascade or preserve dependent wiki according to remaining evidence
   → verify
   ```

4. no notes-specific implementation duplicates the managed wiki pipeline;
5. old brain and search formats fail or migrate explicitly rather than being interpreted ambiguously.

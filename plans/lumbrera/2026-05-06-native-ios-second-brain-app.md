# Native iOS Lumbrera Second Brain App Plan

## Context
The product direction is a native iOS app built on top of Lumbrera, but Lumbrera itself remains invisible to users. The app should feel closer to Google NotebookLM than Obsidian: users import sources, ask grounded questions, search the knowledge base, and build a durable second brain. Lumbrera provides the hidden Markdown/provenance/search/write layer.

## Goals
- Build a native SwiftUI app for a Lumbrera-backed second brain.
- Provide a Markdown wiki viewer with a knowledge tree/navigation view.
- Let users drop/import PDFs and other documents.
- Convert imported documents into Markdown sources for indexing and citation.
- Provide deterministic FTS search across wiki and sources.
- Provide a chat box for grounded Q&A over the brain.
- Support collaboration through Git-backed history and sync, hidden behind product UX.
- Keep the user-facing model focused on brains, sources, wiki knowledge, search, and chat.

## Constraints
- Users should never need to interact with the Lumbrera CLI directly.
- This should not be positioned as an Obsidian clone or manual Markdown editor.
- Git should be mostly invisible, surfaced as sync/history/activity rather than raw repo operations.
- Lumbrera remains the mutation boundary for durable wiki/source changes.
- Real-time Google Docs-style editing is not required for the initial version.
- PDFs may need original binary storage outside Lumbrera-managed Markdown, with extracted Markdown stored as sources.

## Proposed plan
1. Define the user-facing product model:
   - Brain
   - Sources
   - Wiki
   - Search
   - Ask/chat
   - Activity/history
2. Build the SwiftUI shell:
   - Sidebar/tree for wiki navigation.
   - Main Markdown reader.
   - Search screen with snippets and filters.
   - Chat panel or bottom composer.
   - Import/drop/share entry points for PDFs and Markdown.
3. Add a hidden Lumbrera adapter layer:
   - `search(query)` maps to `lumbrera search --json` or an engine API.
   - `writeSource(...)` maps imports into `sources/*.md` through Lumbrera.
   - `writeWiki(...)` persists generated/saved knowledge through Lumbrera.
   - `verify()` and `index()` run behind sync/import flows.
4. Start with a backend-managed implementation:
   - iOS app talks to a small API.
   - Backend owns Lumbrera execution, indexing, LLM calls, PDF extraction/OCR, and Git operations.
   - This avoids iOS executable/process restrictions and simplifies collaboration.
5. Implement import flow:
   - User drops or selects a PDF.
   - Extract text with PDFKit/OCR where needed.
   - Convert extracted content to Markdown.
   - Store original file as an attachment if needed.
   - Store extracted Markdown as a Lumbrera source.
   - Rebuild/update search index.
6. Implement search flow:
   - Expose Lumbrera FTS search as a first-class app feature.
   - Show wiki/source hits, snippets, paths, tags, and section anchors.
   - Allow opening results directly in the Markdown viewer.
7. Implement grounded chat flow:
   - User asks a question.
   - Backend runs Lumbrera search.
   - LLM answers using recommended sections/sources.
   - Response includes citations back to source/wiki material.
   - User can save useful answers or summaries back into the brain.
8. Implement Git-backed collaboration:
   - One Git repo per shared brain.
   - Backend serializes writes per brain.
   - Each import/wiki update becomes a commit.
   - Pull before write, verify after write, push after commit.
   - Surface this as sync status, activity feed, history, and restore.
9. Later evaluate offline/on-device support:
   - Refactor Lumbrera into a reusable engine API if needed.
   - Consider `gomobile`/XCFramework only after the backend-backed product is validated.

## Open questions
- Should the first version be iPad-first, iPhone-first, or universal?
- Which Git provider should be supported first: GitHub, GitLab, Gitea, or provider-agnostic SSH/HTTPS?
- Should original PDFs live in Git LFS, object storage, or local/device storage?
- What LLM provider should power chat and wiki synthesis initially?
- Should users be allowed to manually edit wiki Markdown, or only save generated/suggested updates?
- What is the initial collaboration model: shared team brain, personal brain with invite, or workspace/project brains?

## Next steps
- Decide whether v1 is backend-backed only or must support offline local operation.
- Sketch the core iOS screens: Library/Brain, Wiki, Sources, Search, Ask, Activity.
- Define the backend API contract around import, search, chat, page read, and save-to-brain.
- Define the storage layout for PDFs, extracted Markdown sources, and Lumbrera wiki pages.
- Prototype one end-to-end flow: import PDF → extract Markdown → Lumbrera source write → FTS search → chat answer with citations.

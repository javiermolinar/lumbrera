# Always-Running Lumbrera Slack Agent Plan

## Context
New product idea: run Lumbrera behind an always-on agent daemon that can be invoked from Slack. Slack becomes a lightweight team interface for asking questions, ingesting useful conversations/files, and approving durable brain updates. Lumbrera remains hidden as the source-grounded Markdown/provenance/write layer.

## Goals
- Provide a Slack-accessible agent for a shared Lumbrera brain.
- Let users ask grounded questions from Slack.
- Let users search the brain from Slack.
- Let users ingest Slack threads, links, files, and PDFs into the brain.
- Let the agent propose wiki updates from team conversations.
- Preserve trust by requiring approval before durable writes in the initial version.
- Keep all durable changes flowing through Lumbrera and Git-backed history.

## Constraints
- Slack is an interface, not the source of truth.
- Users should not need to know or use Lumbrera commands.
- Initial version should avoid fully autonomous writes.
- Durable mutations should go through `lumbrera write`, followed by verification and Git commit/push.
- Agent should serialize writes per brain to avoid Git and Lumbrera conflicts.
- Slack permissions, channel privacy, and data retention boundaries need explicit design.

## Proposed plan
1. Define the agent architecture:
   - Slack app/bot receives slash commands, mentions, file events, link events, and reactions.
   - Agent daemon routes requests to Lumbrera search/write/verify and an LLM provider.
   - Brain storage remains Markdown + Lumbrera metadata + Git.
2. Add Slack command surface:
   - `/brain ask <question>` for grounded Q&A.
   - `/brain search <query>` for deterministic FTS results.
   - `/brain ingest <url|file|thread>` for source capture.
   - `/brain summarize <thread|channel>` for summaries.
   - `/brain save <draft> to <topic>` for approved wiki updates.
   - `/brain health` for maintenance candidates.
3. Add reaction-based capture:
   - `:brain:` on a thread proposes ingestion.
   - `:save-to-brain:` proposes a durable wiki update.
   - Agent replies with a preview and approval buttons.
4. Implement safe write workflow:
   - Draft update from source material.
   - Post preview in Slack with citations and target wiki/source paths.
   - Require human approval.
   - Pull latest Git state.
   - Run `lumbrera write`.
   - Run `lumbrera verify`.
   - Commit and push.
   - Post success/failure back to Slack.
5. Implement grounded answer workflow:
   - Receive Slack question.
   - Run Lumbrera FTS search.
   - Provide recommended sections/sources to the LLM.
   - Return concise answer with citations and links to source/wiki material.
6. Implement ingestion workflow:
   - Accept Slack files, thread permalinks, external URLs, and pasted text.
   - Convert content to Markdown source files.
   - Preserve provenance back to Slack URLs, file metadata, uploader, and timestamp.
   - Index/update the brain.
7. Add scheduled agent jobs:
   - Weekly digest of new sources and wiki changes.
   - Health/consolidation suggestions.
   - Stale or orphaned knowledge candidates.
   - Unanswered questions or recurring topics from Slack.
8. Add operational controls:
   - Per-brain write lock.
   - Audit log of Slack user, action, source, commit SHA, and Lumbrera operation.
   - Rate limits and approval requirements by channel/team.
   - Admin controls for allowed channels and ingestion policies.

## Open questions
- Should the Slack app support one shared team brain or multiple selectable brains?
- Which commands should be available in public channels versus private channels or DMs?
- Should ingestion of Slack threads require consent from channel members or workspace admins?
- What LLM provider should power drafting and Q&A?
- How should original Slack files be stored: Git LFS, object storage, or external links only?
- Should approval happen through Slack buttons, slash commands, or both?
- Should the daemon also power the future iOS app API, or should Slack and iOS have separate adapters?

## Next steps
- Define the minimal Slack command set for v1: likely `/brain ask`, `/brain search`, and `/brain ingest`.
- Sketch the approval message format for proposed writes.
- Design the daemon API around ask, search, ingest, draft, approve, and commit.
- Prototype one end-to-end flow: Slack thread reaction → Markdown source draft → approval → Lumbrera write → verify → Git commit → Slack confirmation.

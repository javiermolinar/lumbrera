# Native Lumbrera MCP: first version

Lumbrera provides `serve --brain <directory>` using MCP Go v1.1.1 and Streamable HTTP at `/mcp`. No Git repository, brain ID, repository URL, commit ID, model or agent is required.

- `brain_search` returns the existing CLI search JSON directly, using shared Go search/index code and the same response projection. Existing paths, anchors, source references, filters and recommended sections are preserved.
- `brain_read` accepts a relative path and optional anchor, byte offset and page size. It returns current Markdown content with pagination metadata. Managed frontmatter is stripped; source content is preserved. Anchors use the existing Markdown parser.
- `brain_status` reports readiness, current index state and read limits. `/livez` is process liveness; `/readyz` reports index readiness.

Inputs and outputs are described by `internal/mcpserver/contracts/mcp-v1.json` and validated by MCP Go. Successful results carry structured content plus equivalent JSON text. Semantic failures use safe error messages. Reads and searches are bounded; no shell or raw command arguments are exposed.

The service operates on the configured directory and its disposable `.brain` index. Search uses the existing automatic index-refresh behavior. Requests are serialized while inspecting or preparing the index; existing Lumbrera mutation locks protect individual operations. Status observes index freshness without rebuilding. Startup prepares the index once; failed preparation leaves liveness up and later searches can retry. There are no snapshots, activation signals, remote fetches or Git citations.

Path traversal, symlink access, internal files and binary assets are rejected. Only content Markdown and explicitly allowed catalogs can be read. Brain instructions and skill scaffolds are inert data, never runtime configuration. The configured directory is trusted local storage; arbitrary external edits bypassing Lumbrera's locks are not supported during an operation.

Content may change between search and read, or between read pages. This version provides no revision consistency, historical reads or last-known-good snapshot. Git synchronization and write-publication coordination are deferred to application work. It remains a read-only MCP interface; only the disposable index is maintained.

Default bind is loopback. Remote binding requires private authenticated ingress supplied by deployment. Browser origins are rejected and MCP Go localhost protection is retained. No OAuth/user identity, Hermes/Slack or deployment integration is implemented here.

Limits: 64 KiB request JSON, 2 MiB encoded tool result, 30-second tool timeout; read pages default to 32768 bytes and allow at most 65536 bytes, without splitting UTF-8 characters. Document reads are capped at 64 MiB.

[Library: MCP Go v1.1.1](https://github.com/mark3labs/mcp-go/tree/v1.1.1). This contract supersedes the earlier Git-snapshot proposal.

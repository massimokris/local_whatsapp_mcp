# LocalWhatsAppMCP service

The Go service owns app configuration, shared scope, independent agent grants, authenticated Streamable HTTP, private Unix control, and the bundled wacli supervisor. It never writes upstream message/session databases. The only direct upstream SQL is a pinned, read-only minimal catalog query.

Build and test from this directory:

```sh
go build -o localwhatsappmcp ./cmd/localwhatsappmcp
go test -tags sqlite_fts5 -race ./...
WACLI_TEST_BINARY=/absolute/path/to/built/wacli go test -tags sqlite_fts5 ./internal/engine
```

The optional engine test creates an isolated synthetic archive; it does not connect to WhatsApp. Service tests use deterministic engine fixtures at the JSON/events boundary. They exercise private pairing/replay/revocation, real MCP request handling, sending permission and idempotency, restart after a failed scope transition, hidden changes, cached attachment denial, and multi-chunk file reads. Live WhatsApp linking, provider login, and actual client reload remain installation acceptance checks.

`serve --data-dir DIR --wacli PATH --helper PATH` binds loopback port 32191 and `DIR/control.sock`. The service takes a separate exclusive owner lock; after a crash it waits for the upstream writer lock before publishing policy. The owned auth/sync subprocess receives a lifetime pipe, so backend death requests graceful writer shutdown. Scope changes cancel reads, stop and await the writer, atomically publish the new policy, and require the exact `policy_loaded` revision before reopening access.

App settings contain credential digests, single-use invitation digests, scope, and durable send identifiers. Bearer credentials and cursor encryption keys use the macOS Keychain Security framework directly; no credential appears in process arguments. Public routes contain only MCP, authenticated attachments, and the cloud extension. Setup and administration remain private Unix routes.

Each send requires a caller-generated `request_id`. The service saves an uncertain record before dispatch and never dispatches that identifier again. An uncertain result requires conversation inspection before choosing to send again; a transport error is not proof of failure. Accepted sends are distinct from observed delivery/read state.

Client setup supports `codex`, `claude` (Claude Code), and a generic `stdio` adapter. Codex and Claude configuration preserve other server entries and reference the credential helper by non-secret connection ID. `headers` is an MCP-host interface and prints credential headers directly to that host; never run it as a conversation-visible diagnostic. `doctor` performs redacted authenticated discovery and a status call. Interrupted configuration installation can resume with `setup --resume CONNECTION_ID`.

Client configuration fields were checked against the official [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference) and [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp) on 2026-10-23. This establishes configuration contracts, not universal compatibility with installed client versions.

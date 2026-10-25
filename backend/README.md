# LocalWhatsAppMCP service

The Go service owns app configuration, shared scope, independent agent grants, authenticated Streamable HTTP, private Unix control, and the bundled wacli supervisor. It never writes upstream message/session databases. The only direct upstream SQL is a pinned, read-only minimal catalog query.

Catalog rows and MCP `find_conversations` include `archived: true/false`, read from the existing `chats.archived` column maintained by wacli's WhatsApp sync. MCP still filters every result by shared chat scope. Archive status is independent of scope and can lag the phone while syncing or offline; the service adds no archive/unarchive action.

Build and test from this directory:

```sh
go build -o localwhatsappmcp ./cmd/localwhatsappmcp
go test -tags sqlite_fts5 -race ./...
WACLI_TEST_BINARY=/absolute/path/to/built/wacli go test -tags sqlite_fts5 ./internal/engine
```

The optional engine test creates an isolated synthetic archive; it does not connect to WhatsApp. Service tests use deterministic engine fixtures at the JSON/events boundary. Pairing verification covers HTTP exchange/replay/revocation alongside real MCP request handling, sending permission and idempotency, restart after a failed scope transition, hidden changes, cached attachment denial, and multi-chunk file reads. Executed checks are recorded in [acceptance evidence](../docs/acceptance.md); live WhatsApp linking and actual client reload remain installation checks.

`serve --data-dir DIR --wacli PATH` binds loopback port 32191 and `DIR/control.sock`. The service takes a separate exclusive owner lock; after a crash it waits for the upstream writer lock before publishing policy. The owned auth/sync subprocess receives a lifetime pipe, so backend death requests graceful writer shutdown. Scope changes cancel reads, stop and await the writer, atomically publish the new policy, and require the exact `policy_loaded` revision before reopening access.

App settings contain credential digests, single-use invitation digests, scope, and durable send identifiers. Bearer credentials and cursor encryption keys live in `credentials.db` inside the application data directory, protected by directory mode 0700 and file mode 0600. The service has no Keychain dependency. The loopback HTTP listener serves authenticated MCP, authenticated attachments, and the one-use pairing exchange. Invitation creation and all administration remain private Unix routes. No tunnel, public hostname, OAuth endpoint, or provider login is supported; legacy cloud configuration is ignored and hosted-agent grants are not restored.

Each send requires a caller-generated `request_id`. The service saves an uncertain record before dispatch and never dispatches that identifier again. An uncertain result requires conversation inspection before choosing to send again; a transport error is not proof of failure. Accepted sends are distinct from observed delivery/read state.

The app creates a named invitation through private `POST /pairing`. The generic instructions direct an agent to POST `{"token":"<temporary pairing code>"}` as JSON to `http://127.0.0.1:32191/pairing/redeem` (using the running service's actual port). A successful exchange returns `{"id":"...","url":"http://127.0.0.1:32191/mcp","access_token":"...","token_type":"Bearer"}`. The code expires after ten minutes and is consumed once; the connection starts with sending disabled. The agent saves the bearer token, configures its own Streamable HTTP client, and verifies `check_status`. If the successful response is lost, create a new invitation instead of replaying the consumed code.

`localwhatsappmcp` retains only the native app's service/control responsibilities. The client-specific `setup`, `headers`, `doctor`, and `stdio` commands and configuration installers are removed. Existing credential grants are preserved; any client configuration referring to removed commands needs agent-managed reconfiguration. Manual URL/token setup remains available. See [component interfaces](../docs/implementation/interfaces.md#local-mcp-boundary) for the exchange contract and [pairing decision](../docs/wayfinder/comments/003-agent-pairing.md#generic-http-pairing-revision--2026-10-24) for supersession of the older helper design.

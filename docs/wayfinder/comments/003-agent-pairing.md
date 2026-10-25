# Research update: Preserve one-time pairing across MCP client connections

- Date: 2026-10-23
- Ticket: [Preserve one-time pairing across MCP client connections](../issues/003-agent-pairing.md)
- Research worker: `/root/mcp_pairing_facts`
- Result: partial technical answer; ticket remains open and available for follow-up
- Evidence: [Agent-pairing integration](../../research/agent-pairing.md)

The documented local-client mechanisms support a bundled helper that redeems the single-use invitation, creates a read-only agent grant, saves credentials privately, and installs a helper-backed HTTP connection or stdio adapter. App-controlled sending permission remains independent of transport credential configuration. This preserves immediate reading access and avoids an additional app approval.

For hosted clients requiring OAuth, keep standard authorization behind their ordinary browser Connect surface and bind it to the specific pre-paired agent grant. The Go MCP SDK supplies resource-server primitives, not a complete authorization server. The research proposes a private browser handoff and identifies client/protocol validation requirements.

The architecture is plausible, but two experience-critical mechanisms remain unverified:

1. A secure browser handoff that binds the cloud OAuth request to the correct pre-paired grant without adding another app approval or exposing a durable secret.
2. First-time ngrok credential import after provider login, with no long-lived token passing through agent conversation. Reusing existing credentials is documented; automatic import after a new browser login is not established by the reviewed sources.

Keep this ticket open until those mechanisms have a concrete documented route or focused implementation evidence. These are technical follow-up items for the implementer, not new product questions for the user. Do not claim that a hosted client works merely because the tunnel is reachable, or replace the agreed experience with manual OAuth/header setup.

Release the research claim after this update. No account access, provider setup, client configuration, or end-to-end test was performed. If evidence later proves the fixed experience impossible for a target, report that specific consequence rather than silently changing the specification.

## Implementation resolution — 2026-10-23

The end-to-end implementation request claimed this technical follow-up for `massi`. Both remaining mechanisms now have concrete implementation routes and focused verification, so the research question is answered.

The cloud helper opens a one-use loopback browser handoff that posts a short-lived secret to the public HTTPS bootstrap endpoint. The resulting browser session is bound to a specific pre-paired grant, expected client, and exact redirect URI. The ordinary hosted-client OAuth request adds its actual S256 challenge and resource audience; a CSRF-protected Connect submission consumes the session. Deterministic tests exercise incorrect clients, one-use bootstrap, expiration, PKCE exchange/replay, restart, rotating tokens, and grant revocation. No additional approval appears in the native app. See [cloud setup and protocol](../../cloud-setup.md) and the historical `backend/internal/cloud/oauth_test.go` fixtures.

For first-time ngrok setup, the helper explicitly opens the provider dashboard and a native hidden-input credential dialog. The user moves the provider authtoken directly into that dialog; the helper transfers it through the private Unix control route to Keychain. Existing ngrok configuration is imported privately when available. This is a provider credential step, not manual MCP headers or OAuth-server setup, and never passes the credential through the agent conversation. Cloudflare uses its normal browser login and named-tunnel credentials-file import. The historical provider helper (`backend/cmd/localwhatsappmcp/cloud.go`) implemented both routes; background recovery never opens login windows.

Local Codex/Claude Code configuration and the generic stdio bridge use independent read-only-by-default grants and the private helper/Keychain path. Actual client reload, real provider authentication/DNS, and hosted browser compatibility remain [live installation checks](../../acceptance.md#live-installation-checks). No real account was connected while resolving this ticket. Closure records an implementation route and synthetic protocol evidence, not universal client compatibility.

## Local-only revision — 2026-10-23

The [updated product agreement](001-product-contract.md#local-only-revision--2026-10-23) removes cloud access for now. Its local helper, Keychain, and stdio/loopback HTTP route remains the supported pairing design. The cloud browser handoff, OAuth server, provider credential setup, and public endpoint described above are superseded historical implementation evidence. They are not supported setup paths in the current build.

The pairing invitation still expires after a short interval and can be redeemed once. The helper stores the resulting durable credential automatically, so each local agent can be revoked and have its own sending setting without asking the user to copy authentication headers or manage a long-lived token.


## SQLite credential revision — 2026-10-23

The user explicitly replaced Keychain with local SQLite storage to avoid macOS credential-access prompts. The current service and setup helper store app-owned keys and bearer credentials in the private `credentials.db` database. Directory/file permissions protect that file; the app does not add database encryption or request Keychain access. WhatsApp linking and archive storage remain unchanged.

Migration creates a fresh internal key, which expires old read checkpoints and temporary attachment links. Existing configured local agent identities retain their grants and permissions while receiving fresh credentials through the same helper references. A completed migration requires its existing credential database on restart; losing it is a recovery error, not permission to regenerate credentials. The [specification](../../../SPEC.md) and [component interfaces](../../implementation/interfaces.md) define the current contract; [acceptance evidence](../../acceptance.md) records executed checks. Earlier Keychain notes above remain historical.

## Local MCP UI restoration — 2026-10-23

The user requested adding MCP connection to the UI after the local-only revision. This supersedes the temporary deferral of agent setup and management in the WhatsApp design revision. The native overview now includes a local MCP entry, with a dedicated screen for connection information, one-time setup instructions, saved agent connections, sending permission, and revocation. Codex, Claude Code, and generic local stdio adapters remain the supported implementation targets.

This reuses the existing private-helper pairing design and SQLite credential storage; it does not add public HTTP exposure, tunnels, hosted-agent setup, or OAuth. The loopback endpoint remains local to the Mac. A saved agent grant and a running MCP service must not be presented as evidence of an active client. The [specification](../../../SPEC.md#connect-agents) and [UI reference](../../ui-reference.md#local-mcp-connections) define the restored screens. Verification belongs in [acceptance evidence](../../acceptance.md); the earlier package and live-check limitations remain in effect until separately checked.

## Generic HTTP pairing revision — 2026-10-24

The user explicitly chose generic setup instructions, a temporary one-use pairing code, and an ordinary HTTP exchange, with each agent responsible for its own client configuration. This supersedes the client-specific setup, credential-header, diagnostic, and stdio helper design above. The service/control executable remains bundled because the native app uses it to run and manage the backend. The user deferred UI changes; existing screens and non-manual client choices remain, but those choices produce the same generic invitation and do not select an installer. Manual URL/token setup is unchanged.

The app creates a named invitation through private control. Its instructions identify the local `POST /pairing/redeem` endpoint and JSON body `{"token":"<temporary pairing code>"}`. A code is random, expires after ten minutes, and creates at most one connection. Successful exchange returns the connection ID, full MCP URL, persistent access token, and bearer token type. The agent saves the token, configures its MCP client for future sessions, and verifies `check_status`. Reading is immediately authorized within shared chat scope; sending starts disabled and revocation remains independent per connection. Invitation creation and grant administration remain private. The [component interface](../../implementation/interfaces.md#local-mcp-boundary) defines the wire contract.

This design gives the persistent credential to the redeeming agent instead of relying on a private helper installation. Generated instructions contain only the temporary code and ask the agent to avoid printing the access token. The app cannot guarantee that an agent omits HTTP responses from its internal tool history. The service retains local SQLite credential storage, redacted state/logs, and loopback-only access; there is no public endpoint or OAuth flow.

Existing bearer grants remain valid, but clients that call the removed `setup`, `headers`, `doctor`, or `stdio` commands require agent-managed reconfiguration. The app must not silently rewrite user client configuration files. A consumed code stays consumed if its HTTP response is lost; the recovery path is a new invitation and removal of any unused connection, rather than replay or helper resume.

This comment records the user's accepted change, not a claim of executed tests or universal client compatibility. The [specification](../../../SPEC.md#connect-agents) defines acceptance, and [acceptance evidence](../../acceptance.md) records verification and remaining live checks. Earlier research and resolutions are preserved as superseded history.

## MCP connection UI simplification — 2026-10-24

The user subsequently requested the UI redesign deferred in the generic HTTP pairing revision. With no connections, **Connect your agent** opens directly with a **Copy instructions** action that creates and copies the temporary invitation in one step. Remove the name form and client selector; assign names such as **Agent 1** automatically. Inline **Do it manually** expands a shared credentials card with copy icons beside the MCP URL and abbreviated access token. Copying the token uses its complete value. Manual creation keeps setup open so both values can be copied.

With saved connections, **Local MCP** shows expandable home-style cards containing each name, last-request/sending subtitle, and chevron. Expanded controls are **Delete connection** with a confirmed **Delete** action, **Allow message sending**, and **Credentials**. **Add new connection** opens the same setup screen; finishing/back returns to the list, and successful pairing returns automatically using the exact invitation identity. Saved credentials use the same display/copy card as manual setup.

To support that disclosure, private `GET /agents/ID/credentials` now returns the existing stored credential for any active connection, including generic paired grants. It must not rotate a token, change permission, create a connection, expose another connection's token, or become a public HTTP/MCP route. Logs and ordinary state remain redacted. The generic exchange protocol, ten-minute one-use code, per-connection revocation, initially disabled sending, and local-only boundary are unchanged.

The [specification](../../../SPEC.md#connect-agents), [UI reference](../../ui-reference.md#local-mcp-connections), and [component interfaces](../../implementation/interfaces.md#serviceprivate-control-boundary) define this revision. This records the user's design decision; executed UI, backend, and packaging checks belong in [acceptance evidence](../../acceptance.md). Earlier resolutions remain as history.

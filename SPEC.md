# LocalWhatsAppMCP — v1 specification

Status: accepted product direction; implementation has not started in this folder. Recorded 2026-10-23 from the originating conversation. The [Wayfinder map](docs/wayfinder/map.md) records decisions and technical evidence; this document defines the product acceptance contract.

## Product

LocalWhatsAppMCP is a small native macOS menu bar app that connects a user's WhatsApp to their preferred agents through MCP. The first users already run local agents capable of following setup instructions and configuring software. Multiple agents can connect simultaneously to the same WhatsApp connection and local archive.

The app owns WhatsApp connectivity, local storage, chat selection, MCP access, and optional tunnel configuration. The connected agent owns reasoning, summaries, task extraction, scheduling, and notifications. The app does not require an AI model or model-provider API key.

Examples the product must enable through ordinary tools:

- “Every 30 minutes, scan my WhatsApp and tell me about tasks or requests for me,” when the chosen agent supports scheduled work and notifications.
- “Give me a brief of the last two hours of group X.”
- “Extract tasks from chat Y from the last two days.”
- “Read the document attached to this message.”
- “Send this reply and attachment,” when sending is enabled for that agent and the destination is selected.

Public name: **LocalWhatsAppMCP**. Project folder: **local_whatsapp_mcp**. The initial implementation uses one WhatsApp account per app installation as a simple engineering default; simultaneous connections refer to agents sharing that account.

## Onboarding and everyday use

### Connect WhatsApp

1. Open the app and choose **Connect WhatsApp**.
2. Display the QR code supplied by wacli and the progress/status of linking the device.
3. As soon as the connection delivers history or live messages, save the received content. Initial capture is enabled before the user has selected chats.
4. Show the chat-scope choice once the connection is established and the chat catalog can be presented. History import may continue while the user chooses.

The first import captures everything delivered during setup. It must not depend on selecting chats before WhatsApp starts sending history. If setup is interrupted, retain the connection and archive and resume setup on the next launch; agent access remains unavailable until scope is confirmed.

WhatsApp controls the history it provides. Use “Import available history” and describe observed coverage and import progress. An idle sync, recent message timestamp, or completed page traversal does not prove that all WhatsApp history is present.

### Choose chat scope

Present a clear choice with:

- Primary action: **Continue with all chats**.
- Secondary action below it: **Select specific chats**.

**All chats** is a persistent mode. Existing and future direct chats and groups are included automatically.

**Selected chats** is an explicit list. New conversations are excluded until the user adds them. The chooser should feel like a WhatsApp chat list:

- Show direct chats and groups together, with recognizable names, sorted by most recent known activity.
- Fall back to a phone number or stable identity when a name is unavailable.
- Search by group/chat name, associated contact name, and phone number.
- Place a selection control on the right of each row.
- When search is active, **Select all** selects all results matching that search, including results outside the visible viewport. It preserves selections outside that result set.
- Keep selected state while changing or clearing the search.
- Allow the user to change scope later from the app, including switching between All chats and Selected chats.

The local chooser may retain minimal catalog metadata for unselected chats so they can be added later. Message previews, message bodies, and attachments are conversation content and follow the storage/access rules below.

### Connect agents

After chat scope is confirmed, offer **Connect an agent**. Preserve this experience:

1. Generate copyable setup instructions containing the MCP connection information and a short-lived, single-use pairing token.
2. The user gives those instructions to their local agent.
3. The agent uses the bundled setup helper to exchange the pairing token and install its own credential directly into the chosen client's configuration.
4. Successful redemption grants reading access immediately. There is no additional approval prompt in the app.
5. Verify the connection and show it as a named entry in the app.

The user does not configure authentication headers, an OAuth server, or an identity provider. Long-lived credentials must not be printed into the agent conversation or copied manually by the user. Implement authentication behind this experience; protocol choices do not justify redesigning onboarding.

Each connection has its own revocable credential and its own **Enable message sending** setting, initially off. Disconnecting one agent revokes that connection without disrupting WhatsApp sync or other agents. Chat scope is shared across all agent connections.

### Optional cloud access

Local access is the default. **Connect cloud agent** enables an optional tunnel to the same local MCP server. Support **ngrok** by default and **cloudflared / Cloudflare Tunnel** as the alternative.

The generated instructions let the user's local agent:

1. Install the selected tunnel executable if it is missing.
2. Reuse existing provider authentication or guide the provider's browser login.
3. Create the tunnel and configure a stable public hostname using the provider/account's available capabilities.
4. Save provider choice, hostname, and credential references directly into the app's local configuration; store secrets through the app's protected credential interface.
5. Verify the public MCP endpoint and configure the chosen MCP client.

A cloud client's required browser connection/authorization step is acceptable. The implementation handles its authentication protocol; the user still performs no manual OAuth/header configuration and creates no separate LocalWhatsAppMCP login account. Automate client installation where the client supports it; use its normal browser connection surface where required.

The app retains the setup and owns subsequent tunnel startup, shutdown, and recovery. Background recovery should report a need for provider login rather than opening unexpected login windows. Tunnel setup does not move the archive or WhatsApp session to a cloud service.

Product names such as Codex, Claude, ChatGPT, Muse, Meta, and Grok express the intended agent ecosystem, not a verified support matrix. Record actual transport, authentication, file-handling, and installation support before claiming compatibility with a specific client/version. A tunnel alone does not establish client compatibility.

## Storage and access contract

### Initial capture and confirmed scope

Before scope is confirmed, save all received message content and attachment metadata. Keep MCP content access unavailable during this setup state.

On scope confirmation, commit the configuration used by both ingestion and MCP authorization. From that point onward:

| Operation | Selected conversation | Unselected conversation |
| --- | --- | --- |
| Store new messages or newly delivered historical content | Store | Discard before message-archive persistence |
| Retrieve existing content through MCP | Allow | Reject |
| Include content or conversation results in MCP search/listing | Include when matched | Exclude |
| Retrieve a cached or remote attachment | Allow | Reject |
| Send text or an attachment | Allow only if this agent has sending enabled | Reject |

Previously captured content from unselected chats may remain in the local archive. The configuration is a deterministic access boundary, independent of what happens to exist on disk. Filtering must be enforced in server code across every tool and file/resource path, not delegated to the model's instructions.

The scope governs chat content; minimal local catalog metadata and the linked-device protocol state have separate purposes. The selection must remain useful for later additions without retaining unselected message previews. The filter does not claim that WhatsApp stops delivering unselected events to the linked device.

Removing a chat immediately prevents further ingestion and future MCP retrieval through every connection, including attachment cache paths. This does not automatically delete its initial/previous archive. Re-selecting it can make retained content accessible again. Messages missed while it was excluded are available only if later supplied by WhatsApp; do not claim gap-free backfill.

Authorization is evaluated against the current configuration on each request. Sending permission and chat scope are checked again before dispatching a send. Content already returned to an external agent cannot be recalled by changing the selection.

Use stable WhatsApp identities for scope, with verified identity aliases handled consistently. Names and phone-number display changes must not accidentally grant access to another conversation. Preserve policy across restarts and reconnections; a failure to load confirmed policy must not silently restore unrestricted access/capture.

### Attachments

Store the message's attachment metadata during sync, then download the file when requested. Reuse a valid local cached copy on later requests. Attachment retrieval is ordinary authenticated server functionality and carries the same chat permission as its parent message.

Use wacli's existing media-download and recovery behavior. A recorded reference is not a guarantee of future availability: expired server media may require an online phone that still holds the file. Distinguish unavailable media, a phone that did not respond, and a retryable download error. Never represent failure as an empty successful attachment.

Return content in a form the MCP client can consume. A local filesystem path alone is not a cloud download mechanism. Any attachment endpoint/resource applies agent authentication and current chat scope.

### Local persistence and credentials

The intended application support directory is `~/Library/Application Support/LocalWhatsAppMCP/`. Keep the wacli session database, message archive, downloaded media, and app configuration organized there, with appropriate local file permissions.

Reuse wacli's `session.db` for linked-device state and `wacli.db` for messages/catalog/search. Keep app configuration, shared chat scope, and agent grants separate from upstream-owned records. Use macOS Keychain for app-managed secrets; WhatsApp session keys remain in wacli's protected session store. Store credential verification material rather than recoverable bearer tokens where possible.

Runtime databases, pairing tokens, credentials, and downloaded private media do not belong in source control, generated instructions after redemption, or diagnostic logs. Setup saves secrets directly through the helper; normal configuration may contain credential references.

## MCP surface and permissions

Use the official Go MCP SDK with Streamable HTTP on a loopback address. Local agents authenticate to this service. Optional public HTTPS access reaches it through the configured tunnel. Private app-control operations must not become public MCP tools through that tunnel.

These are the required capabilities; final tool schemas/names are implementation details:

| Capability | Required behavior |
| --- | --- |
| Find conversations | List/search permitted conversations and the metadata needed to identify them. |
| Read messages | Retrieve messages by conversation and time range, with bounded pagination and stable message references. |
| Search messages | Search permitted message content, including useful chat/sender/time/media filters. |
| Read context | Retrieve surrounding messages for a specific permitted message. |
| Get updates | Return scoped incremental changes since a checkpoint so agents can process new information. |
| Get attachments | Retrieve an authorized attachment on demand and report cache/download/unavailable outcomes. |
| Send text | Send a new text message to a selected conversation when enabled for the calling agent. |
| Send attachments | Send a new attachment, with caption where supported, under the same permission. |
| Check status | Report relevant connection/sync state, archive freshness, and available-history limitations without leaking secrets. |

Expose specific tools; a generic unrestricted CLI executor would defeat scope enforcement. Reading access is the default capability for each connection. Every invocation enforces authorization even if the tool is hidden in that client's tool list.

Sending permission covers new text and attachments only. It can operate in a selected direct chat or selected group. Editing, deleting/revoking, group creation/administration, membership changes, and other unrelated WhatsApp mutations are excluded from v1.

Incremental results need safe retry/checkpoint handling and explicit checkpoint-expiry behavior. A scoped agent must not receive unselected message references or content through a global change feed. Results describe the retained archive; stale or partial data must be identifiable by the agent.

Distinguish an accepted send from observed delivery/read state. Avoid duplicating a message after an uncertain response; use request identifiers or reconciliation appropriate to wacli. Per-message confirmation policy inside the chosen agent is the agent's responsibility; the app's explicit sending toggle is its authorization control.

## Runtime behavior

- Launch at login using native macOS registration, honoring any OS-required enablement.
- Keep the app in the menu bar. Closing the panel leaves WhatsApp sync, MCP access, and any enabled tunnel running.
- **Quit** explicitly stops the app-owned backend, sync, and tunnel processes.
- Respect normal Mac sleep. Do not keep the computer awake automatically. Reconnect on wake and report current state.
- Cloud clients cannot reach the archive while the Mac/app is unavailable. Explain this when enabling cloud access.
- Restore saved chat scope, agent grants, and tunnel configuration on startup.
- Make disconnected, importing/syncing, ready, and attention-required states legible. Do not describe a process heartbeat as proof of complete history or continuous WhatsApp connectivity.
- One wacli sync owner manages the WhatsApp session/store; the Go service coordinates commands through upstream-supported interfaces.

Sync, sending, file serving, and tunnel recovery are implementation responsibilities using the chosen stack. They are not open product questions.

## Stack and ownership

| Layer | Accepted choice and responsibility |
| --- | --- |
| Native app | SwiftUI views and AppKit menu bar/panel behavior. Use [Dock's UI reference](docs/ui-reference.md). |
| Login lifecycle | ServiceManagement / `SMAppService`. |
| Backend | Bundled Go service: app configuration, agent authentication/grants, scope enforcement, MCP, and process/tunnel coordination. |
| WhatsApp engine | Pinned bundled wacli executable, patched narrowly for content ingestion scope. wacli owns the WhatsApp connection and its store writes. |
| Integration boundary | wacli JSON command results and NDJSON auth/sync lifecycle events; respect the running sync process and store-lock/delegation behavior. |
| MCP | Official MCP Go SDK; Streamable HTTP locally, HTTPS through the optional tunnel. |
| Data/search | SQLite and FTS5 through the existing wacli archive, plus separately owned app settings/grants. |
| Secrets | macOS Keychain for app-managed credentials, with separate upstream WhatsApp session storage. |
| Media | Existing wacli download/recovery and local cache. |
| Tunnels | ngrok executable by default; cloudflared as the supported alternative. Install/configure through the user's local agent only when requested. |
| Setup | Generated instructions and a bundled helper that exchanges pairing tokens, persists configuration, and installs the client connection. |
| Distribution | Swift Package Manager and Go build tools; native `.app` and `.dmg` packaging. |

Build versions, minimum macOS version, architecture support, and signing/notarization are to be established from the pinned dependencies and actual packaging checks. Dock's own deployment target and release commands are not evidence that this new app builds or supports the same systems.

The local Go experiment in the sibling `whatsapp-mcp/` folder is not this app's implementation or a replacement for the accepted wacli boundary. This folder currently contains planning documents only.

## UI reference and surfaces

`/Users/massi/development/dock` is the visual reference. Use its native panel style, SF Mono typography, compact aligned rows, fine separators, adaptive neutral palette, restrained status colors, and button feedback. The [UI reference guide](docs/ui-reference.md) records exact source-grounded tokens and their intended adaptation.

Provide surfaces for WhatsApp QR/setup status, initial chat-scope choice, searchable chat selection, named agent connections and sending toggles, optional cloud setup/status, and settings/status/quit. QR display and a useful chat list may need more room than Dock's fixed panel; preserve the visual language while fitting the task.

Keyboard access, meaningful control labels, readable errors, and selection state must remain usable in both appearances. Avoid exposing protocol details in ordinary product flows unless they help the user recover from a concrete problem.

## Acceptance scenarios

These are future implementation checks, not tests already executed.

| Scenario | Expected outcome |
| --- | --- |
| Messages arrive immediately after QR pairing | All received initial content is saved while setup is pending; agents cannot query it yet. |
| Initial capture includes Work and Family; user selects Work | Family's existing content remains inaccessible through every MCP path; subsequent Family content is discarded before storage. |
| Scope changes during a history batch | After the confirmed policy transition, newly processed unselected content is not persisted or returned. |
| User searches “xyz” and selects all | Every matching conversation is selected, including off-screen results; unrelated existing selections remain. |
| A new group appears | All chats mode includes it; Selected chats mode excludes it until added. |
| Two agents connect | Both share selected-chat access; each has a distinct credential and an independent sending toggle. |
| A pairing invitation is redeemed twice or after expiry | The second/expired redemption fails without granting a connection. |
| Pairing succeeds | Reading access works immediately; no additional app approval appears; durable credentials are installed outside conversation output. |
| Agent with sending disabled calls a send tool directly | The server rejects the operation. |
| Sending-enabled agent targets an unselected chat | The server rejects the operation. |
| An agent requests edit/delete/group administration | The capability is unavailable. |
| User removes a chat or revokes an agent | Subsequent content/file access is denied, including an already cached attachment. Other agents remain connected when only one is revoked. |
| Agent requests an attachment | The app retrieves/caches authorized media or returns an explicit recoverable/unavailable outcome. |
| Agent resumes incremental processing | Updates are scoped and replayable; expired checkpoints require an explicit recovery path. |
| App closes its panel, then Mac sleeps and wakes | Closing retains background operation; sleep is respected; wake reconnects using saved policy and credentials. |
| User chooses Quit | All app-owned processes stop. |
| User enables either supported tunnel provider | Agentic setup saves a stable configuration, verifies authenticated access, and the app restores it on later launches. |
| User restarts after an incomplete setup or configuration error | The app preserves setup/policy state and asks for the specific recovery action; it does not expose all archived content. |

## Boundaries

V1 does not include agent editing/deleting, group administration, an in-app agent scheduler or AI engine, a hosted archive, automatic prevention of Mac sleep, guaranteed complete WhatsApp history, or a promise that every agent product accepts the same authentication/installation method.

Routine implementation choices remain delegated to the implementer. Return to the user only when evidence requires changing a meaningful part of this experience. The [decision map](docs/wayfinder/map.md) tracks those decisions without turning the app build into a series of product interviews.

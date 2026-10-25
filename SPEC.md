# LocalWhatsAppMCP — v1 specification

Status: accepted product contract, revised to generic HTTP pairing, simplified connection screens, and direct three-step onboarding on 2026-10-24, following the local-only revision on 2026-10-23. Earlier implementation and packaging evidence may predate these revisions. See [installation and acceptance](docs/acceptance.md) for verification evidence and remaining live-account checks. The [Wayfinder map](docs/wayfinder/map.md) records decisions and technical evidence; this document defines the product acceptance contract.

## Product

LocalWhatsAppMCP is a small native macOS menu bar app that connects a user's WhatsApp to their preferred agents through MCP. The first users already run local agents capable of following setup instructions and configuring software. Multiple agents can connect simultaneously to the same WhatsApp connection and local archive.

The app owns WhatsApp connectivity, local storage, chat selection, local MCP access, and agent credentials. The connected agent owns reasoning, summaries, task extraction, scheduling, and notifications. The app does not require an AI model or model-provider API key.

Examples the product must enable through ordinary tools:

- “Every 30 minutes, scan my WhatsApp and tell me about tasks or requests for me,” when the chosen agent supports scheduled work and notifications.
- “Give me a brief of the last two hours of group X.”
- “Extract tasks from chat Y from the last two days.”
- “Read the document attached to this message.”
- “Send this reply and attachment,” when sending is enabled for that agent and the destination is selected.

Public name: **LocalWhatsAppMCP**. Project folder: **local_whatsapp_mcp**. The initial implementation uses one WhatsApp account per app installation as a simple engineering default; simultaneous connections refer to agents sharing that account.

## Onboarding and everyday use

Initial onboarding follows **Link WhatsApp → Choose chats → Connect your agent**. Linking opens the existing chat selector directly, and saving initial scope opens agent setup directly. The overview and a separate all-chats-versus-specific-chats choice are not intermediate onboarding steps. Users may exit onboarding and navigate normally at any point; background state refreshes must not redirect them back into it. On a fresh launch, resume linking or unconfirmed chat selection from persisted WhatsApp authentication and chat scope. Confirmed scope preserves ordinary overview, relinking, and chat-edit navigation, including when no agent connections remain.

### Connect WhatsApp

1. Open the app. Read the persisted WhatsApp session's authentication state before choosing a page. If WhatsApp is not linked, open **Connect WhatsApp** directly and prepare its QR code. For a linked account, open **Choose chats** if scope is unconfirmed, or the overview if scope is confirmed, including while sync is starting or the transport is temporarily reconnecting and when no agent connections remain. Reopening a linked account's window preserves current navigation and allows explicit Settings navigation. Reopening without a linked account prepares QR setup, including after sign-out or revocation. The saved engine session is the source of truth; saved sync intent or a separate first-run flag must not suppress required relinking.
2. Display the QR code supplied by wacli and the progress/status of linking the device.
   New WhatsApp links identify the device as **LocalWhatsAppMCP** on the phone's Linked devices screen.
3. As soon as the connection delivers history or live messages, save the received content. Initial capture is enabled before the user has selected chats.
4. As soon as initial linking succeeds, hide the phone instructions and QR setup controls and automatically open **Choose chats**, including its loading state while the catalog arrives. No **Continue** click or overview visit is required. History import may continue while the user chooses. Run the local MCP service automatically, but keep MCP content access blocked until scope is confirmed. Relinking with confirmed scope returns to the overview, with its WhatsApp card expanded. If the user exits or navigates away from onboarding, respect that choice instead of redirecting on a later status refresh.

The first import captures everything delivered during setup. It must not depend on selecting chats before WhatsApp starts sending history. If linking or chat selection is interrupted, retain the connection and archive and resume the unfinished step on the next launch; agent access remains unavailable until scope is confirmed.

WhatsApp controls the history it provides. Use “Import available history” and describe observed coverage and import progress. An idle sync, recent message timestamp, or completed page traversal does not prove that all WhatsApp history is present.

The WhatsApp card keeps its WhatsApp logo and state text without a redundant circular status control. Its collapsed header has a right chevron. Its expanded content provides **Disconnect** with confirmation. While WhatsApp is linked and scope is confirmed, add a **Choose connected chats** row directly below **Stop syncing WhatsApp**, using the same title/subtext/action layout. Show **X chats connected** from saved scope, deduplicating verified identities, and a compact **Choose** button that opens the selector. While the catalog is unavailable, show **Loading connected chats…** instead of an invented count; chooser drafts never change this count. Hide the chooser row during disconnect, after sign-out, and while awaiting QR linking; retain the saved scope for reconnection. Disconnect stops sync and signs this Mac out of the linked device, retaining the local archive, chat scope, and agent grants. Save disconnect intent before signing out so background restart and wake cannot silently reconnect. Failed or interrupted sign-out remains visibly retryable on the overview. Opening the app after sign-out finishes, or choosing **Connect**, prepares a new QR; linking requires a new phone scan. Disconnected agents can still read authorized archived content and verified cached attachments; sends and uncached attachment retrieval remain unavailable.

### Choose chat scope

Open the existing **Choose chats** selector directly during onboarding. Keep both scope actions inside that selector:

- Primary action: **Save selection**.
- Secondary action below it: **Continue with all chats**.

After either action successfully saves initial scope, open **Connect your agent** directly. Do not advance after a failed save. Later chat edits return to the overview, and users may leave the selector without completing onboarding.

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

Conversation results include the last locally synced WhatsApp **archived** status as a boolean. It mirrors WhatsApp's Archived folder and does not change chat scope, ongoing retention, or agent access. wacli owns updates to the existing SQLite field; the service reads it and exposes it only for permitted chats through MCP. The value may lag the phone while app-state synchronization is incomplete or the account is offline; a false value alone does not prove synchronization is complete.

### Connect agents

The app provides a local MCP server on this Mac automatically; starting it requires no provider setup. Initial scope confirmation opens **Connect your agent** directly. Outside onboarding, a local MCP card on the overview shows its current access state and opens connection setup or management. With no connections, open **Connect your agent** directly. Place the primary **Copy instructions** action and inline **Do it manually** disclosure outside and below its instructions card. Setup has no name field, client selector, separate generation step, or instruction editor. New connections use the first unused name in the sequence **Connection 1**, **Connection 2**, and so on. Preserve existing saved names.

Under **Connect your agent**, show **Give your agent access to WhatsApp chats**. The instructions card contains these three numbered steps:

1. Open your AI agent on this Mac.
2. Copy the instructions and paste them into your agent’s chat.
3. Send the message and follow any setup steps your agent provides.

After copying, show **Instructions copied. Paste them into your agent’s chat within 10 minutes.** Show **Done** only while **Do it manually** is expanded; opening setup to add a connection does not itself show Done. Add no bottom/footer explanatory text.

**Do it manually** reveals **Add this URL and token to your agent’s MCP settings.** followed by shared inline credential rows below the manual action, outside the instructions card and without a new credentials card. Each row places its **URL** or **Token** label, value, and copy icon on one line. Display the complete underlying token, using middle truncation only when the available width requires it; copying always puts the full value on the clipboard. This creates a manual connection when needed and keeps setup open so the user can copy both values; hiding and reopening those details within setup reuses that connection. The user enters the values directly into a local client's Streamable HTTP configuration using bearer authentication. The saved credential remains valid until the connection is deleted.

For agent-managed setup:

1. **Copy instructions** creates and copies generic instructions in one action, using an automatic connection name. The copied text contains the loopback HTTP exchange endpoint and a random, single-use pairing code that expires after ten minutes. It contains no persistent access token, helper executable path, data-directory argument, or client-specific installation command.
2. The user gives those instructions to their local agent.
3. The agent sends the code in a JSON HTTP POST body to the exchange endpoint. The response contains the full MCP URL and its connection's persistent bearer access token. The agent saves the credential, configures its own MCP client for future sessions, and verifies the connection with `check_status`. The app does not edit client configuration files or install a credential helper or stdio bridge.
4. Successful redemption grants reading access immediately. There is no additional approval prompt in the app.
5. Recognize redemption by the invitation's exact correlation identifier, clear temporary instructions, and return to the connection list. A saved grant is distinct from a recently active client.

When connections exist, the screen title is **Local MCP**. Show each connection as an expandable card matching the home cards, with an icon to the left of its name, a subtitle combining its last request time (or **No request yet**) and **Message sending on/off**, and a chevron. Every connection uses the `apple.terminal` SF Symbol, with `terminal` as a compatibility fallback when the preferred symbol is unavailable on older macOS. The icon is presentation only: do not randomly select or persist per-connection icon metadata, and ignore older saved icon fields. Match the overview's Local MCP icon treatment: 23 pt, muted secondary color, in a 32 × 32 pt frame. The expanded card contains **Delete connection** with a compact **Delete** button and confirmation, **Allow message sending** on the left with its switch aligned to the right, and **Credentials** on the left with a compact **Show**/**Hide** button on the right. Credentials appear as the same inline URL/token rows used in manual setup, within the connection card rather than a nested card surface. Their copy actions retain the full values. Showing credentials retrieves the existing stored credential for that connection through private control, for both manual and paired connections, without rotating the token or changing permissions.

Match the expanded WhatsApp card's action-row spacing: 14 pt row insets, a 12 pt horizontal gap between label content and control, and a 5 pt gap between each title and subtitle. **Delete**, **Show**, and **Hide** use the same button width as WhatsApp's **Disconnect** and **Choose**. Under **Delete connection**, show **Remove agent’s access to WhatsApp**; under **Allow message sending**, show **Let the agent send messages for you**. Under **Credentials**, show **No request yet** or **Last used at** followed by the recorded request timestamp in localized short date/time format. These row subtitles supplement the unchanged connection-header activity and sending status.

**Add new connection** opens the same setup screen. Finishing or going back returns to the existing list; successful pairing returns automatically. Manual creation may save a connection before leaving setup, so copying remains available. Clear transient in-app credentials on collapse, navigation, deletion, or Quit. Retain explicitly copied URL, token, and instruction text on the clipboard so Copy → Done → paste works; leaving setup does not invalidate an unredeemed invitation before its server-side expiry. Deleting a connection revokes it immediately without changing client configuration, WhatsApp sync, or other connections.

Invitation creation and connection management remain on private app control. The loopback HTTP exchange requires possession of a valid pairing code and atomically consumes it when creating one connection. Expired or consumed codes cannot grant access again. If the agent loses the successful exchange response, create a new invitation; do not make the old code reusable. The agent owns client configuration, credential storage, and any required client reload. These steps may depend on its client's supported transport and capabilities.

Show the loopback endpoint in connection credentials, with truthful service, scope, and error states. A running server is not proof that a client is connected or that WhatsApp history is complete. Both setup paths require confirmed chat scope. Generated instructions are temporary setup material; include their expiration and clear transient in-app instructions after leaving setup while retaining explicitly copied text on the clipboard.

The user's explicit requests for manual URL/token configuration and generic HTTP pairing supersede the earlier requirement that a bundled helper install credentials outside agent-visible output. Manual setup intentionally displays and copies the selected connection's access token. Generic pairing returns a durable credential to the redeeming agent and instructs it to store the value privately without printing it in its reply; the app cannot guarantee how the agent records HTTP responses. Neither path requires an OAuth server or identity provider.

Each connection has its own revocable credential and its own **Allow message sending** setting, initially off. Deleting one connection revokes its access without disrupting WhatsApp sync or other agents. Chat scope is shared across all agent connections.

### Local connection boundary

The current product connects agents running on the same Mac. The MCP service binds only to a loopback IP address (`127.0.0.1` by default); HTTP is its local transport. It is not published to the network, and the app provides no tunnel, public hostname, hosted-agent setup, OAuth server, or browser authorization flow.

Local connections still use independent credentials so the app can identify each agent, revoke it individually, and enforce its sending permission. Manual connections use the copied persistent access token. In generic pairing, the temporary code authorizes one HTTP exchange for a durable credential; the agent handles installation. Neither path requires repeated pairing during ordinary use. Possession of an access token alone does not authorize creating another connection or changing app settings.

Record actual transport, file-handling, and installation support before claiming compatibility with a specific client/version. Local access requires the app and Mac to be running.

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

Authorization is evaluated against the current configuration on each request. Sending permission and chat scope are checked again before dispatching a send. Content already returned to an agent cannot be recalled by changing the selection.

Use stable WhatsApp identities for scope, with verified identity aliases handled consistently. Names and phone-number display changes must not accidentally grant access to another conversation. Preserve policy across restarts and reconnections; a failure to load confirmed policy must not silently restore unrestricted access/capture.

### Attachments

Store the message's attachment metadata during sync, then download the file when requested. Reuse a valid local cached copy on later requests. Attachment retrieval is ordinary authenticated server functionality and carries the same chat permission as its parent message.

Use wacli's existing media-download and recovery behavior. A recorded reference is not a guarantee of future availability: expired server media may require an online phone that still holds the file. Distinguish unavailable media, a phone that did not respond, and a retryable download error. Never represent failure as an empty successful attachment.

Return attachment content in a form the local MCP client can consume. Any attachment endpoint/resource applies agent authentication and current chat scope.

### Local persistence and credentials

The intended application support directory is `~/Library/Application Support/LocalWhatsAppMCP/`. Keep the wacli session database, message archive, downloaded media, and app configuration organized there, with appropriate local file permissions.

Reuse wacli's `session.db` for linked-device state and `wacli.db` for messages/catalog/search. Keep app configuration, shared chat scope, and agent grants separate from upstream-owned records. Use a private `credentials.db` SQLite database for app-managed secrets, in the application data directory (directory mode 0700, database mode 0600). The app never reads, writes, or requests access to macOS Keychain. Secrets are protected by local file permissions, without application-level database encryption. WhatsApp session keys remain in wacli's protected session store. Store credential verification material rather than recoverable bearer tokens where possible.

Runtime databases, pairing codes, credentials, and downloaded private media do not belong in source control or diagnostic logs. Generated instructions contain only a temporary code; their transient in-app state is cleared after redemption or leaving setup, while explicitly copied instructions remain available for pasting. HTTP pairing returns the new connection's bearer token only upon successful exchange; the agent is responsible for keeping its configuration private. Manual creation and saved-connection credentials return only the selected connection's stored bearer token through private app control, for the user to copy into a local client's configuration. This applies to active manual and paired grants and never changes their credentials or permissions. General state responses never include access tokens.

Upgrading from Keychain creates a fresh local checkpoint key without reading Keychain, so old checkpoints and temporary attachment links expire. Local agents keep their identities, scope, and sending permissions while the service rotates their credentials. The generic-pairing revision itself preserves existing bearer grants, but removes `setup`, `headers`, `doctor`, and `stdio` helper commands: clients whose configuration calls those commands must be reconfigured by their agent using the new flow. Do not silently rewrite user client configuration. Preserve the WhatsApp session and archive. After credential migration completes, a missing or corrupt credential database requires recovery instead of silent regeneration.

## MCP surface and permissions

Use the official Go MCP SDK with Streamable HTTP bound only to an explicit loopback IP address (`127.0.0.1` by default). Local agents authenticate with a bearer token and handle their own client configuration. Reject non-loopback listen addresses and requests addressed to non-loopback hosts. Only the one-use pairing exchange is additionally available through HTTP; invitation creation, scope changes, and agent administration remain on the user-owned Unix socket and are not MCP tools or HTTP routes.

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
- Keep the app in the menu bar. Clicking another app, the desktop, or a confirmation dialog must not close or hide the app window. Use normal window layering and Space behavior: focusing another app lets it cover this window, and the window does not float above other apps or follow every Space. Native close hides the window while WhatsApp sync and local MCP access keep running; the menu bar reopens it. Confirmation actions, including Cancel and Delete, remain clickable while their dialog is open.
- **Quit** explicitly stops the app-owned backend and sync processes.
- Respect normal Mac sleep. Do not keep the computer awake automatically. Reconnect on wake unless the user explicitly disconnected WhatsApp, and report current state.
- Restore saved chat scope and local agent grants on startup. Previously saved cloud configuration must not start a tunnel or enable an external endpoint. Discard legacy hosted-agent grants while retaining local grants and their permissions.
- Make disconnected, importing/syncing, ready, and attention-required states legible. Do not describe a process heartbeat as proof of complete history or continuous WhatsApp connectivity.
- One wacli sync owner manages the WhatsApp session/store; the Go service coordinates commands through upstream-supported interfaces.

Sync, sending, file serving, and connection recovery are implementation responsibilities using the chosen stack. They are not open product questions.

## Stack and ownership

| Layer | Accepted choice and responsibility |
| --- | --- |
| Native app | SwiftUI views and AppKit menu bar/panel behavior. Use the [current UI reference](docs/ui-reference.md). |
| Login lifecycle | ServiceManagement / `SMAppService`. |
| Backend | Bundled Go service: app configuration, agent authentication/grants, scope enforcement, local MCP, and process coordination. |
| WhatsApp engine | Pinned bundled wacli executable, patched narrowly for content ingestion scope. wacli owns the WhatsApp connection and its store writes. |
| Integration boundary | wacli JSON command results and NDJSON auth/sync lifecycle events; respect the running sync process and store-lock/delegation behavior. |
| MCP | Official MCP Go SDK; authenticated Streamable HTTP on loopback only. |
| Data/search | SQLite and FTS5 through the existing wacli archive, plus separately owned app settings/grants. |
| Secrets | Private local SQLite database for app-managed credentials, with separate upstream WhatsApp session storage. |
| Media | Existing wacli download/recovery and local cache. |
| Setup | Manual URL/token configuration, or generic instructions with a temporary pairing code exchanged over loopback HTTP; the agent configures its own client. |
| Distribution | Swift Package Manager and Go build tools; native `.app` and `.dmg` packaging. |

Build versions, minimum macOS version, architecture support, and signing/notarization are recorded from pinned dependencies and actual packaging checks in [building.md](docs/building.md). Dock's own deployment target and release commands are not evidence that this new app builds or supports the same systems.

The local Go experiment in the sibling `whatsapp-mcp/` folder is not this app's implementation or a replacement for the accepted wacli boundary. This folder now contains the native app, Go service, pinned engine patch, build tooling, and planning history.

## UI reference and surfaces

The user's dark WhatsApp/network screenshots supplied on 2026-10-23 supersede the earlier Dock visual direction. Use a dark gradient, SF Pro Semibold headings, system sans serif body text, rounded cards, generous space, and white primary actions. The [UI reference guide](docs/ui-reference.md) defines the current adaptation.

The current UI includes WhatsApp connection/sync, QR linking, searchable chat selection, local MCP connection setup and management, and small settings with **Start at Log In** and **Quit**. Under **Start at Log In**, show **Open automatically when you log in to your Mac**; keep the switch aligned right and green when enabled. Initial onboarding routes from linking to the selector to agent setup directly; later scope edits return to the overview. Keep the MCP card only on the overview, using the same dark card treatment. Agent setup opens **Connect your agent** with **Copy instructions** and inline manual credentials; saved connections use expandable cards on **Local MCP**. This supersedes the earlier setup form, the temporary deferral of the pairing UI redesign, and the intervening overview/footer scope choice shown in earlier onboarding references.

Use a single 24 pt bottom inset below the archive note on the QR screen and on home when that note is its last item. When home has footer actions below the note, keep the 26 pt note-to-button gap. Other pages retain their existing spacing.

Use keyboard access, meaningful control labels, readable errors, and accessible selection state. Preserve a scannable QR and scrollable chat list on smaller displays. The reference's iMessage source, contact counts, branding, and network website are outside this app's scope.

## Acceptance scenarios

These scenarios define the acceptance contract. [Installation and acceptance](docs/acceptance.md) distinguishes executed deterministic checks from unexecuted live-account checks.

| Scenario | Expected outcome |
| --- | --- |
| Messages arrive immediately after QR pairing | All received initial content is saved while setup is pending; agents cannot query it yet. |
| Initial WhatsApp linking succeeds on the QR screen | Hide phone-linking steps and automatically open Choose chats, including a loading state if needed; no Continue click, overview, or separate scope-mode choice intervenes. |
| User saves initial chat scope | Open Connect your agent directly after a successful Save selection or Continue with all chats. A failed save retains the selector and explains recovery. |
| WhatsApp is linked and scope is confirmed on the overview | Show a Choose connected chats row below the disconnect row, with the saved scope's deduplicated chat count and a compact Choose button. A collapsed WhatsApp header shows a right chevron. |
| User opens Settings | Show Start at Log In with subtext Open automatically when you log in to your Mac, its switch aligned right and green when enabled, and Quit; the Local MCP card appears only on the overview. |
| User launches again with a saved WhatsApp link | Resume Choose chats when scope is unconfirmed; confirmed scope opens the overview, including when no agent connections remain. Starting sync or reconnecting transport does not repeat QR setup. |
| User exits onboarding or explicitly opens another page | Respect the chosen page through background refreshes and, while WhatsApp remains linked, window reopen; do not force the next onboarding step. Reopening without a linked account prepares QR setup. A fresh app launch resumes linking or unconfirmed chat selection from persisted state. |
| A returning user relinks WhatsApp or edits confirmed chat scope | Preserve normal navigation: successful relinking or scope saving returns to the overview without restarting agent setup. |
| Initial capture includes Work and Family; user selects Work | Family's existing content remains inaccessible through every MCP path; subsequent Family content is discarded before storage. |
| Scope changes during a history batch | After the confirmed policy transition, newly processed unselected content is not persisted or returned. |
| User searches “xyz” and selects all | Every matching conversation is selected, including off-screen results; unrelated existing selections remain. |
| A new group appears | All chats mode includes it; Selected chats mode excludes it until added. |
| User confirms Disconnect in the WhatsApp card | Stop sync and sign out the linked device; retain archive, scope, and grants. Background restart/wake remain disconnected. Opening the app again or choosing Connect prepares a new QR for a phone scan. |
| Sign-out fails or the app closes during sign-out | Keep disconnect intent, offer Retry disconnect, and prevent automatic reconnect or network actions. Authorized archive and verified cached attachments remain readable. |
| User opens MCP with no connections | Open Connect your agent with one Copy instructions action and inline Do it manually; no name field or client selector. Pending scope and service errors remain explicit. |
| User opens MCP with saved connections | Show Local MCP and expandable home-style connection cards with the uniform apple.terminal icon, name, last request/sending subtitle, and chevron. Preserve older names and ignore saved icon metadata; use terminal only when apple.terminal is unavailable. Add new connection opens the same setup screen. |
| User expands Do it manually | Create one automatically named connection when needed; show inline URL and Token rows with each label, value, and copy icon on one line. Use the full token with middle truncation only if needed; copy full values and remain in setup. Sending starts disabled and confirmed scope is required. |
| User clicks Show beside Credentials on any saved active connection | Retrieve that connection's existing URL and token through private app control and display the shared inline rows inside its card; the button becomes Hide. Do not create or rotate a grant, change permissions, or expose other connections' credentials. |
| User collapses credentials, leaves setup, or quits | Clear transient in-app credentials/instructions as applicable; leave explicitly copied URLs, tokens, and instructions available for pasting. Keep the saved connection until deleted; an unredeemed code still expires on the server. |
| User clicks Copy instructions | Create and copy instructions containing a generic HTTP exchange endpoint, single-use pairing code, and ten-minute expiry, omitting the durable credential. Assign the first unused Connection N name automatically; icons require no per-connection state. |
| Two agents connect | Both share selected-chat access; each has a distinct credential and an independent sending toggle. |
| A pairing invitation is redeemed twice or after expiry | The second/expired redemption fails without granting a connection. |
| Pairing succeeds | Return the full MCP URL and persistent access token to the agent; reading is authorized immediately with no additional app approval. The agent saves the token and configures/verifies its own client. Sending starts disabled; the app recognizes its exact invitation and returns to the connection list. |
| An existing client configuration calls a removed setup/header/stdio helper | Existing bearer grants remain valid; the agent must reconfigure its client. The app does not silently edit client configuration. |
| Agent with sending disabled calls a send tool directly | The server rejects the operation. |
| Sending-enabled agent targets an unselected chat | The server rejects the operation. |
| An agent requests edit/delete/group administration | The capability is unavailable. |
| User removes a chat or revokes an agent | Subsequent content/file access is denied, including an already cached attachment. Other agents remain connected when only one is revoked. |
| User clicks Cancel or Delete in a connection confirmation | The selected action runs; confirmation revokes only that connection, and the window stays visible throughout. |
| Agent requests an attachment | The app retrieves/caches authorized media or returns an explicit recoverable/unavailable outcome. |
| Agent resumes incremental processing | Updates are scoped and replayable; expired checkpoints require an explicit recovery path. |
| App closes its panel, then Mac sleeps and wakes | Closing retains background operation; sleep is respected; wake reconnects using saved policy and credentials. |
| User focuses another app or changes Space, then closes the app window with native close | The window stays open at normal window level and can be covered by the other app; it does not follow every Space. Native close hides it and the menu bar can reopen it. |
| User chooses Quit | All app-owned processes stop. |
| WhatsApp connects and chat scope is confirmed | Local MCP access is ready without provider setup; each agent authenticates using its own manual or HTTP-pairing credential. |
| Service is configured with a non-loopback listen address | Startup rejects it rather than exposing MCP on the network. |
| An older configuration contains cloud settings | The app starts only its local service; it does not restore tunnels or external access. |
| User restarts after an incomplete setup or configuration error | The app preserves setup/policy state and asks for the specific recovery action; it does not expose all archived content. |

## Boundaries

V1 does not include external HTTP access, tunnels, hosted-agent connection flows, OAuth authorization, agent editing/deleting, group administration, an in-app agent scheduler or AI engine, a hosted archive, automatic prevention of Mac sleep, guaranteed complete WhatsApp history, or a promise that every agent product accepts the same authentication/installation method.

Routine implementation choices remain delegated to the implementer. Return to the user only when evidence requires changing a meaningful part of this experience. The [decision map](docs/wayfinder/map.md) tracks those decisions without turning the app build into a series of product interviews.

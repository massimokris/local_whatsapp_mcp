# Native application

The Swift package builds the menu bar executable. Its deployment target is macOS 13 because login registration uses `SMAppService`. Verified with Apple Swift 6.2.4 on Apple silicon; additional architectures and OS versions require their own packaging checks.

```sh
swift test --package-path native
swift build -c release --package-path native
```

A complete app bundle supplies `Contents/Helpers/localwhatsappmcp` and `Contents/Helpers/wacli`. The executable communicates with the private service through the bundled service/control executable, using the contract in [interfaces.md](../docs/implementation/interfaces.md). The service owns a loopback-only MCP endpoint; the UI exposes WhatsApp connection, chat selection, local MCP status, agent setup/management, and minimal settings. Running the bare executable without the bundled service and engine is only useful for the demo preview.

With no connections, **Connect your agent** offers **Copy instructions** to create and copy a generic HTTP pairing invitation in one action. Names such as **Connection 1** are automatic; there is no name form or client selector. Inline **Do it manually** expands a shared URL/token credentials card and saves a manual connection when needed, leaving setup open for copying. Each row has a copy icon, and the abbreviated token display still copies the full value. Existing connections appear under **Local MCP** as expandable home-style cards with last-request/sending subtitles, **Delete connection**, **Allow message sending**, and **Credentials** controls. Saved credentials are retrieved for both manual and paired connections without changing tokens or permissions. **Add new connection** opens the same setup; successful pairing returns to the list using the exact invitation identifier. The native app clears transient credentials on collapse, navigation, deletion, or Quit. Explicitly copied URLs, tokens, and instructions remain on the clipboard for pasting, and durable credentials remain in the service's private SQLite database. No client installer or stdio bridge is provided.

Successful WhatsApp linking returns automatically to the overview, retaining the initial scope choice in its footer until scope is confirmed. Subsequent launches read the persisted engine session's authentication state to avoid repeating QR setup while a linked account resumes syncing. The linked WhatsApp card expands automatically; after scope confirmation, a **Choose connected chats** row shows the saved scope's chat count and a compact **Choose** button below the disconnect row. Its collapsed header has a right chevron. MCP content access still requires confirmed scope, and its card appears only on the overview. Settings uses **Start at Log In**, with a green enabled switch. Clicking outside the window leaves it open, including native confirmation dialogs; normal window layering lets other apps cover it, and it does not follow every Space. Native close hides the window without stopping sync, and the menu bar reopens it.

## Safe UI preview

`--demo` (or `LWMCP_DEMO=1`) uses deterministic sample data in memory. It does not launch a helper, link WhatsApp, register login, change saved preferences, or read the real archive. The banner identifies every demo surface.

```sh
LWMCP_DEMO=1 LWMCP_DEMO_PAGE=chats native/.build/debug/LocalWhatsAppMCP
```

Preview pages are `home`, `qr`, `qr-hover`, `initial`, `chats`, and `settings`; `settings-enabled` previews the green login switch without registering with macOS. `disconnected`, `loading`, `linked`, `error`, `qr-error`, and `reconnect` preview connection states. `initial-linked` previews automatic routing to the chat selector before scope confirmation; `reopening-linked` previews a saved link reconnecting on the overview. Set `LWMCP_DEMO_EXPANDED=0` to preview the collapsed WhatsApp header. `catalog-loading` previews disabled selection while the initial catalog loads. MCP previews include `mcp` (no agents), `mcp-agents` (saved cards), `mcp-expanded`, `mcp-credentials`, `mcp-long` (long name and credentials), `mcp-unavailable`, and `mcp-setup` (scope not confirmed). `manual` and `pairing` open generic setup; `manual-ready` expands synthetic manual credentials and `pairing-ready` previews copied-invitation feedback. The current interface uses dark appearance. For a native rendering, set `LWMCP_CAPTURE_PATH` to an absolute PNG destination; `LWMCP_CAPTURE_EXIT=1` quits through the normal app termination path after capture. These capture variables are ignored outside demo mode.

## Verification recorded before the MCP UI simplification

The earlier 2026-10-24 run recorded thirty-three passing native tests, including ten routing tests. They cover search-scoped selection beyond visible rows, preservation of other selections, contact/phone search, canonical identity migration, saved-scope sync summaries, linked-account launch/reopen and automatic QR completion routing, local MCP readiness and loopback endpoint validation, manual credential response validation, invitation expiry and exact redemption identity, owned-process lifecycle, abrupt owner death, shutdown request rejection, bounded startup errors, and fail-closed state decoding. Isolated synthetic native pointer checks also verified confirmation cancellation/removal, outside-click persistence, and native close/reopen. A separate focused app covered the still-open preview at normal window level; card/settings renders verified the revised WhatsApp row, chevron, login label, and green enabled switch. These checks predate the simplified MCP screens. Consult [acceptance evidence](../docs/acceptance.md) for subsequent verification.

Earlier implementation screenshots and callback tests belong to the original UI/cloud build, preserved in the [acceptance history](../docs/acceptance.md). They are not verification of the current local-only interface.

Actual WhatsApp linking, client installation/reload, login-item approval, and Mac sleep/wake remain integration/installation acceptance checks. Demo screenshots do not establish account or client compatibility.

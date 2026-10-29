# Native application

The Swift package builds the menu bar executable. Its deployment target is macOS 13 because login registration uses `SMAppService`. Verified with Apple Swift 6.2.4 on Apple silicon; additional architectures and OS versions require their own packaging checks.

```sh
swift test --package-path native
swift build -c release --package-path native
```

A complete app bundle supplies `Contents/Helpers/localwhatsappmcp` and `Contents/Helpers/wacli`. The executable communicates with the private service through the bundled service/control executable, using the contract in [interfaces.md](../docs/implementation/interfaces.md). The service binds MCP to loopback and accepts supported ngrok hosts through its built-in allowlist; the UI exposes WhatsApp connection, chat selection, local MCP status, agent setup/management, and minimal settings. Running the bare executable without the bundled service and engine is only useful for the demo preview.

Initial onboarding follows **Link WhatsApp → Choose chats → Connect your agent**.

The app displays and copies local connection URLs. For a remote agent, the user supplies the current HTTPS ngrok base as described in [Connect through ngrok](../README.md#connect-through-ngrok). The native app does not discover, configure, or manage the tunnel.

## Safe UI preview

`--demo` (or `LWMCP_DEMO=1`) uses deterministic sample data in memory. It does not launch a helper, link WhatsApp, register login, change saved preferences, or read the real archive. The banner identifies every demo surface.

```sh
LWMCP_DEMO=1 LWMCP_DEMO_PAGE=chats native/.build/debug/LocalWhatsAppMCP
```

Preview pages are `home`, `qr`, `qr-hover`, `initial`, `chats`, and `settings`; `settings-enabled` previews the green login switch without registering with macOS. `disconnected`, `loading`, `linked`, `error`, `qr-error`, and `reconnect` preview connection states. `initial-linked` previews automatic routing to the chat selector before scope confirmation; `reopening-linked` previews a saved link reconnecting on the overview. Set `LWMCP_DEMO_EXPANDED=0` to preview the collapsed WhatsApp header. `catalog-loading` previews disabled selection while the initial catalog loads. MCP previews include `mcp` (no agents), `mcp-agents` (saved cards), `mcp-expanded`, `mcp-credentials`, `mcp-long` (long name and credentials), `mcp-unavailable`, and `mcp-setup` (scope not confirmed). `manual` and `pairing` open generic setup; `manual-ready` expands synthetic manual credentials and `pairing-ready` previews copied-invitation feedback. The current interface uses dark appearance. For a native rendering, set `LWMCP_CAPTURE_PATH` to an absolute PNG destination; `LWMCP_CAPTURE_EXIT=1` quits through the normal app termination path after capture. These capture variables are ignored outside demo mode.

`synced` and `syncing` preview the exact catch-up headers with 1,292 connected chats. `synced` has confirmed `sync_state: "caught_up"` and an older historical timestamp; `syncing` has pending work and a recent timestamp. Header success requires explicit current completion and healthy connection state. Missing or unfamiliar sync states, service read failures, reconnects, wake, and scope changes cannot retain a cached success indication.

`home-mcp` previews the collapsed Local MCP card; `home-mcp-expanded` and its `home-mcp-credentials` alias preview expanded connection controls with synthetic credentials loaded automatically. The rows are URL, Token, Allow message sending, and Delete connection, in that order. URL and Token show their values beneath their labels with Copy buttons on the right. The WhatsApp card puts Choose connected chats before Stop syncing WhatsApp. This simplified surface manages the first connection in name/ID order. With no saved connection, it opens initial agent setup; completing setup returns to the expanded home card. The home cards scroll so expanded credentials remain reachable on shorter screens.

The Local MCP card subtitle shows that connection's last request age and message-sending permission, such as `Last request 2m ago - Message sending on`. A saved connection without request activity shows `No request yet` followed by its actual sending permission. Relative ages refresh every minute. With no saved connection, the subtitle is `Create MCP server connection`.

## Verification

Demo screenshots do not establish actual WhatsApp linking, client configuration/reload, login-item approval, or sleep/wake compatibility; those require live installation checks.

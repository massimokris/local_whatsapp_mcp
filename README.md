# LocalWhatsAppMCP

A native macOS menu bar app that connects your WhatsApp to your preferred local agents through **MCP**. Keep messages on your Mac, choose which chats agents can access, with the local MCP service enforcing chat access.

![Setup walkthrough: connect WhatsApp, continue with all chats, and copy instructions to connect your agent.](docs/images/setup.gif)

*Setup preview with sample data; WhatsApp linking and agent setup are simulated.*

**Status:** local-only access with a dark WhatsApp interface and local MCP connection setup and management. The installation build targets Apple silicon and macOS 13 or newer. It is ad-hoc signed, not Developer ID signed or notarized.

## Install and connect

Open the DMG produced under `dist/`, drag the app to Applications, and launch it. Quit an existing copy before replacing it; your application data is retained. An unlinked account opens directly to the QR connection page:

1. Connect WhatsApp by scanning the QR code from WhatsApp’s Linked Devices screen.
2. **Choose chats** opens automatically after linking. Save your selection, or continue with all chats, to choose what is retained and available to agents.
3. **Connect your agent** opens automatically after initial scope confirmation. Use **Copy instructions**: click once and give the copied text to your agent. It exchanges the temporary code over local HTTP, saves its token, and configures its own MCP connection. Names such as **Connection 1** are automatic. Alternatively, expand **Do it manually** and use the copy icons beside the MCP URL and access token. The full token is displayed with middle truncation when needed; copying retains its full value.
4. Existing connections appear in expandable cards on **Local MCP**, showing last-request activity and sending status. Expand a card to **Allow message sending**, open **Credentials**, or **Delete** the connection with confirmation. **Add new connection** reopens setup. Use **Choose** on the WhatsApp card to update shared scope and the settings button for **Start at Log In** or **Quit**.

The MCP server starts automatically and is available only on this Mac, through loopback HTTP. Each pairing code expires after ten minutes and can create one connection with its own revocable credential; sending starts disabled. The agent handles its client's configuration and reload. There is no public endpoint, tunnel setup, or browser login.

Clients configured by older releases to invoke a bundled helper must be reconfigured by their agent using the new setup instructions.

The panel can close while sync and MCP access continue. Quit stops the app-owned processes. The app honors Mac sleep; local access depends on the Mac being awake and the app running. Launch at login uses native macOS registration and may require OS approval. WhatsApp controls how much history is available; attachments download on demand. Your agent handles reasoning, schedules, and notifications.

Messages, linked-device state, configuration, and cached media live in `~/Library/Application Support/LocalWhatsAppMCP/`. Replacing the app preserves that data.

## Build and verify

On macOS with Xcode command line tools, a valid macOS SDK, Swift 5.9 or newer, Git, and Go 1.27.3:

```sh
./scripts/test.sh
./scripts/package.sh
python3 ./scripts/smoke-bundle.py dist/LocalWhatsAppMCP.app
```

`test.sh` runs native, backend race, pinned-engine, and packaging checks. `package.sh` creates the app, DMG, ZIP, and SHA256 files under `dist/`; `./scripts/build.sh` builds only the app. The bundle smoke test uses synthetic data and temporary local SQLite credentials. Live WhatsApp linking and client connection checks remain part of your installation test. The scripts define target architecture, signing, and notarization options.

## Architecture and project documents

SwiftUI/AppKit provides the native app. A bundled Go service enforces scope and agent permissions through the official MCP Go SDK, and coordinates a pinned wacli executable. wacli owns the WhatsApp connection and SQLite/FTS5 archive; app-managed secrets use a private local SQLite database, without Keychain prompts.

- [Domain language](CONTEXT.md): shared terminology.
- [Component interfaces](docs/implementation/interfaces.md): service, engine, and native boundaries.
- [Wayfinder map](docs/wayfinder/map.md): decisions and integration research.
- [Agent instructions](AGENTS.md): reading routes and working agreements.

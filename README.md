# LocalWhatsAppMCP

A native macOS menu bar app that connects your WhatsApp to your preferred agents through **MCP**. Keep messages on your Mac, choose which chats agents can access, and enable sending independently for each agent.

**Status:** implemented and packaged for local installation and testing. The current package targets Apple silicon and macOS 13 or newer; it was tested on the build Mac. It is ad-hoc signed, not Developer ID signed or notarized. See [acceptance evidence and remaining live checks](docs/acceptance.md).

## Install and connect

Open `dist/LocalWhatsAppMCP-0.1.0-arm64.dmg`, drag the app to Applications, and launch it. Open its menu bar icon, then:

1. Connect WhatsApp by scanning the QR code from WhatsApp’s Linked Devices screen.
2. Continue with all chats, or select the chats and groups to retain and make available to agents.
3. Give your local agent the generated setup instructions and one-time pairing token. The bundled helper saves its credential in Keychain and configures the connection.
4. Enable sending for individual agents when wanted. Reading access starts immediately after pairing.
5. Optionally use [cloud setup](docs/cloud-setup.md) for ngrok or a named Cloudflare Tunnel.

The panel can close while sync and MCP access continue. Quit stops the app-owned processes. The app honors Mac sleep, and cloud access depends on the Mac being awake and the app running. Launch at login uses native macOS registration and may require OS approval. WhatsApp controls how much history is available; attachments download on demand. Your agent handles reasoning, schedules, and notifications.

Messages, linked-device state, configuration, and cached media live in `~/Library/Application Support/LocalWhatsAppMCP/`. Replacing the app preserves that data. [Installation and signing details](docs/building.md#local-installation) explain the local build’s Gatekeeper behavior.

## Build and verify

On macOS with Xcode command line tools, a valid macOS SDK, Swift 5.9 or newer, Git, and Go 1.27.3:

```sh
./scripts/test.sh
./scripts/package.sh
python3 ./scripts/smoke-bundle.py dist/LocalWhatsAppMCP.app
```

`test.sh` runs native, backend race, pinned-engine, and packaging checks. `package.sh` creates the app, DMG, ZIP, and SHA256 files under `dist/`; `./scripts/build.sh` builds only the app. The bundle smoke test uses synthetic data and temporary Keychain credentials. Live account and provider setup remain part of your installation test. See [the build guide](docs/building.md) for individual commands, target architectures, signing, and notarization.

## Architecture and project documents

SwiftUI/AppKit provides the native app. A bundled Go service enforces scope and agent permissions through the official MCP Go SDK, and coordinates a pinned wacli executable. wacli owns the WhatsApp connection and SQLite/FTS5 archive; app-managed secrets use macOS Keychain.

- [Specification](SPEC.md): accepted behavior and acceptance scenarios.
- [Domain language](CONTEXT.md): shared terminology.
- [UI reference](docs/ui-reference.md): Dock visual conventions adapted for this app.
- [Component interfaces](docs/implementation/interfaces.md): service, engine, and native boundaries.
- [Wayfinder map](docs/wayfinder/map.md): decisions and integration research.
- [Agent instructions](AGENTS.md): reading routes and working agreements.

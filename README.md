# LocalWhatsAppMCP

A native macOS menu bar app that connects your WhatsApp to your preferred agents through **MCP**. Keep messages on your Mac, choose which chats agents can access, and optionally let each agent send text and attachments.

**Status:** specification and planning. The app has not been implemented or packaged yet.

## Planned experience

1. Connect WhatsApp by scanning a QR code. The app saves the history WhatsApp makes available.
2. Continue with all chats, or select specific chats and groups to keep synced and accessible to agents.
3. Give your local agent the generated setup instructions and one-time pairing token.
4. Manage connected agents and enable message sending individually when wanted.
5. Optionally connect cloud agents through ngrok or Cloudflare Tunnel, configured by your local agent.

The app stays in the menu bar and opens at login. It respects Mac sleep; cloud access depends on the Mac and app being available. Attachments download on demand. Your agent handles summaries, task extraction, recurring work, and notifications.

## Architecture

**SwiftUI + AppKit** provide the native app, following the visual style of [Open Ratio in the Dock project](/Users/massi/development/dock). A bundled **Go** service exposes MCP through the official Go SDK and coordinates a pinned **wacli** executable. Messages and linked-device state use **SQLite**; app-managed secrets use **macOS Keychain**.

## Project documents

- [Specification](SPEC.md): agreed behavior, stack, boundaries, and acceptance scenarios.
- [Wayfinder map](docs/wayfinder/map.md): decisions and technical integration research.
- [UI reference](docs/ui-reference.md): source-grounded visual conventions.
- [Domain language](CONTEXT.md): shared terminology.
- [Agent instructions](AGENTS.md): where to read before changing the project.

## Development

There is no build or installation command yet. The planned distribution is a native `.app` and `.dmg`, built with Swift Package Manager and Go. Build requirements and supported macOS versions will be documented when the actual package is verified.

Start with [AGENTS.md](AGENTS.md) and the [local tracker](docs/agents/issue-tracker.md) when continuing the work.

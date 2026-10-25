# LocalWhatsAppMCP — Agent Instructions

## Overview

LocalWhatsAppMCP is a native macOS menu bar app that connects the user's WhatsApp to multiple agents running on the same Mac through local MCP. The app owns sync, storage, chat scope, and access; the chosen agents own reasoning, schedules, and notifications.

The implementation uses SwiftUI/AppKit in `native/`, the Go MCP service and private app control in `backend/`, and a pinned wacli patch in `vendor/wacli/`. SQLite/FTS5 stores the archive. MCP binds only to loopback; public HTTP exposure, tunnels, and cloud OAuth are outside the current scope. The local trackers retain accepted decisions and implementation history.

## Read before changing

| Changing | Read |
| --- | --- |
| Product behavior, onboarding, access, or scope | [Decision map](docs/wayfinder/map.md) for accepted decisions and their revisions, and [component interfaces](docs/implementation/interfaces.md) for current integration contracts. |
| Domain terminology | [Domain language](CONTEXT.md) for distinctions between WhatsApp linking, agent pairing, chat scope, and archive contents. |
| UI, interaction, copy, or accessibility | [Native guide](native/README.md) for safe previews and [native views](native/Sources/LocalWhatsAppMCP) for the existing presentation and interaction patterns. |
| WhatsApp ingestion or archive access | [Selective-storage research](docs/research/selective-storage.md) for upstream insertion points and required policy coverage. |
| MCP authentication or agent installation | [Agent-pairing research](docs/research/agent-pairing.md) for client capabilities and the fixed setup experience. |
| Process ownership, native/service integration, or engine JSON | [Component interfaces](docs/implementation/interfaces.md) and [engine integration](vendor/wacli/README.md). |
| Building, packaging, or signing | [Build entry points](README.md#build-and-verify) and the relevant [scripts](scripts) for commands and release options. |
| Planning, claiming, or resolving decisions | [Decision map](docs/wayfinder/map.md) and [tracker operations](docs/agents/issue-tracker.md). |

## Working agreements

- Preserve the agreed setup experience. The user delegated authentication and other integration details; introduce a product question only when evidence requires a material experience change.
- Keep initial capture, confirmed ingestion scope, and MCP access distinct. Old content on disk is never permission to expose it.
- Enforce chat scope and per-agent sending permission in server code on every content/action path. Use specific MCP operations rather than unrestricted CLI execution.
- Keep the wacli patch focused on required integration behavior. Preserve the documented JSON/events boundary and let wacli own its WhatsApp connection/store writes.
- Store operational state and private media in the application's data directory. Keep real account data and secrets out of source files, logs, and fixtures.
- Follow the existing dark native visual language. Keep WhatsApp linking and local MCP pairing distinct; expose local connection status, setup, and per-agent access controls.

## Coding standards

Use meaningful domain names and small interfaces. SwiftUI views render state and invoke actions; put blocking I/O and process work outside the main actor. Keep shared UI state on `@MainActor`. The Go service owns backend policy and integration coordination; upstream-owned WhatsApp persistence stays behind its adapter.

Use deterministic fixtures for scope transitions, identity aliases, credential revocation, and send permissions. Test failure/restart paths that could expose unselected content or duplicate sends. Present file/configuration failures clearly and retain recoverable state.

## Build and verification

Run from the project root on macOS:

- `./scripts/test.sh`: native tests, backend race tests/vet, engine suites with and without FTS5, and packaging checks.
- `./scripts/build.sh`: build and verify the native app with bundled Go service and pinned engine.
- `./scripts/package.sh`: build, verify, and package the app as DMG/ZIP with checksums.
- `python3 ./scripts/smoke-bundle.py dist/LocalWhatsAppMCP.app`: exercise the actual bundle using synthetic data and a temporary local credential database, with no WhatsApp login.

The scripts define toolchain, architecture, signing, and release options. Ad-hoc package verification does not establish Developer ID notarization or runtime compatibility on an untested Mac.

For documentation changes, check relative links, tracker metadata/dependencies, and agreement with retained decisions. Keep synthetic regression evidence distinct from live WhatsApp, client reload, provider login, and sleep/wake checks. Documentation checks do not establish WhatsApp or client compatibility.

## Git and maintenance

Inspect status before editing; keep work inside this project unless the task names another path. The sibling `whatsapp-mcp/` experiment and reference projects are separate work. Stage or commit only owned changes when requested; do not push as part of local planning.

The glossary defines terms, the map indexes decision tickets, and component interfaces describe current integration contracts. Update the narrowest relevant document and preserve resolution history. These local planning documents are intended project artifacts; do not copy Dock's unrelated rule that its `.scratch/` plans are ignored.

# LocalWhatsAppMCP — Agent Instructions

## Overview

LocalWhatsAppMCP is a native macOS menu bar app that connects the user's WhatsApp to multiple local or cloud agents through MCP. The app owns sync, storage, chat scope, and access; the chosen agents own reasoning, schedules, and notifications.

This folder currently contains the specification and local Wayfinder tracker. The accepted stack is SwiftUI/AppKit, a Go MCP service, pinned wacli, SQLite/FTS5, and optional ngrok/cloudflared. Treat these as intended architecture, not implemented modules.

## Read before changing

| Changing | Read |
| --- | --- |
| Product behavior, onboarding, access, or scope | [Specification](SPEC.md) for the accepted contract and acceptance scenarios. |
| Domain terminology | [Domain language](CONTEXT.md) for distinctions between WhatsApp linking, agent pairing, chat scope, and archive contents. |
| UI, interaction, copy, or accessibility | [UI reference](docs/ui-reference.md) for the Dock source references and adaptation rules. |
| WhatsApp ingestion or archive access | [Selective-storage research](docs/research/selective-storage.md) for upstream insertion points and required policy coverage. |
| MCP authentication or agent installation | [Agent-pairing research](docs/research/agent-pairing.md) for client capabilities and the fixed setup experience. |
| Planning, claiming, or resolving decisions | [Decision map](docs/wayfinder/map.md) and [tracker operations](docs/agents/issue-tracker.md). |

## Working agreements

- Preserve the agreed setup experience. The user delegated authentication and other integration details; introduce a product question only when evidence requires a material experience change.
- Keep initial capture, confirmed ingestion scope, and MCP access distinct. Old content on disk is never permission to expose it.
- Enforce chat scope and per-agent sending permission in server code on every content/action path. Use specific MCP operations rather than unrestricted CLI execution.
- Keep the wacli patch focused on required integration behavior. Preserve the documented JSON/events boundary and let wacli own its WhatsApp connection/store writes.
- Store operational state and private media in the application's data directory. Keep real account data and secrets out of source files, logs, and fixtures.
- Follow Dock's visual language, using the reference guide rather than inheriting unrelated Ratio behavior or fixed screen dimensions.

## Coding standards

Use meaningful domain names and small interfaces. SwiftUI views render state and invoke actions; put blocking I/O and process work outside the main actor. Keep shared UI state on `@MainActor`. The Go service owns backend policy and integration coordination; upstream-owned WhatsApp persistence stays behind its adapter.

Use deterministic fixtures for scope transitions, identity aliases, credential revocation, and send permissions. Test failure/restart paths that could expose unselected content or duplicate sends. Present file/configuration failures clearly and retain recoverable state.

## Build and verification

There is no application source, package manifest, or build/test command in this folder yet. Do not report the app as runnable. Add verified commands here and in the README when implementation introduces their actual manifests/scripts.

For documentation changes, check relative links, tracker metadata/dependencies, and agreement with the specification. Runtime acceptance scenarios remain unexecuted until implementation. Documentation checks do not establish WhatsApp or client compatibility.

## Git and maintenance

Inspect status before editing; keep work inside this project unless the task names another path. The sibling `whatsapp-mcp/` experiment and reference projects are separate work. Stage or commit only owned changes when requested; do not push as part of local planning.

The specification is the product contract, the glossary defines terms, and the map indexes decision tickets. Update the narrowest authoritative document and preserve resolution history. These local planning documents are intended project artifacts; do not copy Dock's unrelated rule that its `.scratch/` plans are ignored.

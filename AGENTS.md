# LocalWhatsAppMCP — Agent Instructions

Shared project instructions for coding agents.

## Overview

LocalWhatsAppMCP connects WhatsApp to agents through **MCP**, using a native menu bar app. Agents connect on the same Mac or through a user-managed ngrok tunnel. The app owns sync, storage, chat scope and access. Agents own reasoning, schedules and notifications.

Setup follows **Link WhatsApp → Choose chats → Connect your agent**. Preserve that experience; raise a product question only when evidence requires a material change.

## Architecture

| Area | Implementation |
| --- | --- |
| App shell and UI | AppKit manages the menu bar item and panel; SwiftUI renders the dark interface. |
| State | `@MainActor AppModel` coordinates navigation, app actions and service state. |
| Native core | `LocalWhatsAppCore` contains state models, chat selection, connection summaries and the helper runtime. |
| Local service | Go owns policy, agent credentials, MCP operations and engine coordination. Private app control uses a Unix socket; MCP uses loopback HTTP. |
| WhatsApp engine | Pinned wacli owns the WhatsApp connection and archive writes. A focused patch adds policy and lifecycle integration through its JSON/events boundary. |
| Storage | SQLite/FTS5 stores the archive. Configuration, credentials and private media live in `~/Library/Application Support/LocalWhatsAppMCP/`. |

### Key architecture decisions

- **Chat scope:** Keep initial capture, confirmed ingestion scope and MCP access distinct. Existing content on disk does not authorize access. Recheck scope, credential revocation and per-agent sending permission on every content/action path.
- **Network access:** Bind MCP only to loopback. Accept the local listener and valid `*.ngrok-free.app` hosts/origins. Generate public response URLs only from validated ngrok hosts. The user manages the tunnel and supplies its URL; app-owned tunnel setup and cloud OAuth remain outside scope. Apply the same credentials and scope on both routes. Expose specific MCP operations rather than unrestricted CLI execution.
- **Engine ownership:** Keep the wacli patch focused. Preserve its connection/store ownership and documented policy handoff.
- **Agent pairing:** Copyable instructions contain a temporary, single-use code. The agent configures its own client; sending starts disabled.

## Key files

| File or directory | Purpose |
| --- | --- |
| [LocalWhatsAppMCP.swift](native/Sources/LocalWhatsAppMCP/LocalWhatsAppMCP.swift) | App entry point, menu bar item and panel lifecycle. |
| [AppModel.swift](native/Sources/LocalWhatsAppMCP/AppModel.swift) | Shared state, onboarding, chat scope and connection actions. |
| [PanelView.swift](native/Sources/LocalWhatsAppMCP/PanelView.swift), [MCPViews.swift](native/Sources/LocalWhatsAppMCP/MCPViews.swift) | WhatsApp, chat selection, settings and agent connection screens. |
| [LocalWhatsAppCore](native/Sources/LocalWhatsAppCore) | Native models and helper process/control adapter. |
| [service](backend/internal/service) | MCP authorization, pairing, credentials, scope and sends. |
| [engine](backend/internal/engine) | wacli process supervision and archive adapter. |
| [vendor/wacli](vendor/wacli) | Upstream pin, integration patch and engine build scripts. |
| [scripts](scripts), [packaging](packaging) | Tests, app assembly, signing and distribution. |
| [landing](landing) | Independent Next.js landing page and direct app download, deployable to Vercel. |

## Read before changing

| Changing | Read |
| --- | --- |
| Product behavior or chat scope | [Decision map](docs/wayfinder/map.md) for accepted decisions and revisions, and [component interfaces](docs/implementation/interfaces.md) for current contracts. |
| Domain terminology | [CONTEXT.md](CONTEXT.md) for WhatsApp linking, agent pairing, chat scope and archive contents. |
| Service integration or engine behavior | [Component interfaces](docs/implementation/interfaces.md) for ownership and contracts; [engine guide](vendor/wacli/README.md) for the upstream boundary. |
| Native UI, copy or accessibility | [Native guide](native/README.md) for safe previews and the native views above for existing patterns. |
| Landing page, download link or web deployment | [Landing guide](landing/README.md) for local commands, form behavior, assets and Vercel setup. |
| Ingestion or archive access | [Selective-storage research](docs/research/selective-storage.md) for insertion points and policy coverage. |
| MCP authentication, agent setup or ngrok | [Component interfaces](docs/implementation/interfaces.md#local-mcp-boundary) for current access and URL rules; [agent-pairing research](docs/research/agent-pairing.md) for historical client-capability evidence. |
| Planning or ticket status | [Tracker operations](docs/agents/issue-tracker.md) and the relevant decision or implementation ticket. |

## Build and run

Use macOS, Xcode Command Line Tools, Swift 5.9 or later, Git and Go 1.27.3. The app targets macOS 13 or later; builds are verified on Apple silicon. Run from the repository root:

```sh
./scripts/test.sh
./scripts/build.sh
./scripts/package.sh
python3 ./scripts/smoke-bundle.py dist/LocalWhatsAppMCP.app
```

`test.sh` runs native, backend race/vet, engine FTS5/non-FTS5 and packaging checks. `build.sh` verifies the app; `package.sh` produces DMG/ZIP files and checksums. See [Contributing](README.md#contributing) and the scripts for options. Signing is ad hoc by default; notarization requires release configuration.

For UI checks, use the in-memory demo:

```sh
swift run --package-path native LocalWhatsAppMCP --demo
```

Demo mode launches no helpers and leaves real data and saved preferences alone. Live use requires the bundled service and engine.

## Working agreements

- Keep WhatsApp linking and agent pairing distinct in the UI. Reuse the dark theme, existing controls and accessibility patterns.
- Keep real account data and secrets out of source, logs and fixtures. Store operational data in the app's data directory.
- Report synthetic checks separately from live WhatsApp, client reload, login approval and sleep/wake checks. Screenshots and bundle smoke tests do not establish live compatibility.

## Coding standards

Use meaningful domain names and small interfaces. Views render state and invoke actions. Keep shared UI state on `@MainActor`; put blocking I/O and process work outside it. Keep backend policy in the Go service and upstream persistence behind the engine adapter.

Use deterministic fixtures for scope transitions, identity aliases, credential revocation and sending permissions. Test failure/restart paths that could expose unselected content or duplicate sends. Show file/configuration failures clearly and retain recoverable state.

## Git workflow

Inspect status and diffs before editing. Work on the current branch and stage or commit only owned changes when requested. Push only when requested.

Stay within this project unless the task names another path. The sibling `whatsapp-mcp/` experiment is separate work. Local planning and tracker documents are project artifacts; preserve resolution history.

## Keeping this file current

Update ownership, file responsibilities, commands and conventions here when they change. Keep terms in `CONTEXT.md`, decisions in trackers and contracts in component interfaces. Check links, tracker metadata/dependencies and agreement with retained decisions.

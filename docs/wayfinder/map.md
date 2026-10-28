# LocalWhatsAppMCP v1 decision map

- ID: `LW-MAP`
- Status: closed
- Labels: `wayfinder:map`
- Owner: massi
- Created: 2026-10-23

## Destination

Implementation-ready decisions for LocalWhatsAppMCP v1 that preserve the agreed experience, with concrete integration routes for selective storage and agent pairing. Building and shipping the app is a subsequent effort.

## Notes

- The live conversation established the product requirements and stack. [Define the v1 product contract](issues/001-product-contract.md) records that existing agreement; this charting session does not reopen it.
- Use the local Markdown tracker and its [Wayfinding operations](../agents/issue-tracker.md#wayfinding-operations). Open children are found from ticket metadata, not duplicated in this index.
- Use the wayfinder skill for map work, research for external facts, and domain-modeling when terminology changes. Use grilling only when evidence exposes a consequential user decision; routine integration details belong to the implementer.
- Keep the user experience fixed: QR connection, immediate initial capture, configurable chat scope, copyable agent setup instructions with a one-time pairing token, per-agent sending opt-in, and locally running MCP. The [ngrok host allowance](comments/001-product-contract.md#ngrok-host-allowance--2026-10-27) permits requests through a user-managed tunnel without restoring the app-owned cloud setup removed in the [local-only revision](comments/001-product-contract.md#local-only-revision--2026-10-23).
- The user chose dark screenshots as the visual direction for WhatsApp and local MCP connection setup and management. The public app name is **LocalWhatsAppMCP** and the project folder is **local_whatsapp_mcp**.
- Research notes live in `docs/research/`; their tickets record the isolated research branch and source evidence. Research establishes implementation direction, not a claim of tested client compatibility.

## Decisions so far

- [Define the v1 product contract](issues/001-product-contract.md) — the conversation settled the experience, access boundaries, operating behavior, stack, and visual reference. The local-only revision removed app-owned cloud setup; the [ngrok host allowance](comments/001-product-contract.md#ngrok-host-allowance--2026-10-27) adds request-host support for user-managed tunnels. The current native UI includes WhatsApp and local MCP connection setup.
- [Enforce selected chats across ingestion and MCP access](issues/002-selective-storage.md) — keep a narrow policy patch inside wacli and independent per-request access checks in the Go wrapper; the research identifies write paths, side effects, identity handling, and the policy transition.

- [Preserve one-time pairing across MCP client connections](issues/003-agent-pairing.md) — the [generic HTTP pairing revision](comments/003-agent-pairing.md#generic-http-pairing-revision--2026-10-24) replaces client installers with temporary codes exchanged over loopback HTTP; each agent saves its token and configures its own client. The [MCP connection UI simplification](comments/003-agent-pairing.md#mcp-connection-ui-simplification--2026-10-24) adds one-action instruction copying, inline manual credentials, automatic names, and expandable connection cards; the [home connection controls revision](comments/003-agent-pairing.md#home-connection-controls--2026-10-27) moves the first saved connection's controls into the overview's Local MCP card. Private credential retrieval supports both paired and manual connections. Prior helper and cloud designs remain historical evidence; actual client checks remain explicit.

## Not yet specified

No additional product area currently needs discovery. If the integration research exposes a constraint that changes the agreed experience, describe that consequence here before creating a new decision ticket. Ordinary implementation and acceptance testing do not become new product questions.

## Out of scope

- Building or shipping the application in this map; the destination is settled decisions and a clear integration route.
- Agent editing/deleting of messages and group administration, explicitly removed from v1.
- An in-app AI assistant, scheduler, or task-extraction engine; the connected agent owns those workflows.
- App-owned tunnel/provider setup, hosted-agent OAuth, hosted WhatsApp storage, and an always-on cloud worker. User-managed ngrok requests are covered by the current host allowance.
- Guaranteed complete historical recovery or unconditional compatibility with every named agent product.

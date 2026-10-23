# LocalWhatsAppMCP v1 decision map

- ID: `LW-MAP`
- Status: open
- Labels: `wayfinder:map`
- Owner: massi
- Created: 2026-10-23

## Destination

An implementation-ready [LocalWhatsAppMCP v1 specification](../../SPEC.md) that preserves the agreed experience, with concrete integration decisions for selective storage and agent pairing. Building and shipping the app is a subsequent effort.

## Notes

- The live conversation established the product requirements and stack. [Define the v1 product contract](issues/001-product-contract.md) records that existing agreement; this charting session does not reopen it.
- Use the local Markdown tracker and its [Wayfinding operations](../agents/issue-tracker.md#wayfinding-operations). Open children are found from ticket metadata, not duplicated in this index.
- Use the wayfinder skill for map work, research for external facts, and domain-modeling when terminology changes. Use grilling only when evidence exposes a consequential user decision; routine integration details belong to the implementer.
- Keep the user experience fixed: QR connection, immediate initial capture, configurable chat scope, copyable agent setup instructions with a one-time pairing token, per-agent sending opt-in, and optional agent-configured cloud access.
- [Dock is the UI reference](../ui-reference.md). The public app name is **LocalWhatsAppMCP** and the project folder is **local_whatsapp_mcp**.
- Research notes live in `docs/research/`; their tickets record the isolated research branch and source evidence. Research establishes implementation direction, not a claim of tested client compatibility.

## Decisions so far

- [Define the v1 product contract](issues/001-product-contract.md) — the conversation settled the v1 experience, access boundaries, operating behavior, stack, and visual reference.
- [Enforce selected chats across ingestion and MCP access](issues/002-selective-storage.md) — keep a narrow policy patch inside wacli and independent per-request access checks in the Go wrapper; the research identifies write paths, side effects, identity handling, and the policy transition.

## Not yet specified

No additional product area currently needs discovery. If the integration research exposes a constraint that changes the agreed experience, describe that consequence here before creating a new decision ticket. Ordinary implementation and acceptance testing do not become new product questions.

## Out of scope

- Building or shipping the application in this map; the destination is a specification and clear integration route.
- Agent editing/deleting of messages and group administration, explicitly removed from v1.
- An in-app AI assistant, scheduler, or task-extraction engine; the connected agent owns those workflows.
- Hosted WhatsApp storage or an always-on cloud worker; cloud access tunnels to the running Mac.
- Guaranteed complete historical recovery or unconditional compatibility with every named agent product.

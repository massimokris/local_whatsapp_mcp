# Resolution: Define the v1 product contract

- Date: 2026-10-23
- Ticket: [Define the v1 product contract](../issues/001-product-contract.md)
- Recorded by: Codex, from the user's explicit answers and corrections in the originating conversation
- Result: closed; agreement captured, implementation not started

The user corrected the proposed selection-before-import flow: initial connection saves all received history immediately, then confirmed chat scope governs further storage and all agent access. Initial unselected records may remain stored but cannot be retrieved through MCP.

The conversation established local-agent users as the first audience; simultaneous independent agent connections; shared chat scope; sending text/attachments only after a per-agent opt-in; normal sleep; on-demand media; and a background menu bar app that launches at login. Editing, deleting, and group administration were explicitly removed from v1.

The user fixed the pairing experience and delegated its implementation: generated instructions and a one-time token, automatic reading access on redemption, credentials installed by the local agent, optional agentic tunnel setup with ngrok as default and cloudflared supported. A normal browser connection step for a cloud client was accepted without manual OAuth/header setup. Subsequent instructions reaffirmed that implementation must preserve this experience.

The user accepted the SwiftUI/AppKit, Go, pinned wacli, official MCP Go SDK, SQLite/FTS5, Keychain, ServiceManagement, ngrok/cloudflared, and native app/DMG stack. They chose `/Users/massi/development/dock` as the visual reference and requested concise documentation in the style of that project and `keyboard-template`.

No additional product question is currently blocking. The two research children establish technical routes within this agreement; only evidence requiring a material change to the experience should return to the user.

## Local-only revision — 2026-10-23

The user explicitly removed external HTTP exposure for the current product and requested that WhatsApp connection lead to a local MCP server. This supersedes the optional cloud/tunnel portions of the agreement above. The revised agreement keeps the native app, WhatsApp linking and capture, confirmed chat scope, and MCP service on this Mac, with no public endpoint, provider setup, tunnel management, hosted-client OAuth, or browser callback flow.

Local loopback HTTP remains the MCP transport, with the bundled stdio bridge for clients that need it. Pairing remains a short-lived, single-use invitation: the helper privately installs each local agent's durable credential in Keychain. That preserves independent revocation and initially disabled sending permission without manual authentication configuration. Starting the service is automatic; agent content access still waits for confirmed scope and pairing.

Earlier cloud research, implementation resolutions, and build evidence are historical records. They do not expand the current product scope or establish acceptance of this revision. No new live WhatsApp or client result is claimed by this contract update.

## Current native UI scope — 2026-10-23

The user subsequently directed the interface to focus on WhatsApp and defer agent setup and management until a replacement flow is defined. The dark screenshots supersede the earlier Dock styling. This narrows the native surfaces at this point in the history; local MCP, chat-scope enforcement, and existing agent credentials remain backend responsibilities. The helper pairing contract is retained without claiming that its former native setup screens remain available.

## ngrok host allowance — 2026-10-27

The user requested that the backend accept additional request hosts/origins, specifically valid `*.ngrok-free.app` addresses, so an existing user-managed tunnel can reach pairing and MCP. This narrows the earlier local-only restriction while retaining the loopback listener, private app control, existing bearer authentication, shared chat scope and per-connection sending permission.

The user owns the tunnel and supplies its HTTPS URL to the remote agent. Copied instructions and saved/manual connection details remain local; the user replaces their local base for remote setup. HTTP pairing and attachment responses through an allowed ngrok Host generate HTTPS URLs for that validated host. The change adds no native settings, saved public URL, tunnel discovery, provider setup or OAuth. The [component interfaces](../../implementation/interfaces.md#local-mcp-boundary) define the host/origin rules. Earlier decisions remain historical evidence; this records accepted scope rather than live provider/client verification.

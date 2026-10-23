# Resolution: Define the v1 product contract

- Date: 2026-10-23
- Ticket: [Define the v1 product contract](../issues/001-product-contract.md)
- Recorded by: Codex, from the user's explicit answers and corrections in the originating conversation
- Result: closed; agreement captured, implementation not started

The user approved the product behavior now expressed as the acceptance contract in [SPEC.md](../../../SPEC.md). In particular, they corrected the proposed selection-before-import flow: initial connection saves all received history immediately, then confirmed chat scope governs further storage and all agent access. Initial unselected records may remain stored but cannot be retrieved through MCP.

The conversation established local-agent users as the first audience; simultaneous independent agent connections; shared chat scope; sending text/attachments only after a per-agent opt-in; normal sleep; on-demand media; and a background menu bar app that launches at login. Editing, deleting, and group administration were explicitly removed from v1.

The user fixed the pairing experience and delegated its implementation: generated instructions and a one-time token, automatic reading access on redemption, credentials installed by the local agent, optional agentic tunnel setup with ngrok as default and cloudflared supported. A normal browser connection step for a cloud client was accepted without manual OAuth/header setup. Subsequent instructions reaffirmed that implementation must preserve this experience.

The user accepted the SwiftUI/AppKit, Go, pinned wacli, official MCP Go SDK, SQLite/FTS5, Keychain, ServiceManagement, ngrok/cloudflared, and native app/DMG stack. They chose `/Users/massi/development/dock` as the visual reference and requested concise documentation in the style of that project and `keyboard-template`.

No additional product question is currently blocking. The two research children establish technical routes within this agreement; only evidence requiring a material change to the experience should return to the user.

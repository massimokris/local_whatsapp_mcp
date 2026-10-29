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

## Isolated message failures — 2026-10-29

The user explicitly requested simpler sync health: an isolated message that cannot yet be decrypted or retrieved must not put the whole app into an error state while other messages are arriving and saving. This supersedes the strict failure presentation introduced by the October 27 sync-state implementation (`4ac2b85`) and refined on October 28 (`2ee6f27`). Those changes and their recorded results remain historical evidence.

Active received work still appears as syncing. After the current delivery boundary drains, known isolated message gaps produce a neutral connected state with quiet missing-message context. Verified completion without gaps retains the success label. Blocking storage/journal failures and real authentication or connection failures remain actionable. Durable recovery evidence is retained until matching recovery; a quieter presentation does not claim the missing messages were saved.

The approved representation is the additive `incomplete` sync state, defined in the [current sync contract](../../implementation/interfaces.md#whatsapp-sync-state). It preserves the existing setup flow, design, scope enforcement, and agent permissions. The [implementation task](../../implementation/issues/107-sync-health.md) owns acceptance and verification. The user authorized implementation, independent review, and tests without reconnecting personal WhatsApp, and will decide the next step afterward. No new live compatibility or server-401 cause is established by this revision.

### Review refinement — 2026-10-29

Review found that a decryption callback can also report a failed local store, and that older undecryptable-message metadata cannot reliably distinguish an earlier storage failure. The coordinator's accepted refinement limits quiet gaps to proven content failures and known, nonfailed message/history work that is not actively being processed. Actual replay remains syncing, and the current delivery boundary is still required before showing a quiet connected state.

An older saved failure without a reliable classification remains an error until matching recovery succeeds. Users with such records may still see the prior warning after this update; no data is deleted or reset to remove it. New blocking classifications remain distinct from absent legacy classifications, and storage, authentication, and unknown underlying failures remain actionable. This refines the October 29 acceptance criteria without changing the earlier historical decisions or claiming those review findings have already passed final verification.

## Upstream-owned sync — 2026-10-29

The user subsequently rejected the app-owned synchronization-completion and recovery model and requested its removal. This explicitly supersedes the strict completion/error state introduced on October 27 and the isolated-message-gap revision above. Both prior decisions and their implementation records remain historical evidence.

The app observes ordinary connection, linking and logout state, preserves its existing setup flow and chat-count presentation, and makes no claim that messages are caught up or that all history is saved. It removes the custom completion counters, delivery barriers, health snapshots/watchdog, synced timestamp, recovery journal, replay/repair extension and associated UI. It uses the exact pinned upstream WhatsApp SDK without the local delivery patch. Upstream protocol, session/message persistence, history processing, decryption, acknowledgements, retries and native recovery behavior continue normally.

The local app retains policy enforcement, chat scope, agent credentials and sending permission, process/writer ownership, safe explicit relinking and actual connection-error handling. Connection-only diagnostics remain private and do not become another synchronization monitor. Legacy custom journal files are ignored, not interpreted as failures and not automatically deleted; personal archive/session data and upstream protocol state are unchanged.

The [removal task](../../implementation/issues/108-upstream-sync.md) owns implementation and isolated verification. The [current connection contract](../../implementation/interfaces.md#whatsapp-connection-status) replaces the old sync-state contract. No new personal WhatsApp attempt, data reset, installation, release, commit or push is implied. This decision changes the app's responsibilities; it does not establish why WhatsApp issued the historical 401.

## Repeat onboarding after relinking — 2026-10-29

The user requested that reconnecting WhatsApp after disconnecting it repeat the onboarding steps, even for the same account: choose chats, then copy agent setup instructions. A new explicit QR-link attempt from an observed unlinked account therefore repeats the native setup flow. Ordinary temporary reconnects, wake and relaunch of a still-linked session keep their current behavior.

Saved chat choices prefill the selector and existing agent grants remain intact. Continue leads to the existing Copy instructions screen even when those agents already exist. Native UI progress can survive an interrupted link or setup, but does not change backend authorization or reset the ingestion policy to initial capture. Previously selected content remains authorized until the user saves a different selection. Archive data and credentials are not erased. This extends the onboarding routing while preserving the upstream-owned sync decision above.

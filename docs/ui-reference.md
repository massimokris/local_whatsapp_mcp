# LocalWhatsAppMCP UI reference

Read this when shaping screens, implementing native views, changing copy, or reviewing UI. The user chose `/Users/massi/development/dock` as the visual reference. This document records observed source and its intended adaptation; LocalWhatsAppMCP screens have not been implemented or visually verified.

[SPEC.md](../SPEC.md) owns product behavior and scope. The planned surfaces below translate that behavior into UI guidance; exact layouts and recovery copy remain design work.

## Source and scope

The reference app is Open Ratio, a compact SwiftUI interface hosted by AppKit. Its product is unrelated to WhatsApp; transfer its presentation and interaction language, not its activity tracking features.

| Reference | Use it for |
| --- | --- |
| [AGENTS.md](/Users/massi/development/dock/AGENTS.md) | Reference product context and native UI conventions. |
| [Theme.swift](/Users/massi/development/dock/Sources/RatioNative/Theme.swift) | Adaptive colors, button feedback, and separators. |
| [RatioTypography.swift](/Users/massi/development/dock/Sources/RatioNative/RatioTypography.swift) | Native fonts, line height, tracking, and text glyphs. |
| [MenuBarView.swift](/Users/massi/development/dock/Sources/RatioNative/MenuBarView.swift) | Compact rows, persistent status, inline navigation, and footer. |
| [PreferencesView.swift](/Users/massi/development/dock/Sources/RatioNative/PreferencesView.swift) | Label/control alignment and inline settings. |
| [RatioNativeApp.swift](/Users/massi/development/dock/Sources/RatioNative/RatioNativeApp.swift) | Menu bar anchor, panel outline, focus, and dismissal. |

Inspected on 2026-10-23; reference repository HEAD was `07ffdb1`. Recheck these files if the reference changes. No reference implementation was copied.

## Observed visual system

Use flat, closely aligned rows, restrained separators, monospaced typography, and small native icons. Content and state supply the hierarchy; the reference has no large decorative headings or nested card layout.

| Token | Light | Dark |
| --- | --- | --- |
| Background | `#FAFAFA` | `#0F0F0F` |
| Panel surface | `#EEEEEE` | `#1A1A1A` |
| Selected surface | `#F3F3F3` | `#131313` |
| Separator / pressed surface | `#DDDDDD` | `#242424` |
| Primary text | `#151515` | `#F5F5F5` |
| Secondary text | `#737373` | `#808080` |
| Muted text | `#999999` | `#666666` |
| Green accent | `#16852C` | `#28CD41` |
| Red accent | `#D72C22` | `#FF3B30` |
| Amber accent | `#A06500` | `#FFB000` |

The accent names in Ratio are `create`, `consume`, and `unknown`. For LocalWhatsAppMCP, reserve accents for meaningful connection, error, or attention states, accompanied by text. Activity-category meanings do not transfer.

- Main text: SF Mono, 12 pt regular; bold for limited emphasis; native monospaced system fallback.
- Line height: 1.5 × font size, or 18 pt at the default size. Tracking: 0.4 pt.
- Compact eyebrow helper: 10 pt uppercase. Text glyphs: Menlo Regular, 12 pt; many SF Symbol controls use 13 pt.
- Reference content: 360 × 352 pt; eight 44 pt rows. The outer panel is 360 × 360 pt including an 8 pt top anchor.
- Reference sections: 44 pt summary, 264 pt body, 44 pt footer. Main content commonly has 16 pt leading inset.
- Settings and status rows: 272 pt label/content column and 88 pt action column. Separators are 0.5 pt.
- Outer panel: 8 pt corner radius, shaped menu bar pointer, native shadow, and flat opaque content colors.
- Standard buttons use separator color while pressed. Enabled footer buttons alone use white background and black foreground on hover or press.
- Enabled controls use a pointing-hand cursor. Icon buttons have accessible labels and helpful tooltips.

## Adaptation to this product

Keep the compact native menu bar presentation, typography, palette, row rhythm, and divided footer. A QR code and searchable chat list need more room than Ratio's fixed eight-row view. Allow an expanded setup or management surface while retaining those tokens; exact expanded dimensions remain an implementation choice.

Use a scannable QR code at its appropriate display size, a scrollable conversation list, and sufficient width for names and phone numbers. Preserve readable text and accessible controls instead of shrinking everything into 360 × 352 pt.

Ratio's ratio summary, category arrows, daily history, Reset, Demo, and activity exports are incidental. LocalWhatsAppMCP needs connection status, chat scope, agent connections, and settings in their places.

## Planned onboarding

1. Start with a clear **Connect WhatsApp** action and QR scanning instructions. Show pairing progress and a recovery action when a code expires or linking fails.
2. As soon as WhatsApp connects, capture incoming available history while setup continues. Describe it as available history, with honest progress; chat selection does not precede this initial capture.
3. Present primary **Continue with all chats** and secondary **Select specific chats** below it. Explain the scope before confirmation.
4. For specific selection, show the searchable list described below, then a clear confirmation action. Chat selection establishes the scope for ongoing storage and agent access.
5. After scope confirmation, show **Connect an agent**. Local setup supplies instructions with a short-lived, single-use pairing token; successful redemption grants reading access without another approval prompt.

Keep initial capture and current access scope distinguishable in copy: an excluded conversation can still have an initial local archive while being inaccessible to agents. Never describe selection as deleting that archive.

## Chat selection and management

- Show groups and direct chats together, ordered by most recent activity, in a familiar conversation-list arrangement.
- Each row identifies the chat or group and has a check control on the right. Make long names distinguishable and expose selection state accessibly.
- Search by group/chat name, contact name, or phone number. Search filters the list; it does not silently change the current selection.
- With a search active, **Select all** selects every matching result, including matches outside the visible scroll area. It preserves existing selections outside that result set.
- Show a useful empty-search state and the current selection count so bulk selection is understandable.
- **All chats** is a persistent mode that includes future chats and groups. **Selected chats** is an explicit list; newly discovered conversations start excluded.
- Reuse this selector for later additions and removals. Show the active mode and a route to change it from the main panel.
- Saving a removal stops future storage and agent access for that chat. Previously archived content remains stored; do not imply a purge.

## Agent connections and cloud setup

Show named connections with their local/cloud context, connection state, and **Disconnect** action. Multiple agents share the WhatsApp connection and archive; each has separate credentials and permission state.

Each connection has **Enable message sending**, off by default. The UI should describe text and attachment sending within selected chats. Editing, deleting, and group administration are outside v1.

**Connect cloud agent** introduces provider choice: ngrok by default, with Cloudflare through `cloudflared` also supported. The setup instructions guide the local agent through installation, provider authentication, tunnel/hostname setup, saved local configuration, endpoint verification, and client configuration.

Reflect setup progress and actionable failures in the app. A browser authorization step is acceptable when required by a cloud client. Present long-lived credentials through secure configuration, not conversation text or ordinary status displays. Exact cloud-client compatibility remains subject to technical validation.

## Status, settings, and lifecycle

Keep WhatsApp sync, MCP availability, and tunnel availability separately understandable. A working tunnel does not establish fresh WhatsApp data. Useful states include linking, receiving history, syncing, reconnecting, disconnected, and connection failure; show freshness when known.

Offer settings for chat scope, agent connections and permissions, cloud exposure/provider, appearance, and **Open at login**. Launch at login is enabled for the intended product behavior, with visible recovery if macOS needs the user's approval.

Closing or dismissing the UI leaves sync and any configured tunnel running. **Quit** stops them. Respect normal Mac sleep and reconnect on wake; cloud setup must explain that access depends on the Mac being awake and online.

The reference supports keyboard focus, Escape dismissal, outside-click dismissal, and reopening from the menu bar. Preserve entered setup and selector state across incidental dismissal; implementation must make reopening safe while background work continues.

Attachments are fetched on demand and cached. When an agent or user requests unavailable media, show a truthful failure or recovery state; saving attachment metadata does not establish that its bytes remain downloadable.

## UI verification when implemented

Check light and dark appearance, QR scanning, keyboard search/selection, long chat names, and names that share a phone-number match. Verify search-scoped **Select all** beyond the visible rows and selection retention when the query changes.

Verify reconnect and tunnel-failure states separately, per-agent sending permissions, initial-import copy, reopening after dismissal, and explicit Quit. These are future acceptance checks; this planning document is not evidence that they pass.

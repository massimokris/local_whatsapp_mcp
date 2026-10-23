# Selective storage and MCP access

- Date: 2026-10-23.
- Decision ticket: [Enforce selected chats across ingestion and MCP access](../wayfinder/issues/002-selective-storage.md).
- Upstream inspected: `openclaw/wacli`, commit `5578d1b6892f50a8f2329b11a6b6e27a1a81f88b`.
- Method: primary-source review of the pinned implementation and documentation; no WhatsApp login, implementation, or runtime tests.

## Recommendation

Keep the agreed Swift app → Go MCP supervisor → bundled, pinned wacli boundary.
Add a small, explicit content-scope policy to wacli's app layer, retaining its existing WhatsApp client, SQLite schema, migrations, and command interface.
Enforce the same scope independently in the MCP supervisor; an ingestion filter cannot protect unselected records captured before selection.
The technical route is sufficiently established to close the research question. Implementation acceptance checks remain outstanding, not product decisions.

## Contract preserved

Initial pairing immediately captures all history/events WhatsApp delivers; there is no selection-before-import change.
After the user confirms all chats or a selected set, that scope controls subsequent content retention and all MCP access.
Previously captured unselected records may remain; this work does not introduce automatic historical deletion.
Chat/contact catalogue metadata remains available to the local app so users can discover, search, add, and remove chats later.
Media stays on demand. Agents receive the shared selected scope; sending text/files additionally requires that agent's existing send permission.
Only locally retained history can be returned; WhatsApp history delivery remains best-effort. [S1]

## Verified upstream facts

1. `runAuth` invokes `App.Sync` with QR enabled and contact/group refreshes; `auth --follow` can continue receiving initial history. [S2]
2. `newApp` constructs one `app.App` and acquires the per-store writer lock when requested. There is currently no content-scope option. [S3]
3. Live messages enter `handleLiveSyncMessage`; history enters `handleHistorySync`. Both call `storeParsedMessageForSync`, which reaches `storeParsedMessage`, sometimes through `syncStorageLimits`. [S4][S5]
4. `BackfillHistory` also calls `handleHistorySync`, including a call with `SyncOptions{}`. A filter attached only to ordinary sync options would therefore be incomplete. [S6]
5. `storeParsedMessage` canonicalizes the chat, updates chat/group/contact metadata, then writes message text, captions, attachment keys, reply references, location data, and associated call/star records. Status broadcasts use a separate branch. [S7]
6. A successful return triggers further behavior outside that function: stored counters, unread updates, live webhooks, and poll side records. Media enqueueing currently occurs outside the successful-store condition in both handlers. [S4][S8]
7. Live call events and history call logs can reach `storeParsedCallEvent` without passing through ordinary message storage. [S4][S7]
8. Group/contact refreshes already operate independently of retaining message bodies. `UpsertChatMetadata` and `upsertMessageChat` distinguish naming from activity timestamps. [S7][S14]
9. `canonicalStoreJID` resolves known LIDs to phone-number JIDs and removes device addressing from phone JIDs. Startup can migrate old LID rows; read filters expand verified phone/LID equivalents. [S10][S15][S16]
10. Exact media download checks/copies the cache before taking a writer lock; read-only mode can still download to an explicit output file. Writable downloads can delegate to the running sync owner, including exact phone recovery. [S11]
11. Message list/search support chat/time filters and bound pagination; the CLI accepts one chat selector, while store parameters contain `ChatJIDs`. Global change-feed records include chat/message IDs and have no scope filter. [S12][S17]
12. Upstream explicitly supports JSON CLI wrappers and read-only SQLite companions; it prohibits companion writes to wacli's databases and warns that their schema can evolve. [S13]

## Patch boundary and insertion points

The following is the recommended implementation, not an existing upstream feature.

| Area | Change |
| --- | --- |
| Configuration | Add a versioned policy path/snapshot through `rootFlags` → `newApp` → `app.Options`, applying to every process for this app's fixed store. |
| Policy states | Persist explicit initial capture, all chats, or selected canonical chat identities. An empty selected set means none; missing/corrupt established policy never silently means all. |
| Main message path | Split catalogue updates from content persistence in `storeParsedMessage`; preserve names/IDs/kind/last activity, then check the canonical chat before any payload-bearing database write. |
| Return contract | Return retained/skipped/error distinctly through `storeParsedMessageForSync` and `syncStorageLimits.StoreParsedMessage`. Intentional skips are neither successful content stores nor persistence failures. |
| Live handler | Only retained messages increment content counters, generate message webhooks, write poll records, or enqueue media. Keep connection/protocol bookkeeping running. |
| History handler | Maintain conversation catalogue data even when its content is excluded; skip payload persistence and pending poll/media work for excluded chats. Apply to late initial chunks and on-demand responses alike. |
| Other content paths | Apply the same predicate before `storeParsedCallEvent` writes and before status/other payload branches can bypass the main message decision. Do not add calls/status MCP tools. |
| Metadata/protocol | Continue contact/group discovery, verified identity mappings, session keys, chat-state synchronization, and deletion/tombstone handling; do not treat protocol state as agent-readable content. |
| Media defense | Check scope before cache lookup/copy, network download, phone retry, and queued media execution. Keep automatic media download disabled for the agreed on-demand flow. |

The crucial insertion point is after canonical identity/catalogue maintenance and before `UpsertMessage`, `UpsertStatusMessage`, `UpsertMessageLocation`, or related content writes. [S7]
Do not implement the filter as `return nil` alone: current callers interpret nil as retained and can persist side records or deliver content anyway. [S4][S8]
Keep the policy on `App`, not solely on `SyncOptions`, so auth, follow sync, reconnect, backfill, and delegated media use the same rule. [S2][S3][S6][S11]
Preserve the existing store ownership: the Go supervisor writes its own policy/configuration, and patched wacli owns WhatsApp-store mutations. [S13]
WhatsApp still delivers linked-device events; excluded payloads may be handled in memory to maintain protocol state, but must not enter the app's retained content or agent output.

## Applying a scope change

Use a serialized supervisor operation and a process restart for v1; a new live policy-reload protocol is unnecessary.

1. Persist the intended next policy revision in app configuration and put MCP access behind a transition barrier.
2. Immediately stop issuing new work for removed chats; cancel their pending reads/downloads and invalidate their response/cache handles.
3. Stop admitting new CLI writer operations, stop the old auth/sync owner, and await process exit and outstanding writer operations.
4. Atomically publish the next policy snapshot, then restart `sync --follow --events` against the same fixed store with that revision.
5. Require a machine-readable policy-loaded acknowledgment from the patched process before reporting that selection is applied and reopening MCP reads.
6. Persist and recover unfinished transitions on restart; recovery must never briefly start an unrestricted writer or unrestricted MCP endpoint.

The cutoff is successful application of the selection, not a message timestamp: a delayed history chunk received afterwards must obey the new policy even if every message is old.
Upstream can continue inline history persistence during shutdown, so merely sending a stop signal is not a completed cutoff; await process termination. [S9]
The process restart preserves the WhatsApp session and catalogue. It does not require QR pairing again or establish complete replay coverage. [S1][S13]
If restart fails, retain the intended policy and show a retryable sync error; do not restore unrestricted capture as a recovery shortcut.
Changing to all chats removes the retention restriction; selecting a new chat admits future received content and already-retained history without promising recovery of a missed interval.

## Canonical identities and catalogue

Store scope by account/store plus stable WhatsApp identities, never by display name or list position.
Use wacli's verified PN↔LID mapping; expand allowed identities when a verified pair becomes known and preserve the selection across migration.
An unmapped LID remains distinct until verified. Matching names, phone-like strings, or group titles must not merge authorization identities. [S10][S18]
Apply the same resolution to MCP targets, returned rows, delegated sends, and `(chat, message)` attachment keys.
Group IDs retain their group identity; a selected group's membership must not automatically authorize a participant's direct chat.
Keep excluded-chat catalogue entries, including names and known activity timestamps, in the local selector. Do not retain previews/body text merely to sort or label that list.
Preserve catalogue updates before a content skip; otherwise newly appearing excluded chats could become impossible to discover later. [S7][S9]
Full catalogue access belongs to the local app's administration interface, not an agent's MCP discovery tools.

## Independent MCP enforcement

Every tool authenticates the agent, loads the current shared scope revision, resolves target identities, and checks authorization before calling wacli.
Never expose a raw command tool, arbitrary `--store`, shell, SQLite query, file path, or delegate socket through MCP.
For chat-specific reads, call `--read-only --json` with an explicit authorized chat, then validate every returned row/context item before constructing the response. [S12][S13]
For cross-chat search, begin with bounded per-authorized-chat CLI queries and merge their results in the wrapper; no unfiltered global result may be returned accidentally.
If scaling requires shared SQL later, use parameterized read-only queries containing the allowed identity set before limits/counts, with the schema pinned to the bundled release. [S13]
The selector's complete chat/contact catalogue must not leak through MCP names, suggestions, ambiguous-recipient errors, counts, snippets, history coverage, or health output.
Sanitize upstream errors and construct explicit response objects; do not forward arbitrary stdout/stderr or every JSON field.
Do not forward upstream pagination/change tokens directly as authorization: bind wrapper cursors to agent, store, query, and scope revision, and reject them after scope changes. [S12]
If exposing changes, filter chat IDs and both sides of identity moves; consume hidden entries internally so they neither leak nor stall progress. Preserve explicit expired-checkpoint errors. [S12]
Keep checkpoints per agent. Deriving a summary or creating a schedule remains the agent's job; the app supplies scoped evidence.
Recheck scope before emitting a completed result, so removing a chat also blocks requests that began under the previous revision.

## Attachments and sending

Identify downloads by authorized `(chat_jid, message_id)`, not by user-supplied paths or upstream attachment URLs.
Authorization must precede `downloadExactMedia`: its cache path can return bytes before any network/session lock. A cached old attachment is still protected by current scope. [S11]
Choose output paths in an app-owned private staging/cache directory; publish an authenticated, scoped attachment handle only after checking the latest policy revision.
Recheck authorization on every attachment fetch, including cloud/tunnel requests. Do not expose the media directory as a public static server.
Cancel removed-chat downloads during transitions; discard unpublished staging output. Already delivered bytes cannot be recalled from an agent.
Exact recovery uses `RetryMediaMessage`; keep bulk media retry/backfill commands outside the on-demand attachment tool so they cannot sweep old excluded records. [S11]
Text/file sends require both the per-agent send capability and an authorized resolved chat; revalidate the destination at dispatch, including after any verified identity resolution.
During scope transitions, stop admitting sends and wait for dispatched operations before acknowledging completion; a send already accepted by WhatsApp cannot be undone by revoking scope.
This preserves the v1 send-only mutation surface and does not introduce editing, deletion, or group administration.

## Acceptance checks — specified, not executed

- Initial capture: fixtures from several chats persist before selection; no premature allowlist drops incoming history.
- Cutoff: while history is arriving, confirm one chat; after acknowledgment, only its content persists, including delayed older history and reconnect replay.
- Retained history: an excluded chat's old rows remain on disk but cannot be listed, searched, read by guessed ID, exported, or reached through context/attachments.
- Catalogue: excluded/new chats remain selectable by name/phone and recency, without storing message previews or new excluded body text.
- Side effects: excluded edits, polls, reactions, locations, call paths, media jobs, and webhook delivery do not bypass retention; intentional skips do not report storage failures.
- Identity: selected LID later maps to phone JID without losing access; equivalent requests cannot bypass removal; same-named chats remain separate.
- Restart/crash: interrupt every transition step; restart never falls back to initial/all capture, and the applied policy revision matches the UI and MCP.
- Concurrent agents: independent cursors remain independent; scope removal invalidates in-flight responses and cached handles for every agent.
- Attachment cache: an old excluded cached file, direct download, phone retry, and guessed path all fail before bytes are published; selected downloads still work beside sync.
- Sending: denied agents and excluded destinations never reach dispatch; allowed text/file sends retain normal wacli behavior, including failures and uncertain acknowledgments.
- Pagination: cross-chat limits cover authorized chats; scope changes invalidate cursors; hidden change-feed rows cannot leak identifiers or prevent forward progress.
- Regression: run upstream focused sync/history/media/identity tests plus these policy cases; inspect SQLite/FTS and side tables, not only returned MCP text.

## Primary sources at the pinned commit

[S1]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/docs/auth.md
[S2]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/cmd/wacli/auth.go#L82
[S3]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/cmd/wacli/root.go#L120
[S4]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/sync_events.go#L608
[S5]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/sync_limits.go#L27
[S6]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/backfill.go#L49
[S7]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/sync.go#L398
[S8]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/sync_polls.go#L20
[S9]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/group_info.go#L111
[S10]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/jid.go#L9
[S11]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/cmd/wacli/media_download.go#L37
[S12]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/docs/messages.md
[S13]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/docs/integrations.md
[S14]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/bootstrap.go#L11
[S15]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/internal/app/lid_migration.go#L12
[S16]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/cmd/wacli/messages_helpers.go#L18
[S17]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/cmd/wacli/messages_read.go#L14
[S18]: https://github.com/openclaw/wacli/blob/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b/docs/contacts.md

# Remove custom synchronization tracking

- ID: `LW-108`
- Status: closed
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: none
- Supersedes: [Keep isolated message gaps separate from sync failure](107-sync-health.md)
- Resolution: [Removal and verification completed](../comments/108-upstream-sync.md#completed--2026-10-29)

## Authorized change

On October 29, 2026, the user explicitly requested removal of the custom completion tracking, recovery journal, replay/repair machinery and synced-state presentation, relying on upstream synchronization instead. This supersedes both the strict sync-state design and the subsequent isolated-message-gap revision. The [product revision](../../wayfinder/comments/001-product-contract.md#upstream-owned-sync--2026-10-29) records the decision; the [current connection contract](../interfaces.md#whatsapp-connection-status) defines the resulting app interface.

Implement and test the removal on the current branch, preserving other uncommitted work. Do not commit, push, reconnect a personal WhatsApp account, reset personal data or claim the historical 401 is fixed. Simulated accounts and disposable stores are the verification surface.

## Required result

Use the exact pinned, unmodified WhatsApp SDK. Remove the local SDK delivery patch, its build-time module replacement and completion-specific fixtures. Remove app-owned delivery barriers, pending-work counters, sync snapshots/watchdog, custom failure classification, the recovery journal and its replay/repair extension. The removal applies to the custom layer: preserve the pinned engine's normal message/session persistence, history download, acknowledgements, decryption, retries, app-state/contact recovery and private protocol state.

Private app state and MCP status retain connection status, linked state, QR data, actual connection errors and intentional-disconnect state. Remove app `sync_state`, `last_synced_at` and MCP `source.last_sync_at`. The native header describes the connection and selected chat count, without a caught-up/incomplete classification, synced timestamp, recovery notice or activity inferred from custom bookkeeping.

Retain chat-scope enforcement, policy acknowledgement before access, verified identity aliases, per-agent authorization and sending permission, single-writer/process ownership, safe explicit relinking after confirmed revocation, terminal logout handling and native stale-response guards. Connection-only diagnostic history remains bounded and private; it cannot infer completion or schedule recovery.

Legacy custom journal/baseline files are ignored and left untouched. They cannot block startup or produce an error, and this task neither deletes them nor modifies the user's archive, session, credentials or scope. Upstream-owned recovery databases and markers are not custom leftovers and remain operational.

## Acceptance

| Scenario | Required behavior |
| --- | --- |
| Clean pinned-source preparation | Uses exact upstream SDK without the local delivery patch/replacement; remaining engine patch applies cleanly. |
| Fresh synthetic QR/pairing/login and ordinary message processing | Connection status follows upstream lifecycle; selected messages continue through upstream persistence. No completion tracker, sync snapshot or recovery ledger is started. |
| Existing arbitrary/failed/corrupt legacy custom journal | Connection and message processing do not read it, rewrite it, delete it, schedule repair from it or derive status from it. |
| An undecryptable update or history warning occurs | Upstream handling remains intact; no custom saving-error banner, failure ledger or claimed completion state appears. |
| A connection logs out or its owned process exits | Existing truthful connection failure and terminal behavior remain; an old process cannot revive the new connection. |
| Wake, reconnect, delayed startup and native stale responses | Existing action ownership and authentication checks hold; no invented completion timer or state is substituted. |
| Scope changes overlap/fail, agents are revoked, or sending is disabled | Existing fail-closed policy and authorization behavior remains. |
| Private state, MCP status, native model and preview | Agree on connection-only behavior; removed sync/freshness fields and UI actions are absent. |
| Existing upstream internal retrieval/recovery state | Remains owned by upstream; application neither deletes it nor turns it into a completion claim. |

## Verification and closure

Capture the pre-edit working tree under `build/remove-sync-tracking/` and review this task against that snapshot plus interactions with earlier uncommitted fixes. Use focused tests through the engine's actual lifecycle and data interfaces, the private/MCP surface and native actions. Verify the removal with an old-journal noninterference fixture and pinned SDK/source checks, rather than testing a replacement completion model.

Run the required repository checks and independent Standards and Spec reviews. Record results and actual limits in the completion comment before closing. Synthetic success cannot explain the historical server-side 401 or establish live WhatsApp compatibility.

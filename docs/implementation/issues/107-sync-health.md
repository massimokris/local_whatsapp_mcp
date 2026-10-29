# Keep isolated message gaps separate from sync failure

- ID: `LW-107`
- Status: closed
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: none
- Resolution: [Implementation, review, and isolated verification](../comments/107-sync-health.md)
- Superseded by: [Remove custom synchronization tracking](108-upstream-sync.md)

This closed ticket is a historical implementation record. The user's later [upstream-sync decision](../../wayfinder/comments/001-product-contract.md#upstream-owned-sync--2026-10-29) supersedes its runtime design; [Remove custom synchronization tracking](108-upstream-sync.md) owns the replacement. The requirements and results below are preserved as written for this completed work.

## Authorized change

On October 29, 2026, the user requested that isolated message failures stop placing the entire app in an error state. Implementation, independent review, and isolated testing are authorized; the user will decide the next step afterward. Personal WhatsApp reconnection, installation, release, and data resets are outside this task.

The [sync-health revision](../../wayfinder/comments/001-product-contract.md#isolated-message-failures--2026-10-29) supersedes the earlier rule that every unresolved message failure must appear as a global sync error. The [current sync contract](../interfaces.md#whatsapp-sync-state) defines the wire and presentation behavior.

## Result

Add `incomplete` to the existing sync-state vocabulary. It means the current connected session has reached a delivery boundary, active work has drained, and only known message gaps remain. Keep `syncing` for active work, `caught_up` for verified completion without known gaps, `unknown` for invalidated completion evidence, and `error` for blocking failures.

The native app presents `incomplete` as connected, with the existing chat count and quiet context that some messages are unavailable. It does not show an error banner, a recovery action, a complete-sync claim, or ongoing-sync activity for drained work. Setup, chat scope, agent pairing, and sending rules retain their existing behavior.

Gap classification belongs at the failure cause. Only proven content failures in message decryption, history-blob retrieval, and encrypted edit/reaction/poll parsing, decryption, or validation persist `failure_class: "content"` as recoverable gaps. A decryption method can also fail because its message-secret or session store failed; those failures, authentication failures, archive/journal writes, receipt persistence, receive decoding, startup/app-state failures, panics, and unknown causes remain blocking. New blocking failures persist `failure_class: "blocking"`; that explicit classification is distinct from a missing field and dominates content classification for the same work. The SDK preserves the underlying cause in memory through a new `DecryptError` field excluded from event JSON and the recovery journal. The field adds no log output; existing SDK and application warnings are unchanged.

Older failed records without a reliable failure class remain blocking until matching successful recovery, even when they contain undecryptable-message metadata. Earlier code could replace that metadata after a storage failure, so it cannot prove the original cause. An existing installation may therefore retain a prior warning after this update. This limitation does not authorize deleting or resetting any saved data.

A saved `Message`, `UndecryptableMessage`, or `HistorySync` record with `failed: false` is different: if no worker currently owns it, it can represent an inactive message gap. Claiming it for actual replay makes work active and therefore `syncing`; only after active work drains and a fresh current delivery boundary is established may remaining gaps yield `incomplete`. Opaque or app-state pending work retains its existing conservative treatment.

The durable journal remains authoritative for unresolved work. Matching successful recovery clears only the work it covers. Reconnects, unrelated successful messages, new snapshots, or a change in presentation cannot erase gaps or manufacture recovery. This task adds no automatic repair mode, data discard, new retry policy, timers, thresholds, or background connection attempt.

## Acceptance

| Scenario | Required observable behavior |
| --- | --- |
| Fresh synthetic pairing/login, then a selected text message with a proven content-decryption failure while other messages save | Work in progress stays `syncing`; the isolated failure does not produce the global saving-error banner. The gap remains durable. |
| Current delivery boundary completes with only isolated gaps left | `incomplete`, connected presentation, quiet missing-message context; no error banner, retry action, complete-sync label, or perpetual syncing indicator. |
| Current delivery boundary completes without gaps | Existing `caught_up` and “Synced just now” behavior remains. |
| More work arrives while incomplete | `syncing` during that work, then `incomplete` if gaps remain. Unrelated successful work does not clear them. |
| A readable replay of the affected message saves successfully | Only matching debt clears; a fresh valid completion can become `caught_up`. |
| Matching recovery encounters a real archive or journal write failure | `error` remains actionable. A message-gap classification cannot hide failed persistence. |
| A real SDK decryption callback encounters a message-secret or session-store I/O error | The underlying cause survives wrapping and produces a blocking failure; the same callback with a proven content failure may produce a gap. The new `DecryptError` field does not enter event JSON or the recovery journal and adds no log output; existing warnings are unchanged. |
| An older failed record lacks an explicit classification, including one with undecryptable-message metadata | It remains blocking until matching successful recovery. The update neither guesses the old cause nor deletes data to remove the warning. |
| Saved typed message/history work is pending but has not failed and has no active owner | It remains durable as an inactive gap. Actual replay is `syncing`; drained work can become `incomplete` only after a fresh current delivery boundary. Opaque or app-state pending work is not quietly downgraded. |
| Restart, reconnect, wake, or scope change with saved gaps | Existing invalidation applies. A matching connected session and a fresh current delivery boundary are required before `incomplete`; saved debt alone cannot prove current health. |
| A blocking failure recovers while message gaps remain | Only authoritative current engine evidence can replace the error with `syncing` or `incomplete`; a copied description or ordinary lifecycle callback cannot forgive the failure. |
| Stale process, scope, or connection events arrive | They cannot restore completion, clear failure, or change current gap state. |
| Real logout, revocation, or failed connection health follows incomplete | Existing connection/authentication error takes priority; terminal logout cannot be revived by late progress. |
| An older engine emits an unknown state or unclassified failure | Conservative existing behavior remains; it cannot authorize a success label or quietly forgive an error. |
| Explicit expected view-once absence or an out-of-scope event occurs | Existing exclusions remain; no new gap or failure is invented. |

## Verification and completion

Use real `App.Sync` with disposable transport/archive fixtures for the fresh-link sequence; actual SDK callbacks backed by failing message-secret and session stores for cause classification; engine child-event streams for supervision; private state and authenticated MCP status for agreement; and native `AppModel`/summary interfaces for presentation. Include restart/recovery, pending-work replay, old failed records, and storage-failure controls so the test does not merely assert a renamed enum. Preserve the existing receive-order and current-process/current-scope/current-connection checks.

Run focused red/green slices, then the complete repository checks and independent Standards and Spec reviews. Record results in a dated completion comment before closing this ticket. Synthetic results do not prove live WhatsApp compatibility or explain the separately observed server-side 401.

For review, compare this task against the saved pre-edit working-tree snapshot under `build/sync-simplification/`, and also inspect integration with the existing uncommitted changes. The repository HEAD alone includes earlier fixes and is not a task-only baseline. Keep all previous source changes and personal data intact; leave changes uncommitted for the user's next decision.

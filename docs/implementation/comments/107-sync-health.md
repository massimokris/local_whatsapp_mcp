# Isolated message gaps and sync health

- Date: 2026-10-29
- Ticket: [Keep isolated message gaps separate from sync failure](../issues/107-sync-health.md)
- Result: implemented, independently reviewed, and verified with isolated fixtures; closed

Implemented the [October 29 sync-health revision](../../wayfinder/comments/001-product-contract.md#isolated-message-failures--2026-10-29). Proven isolated content failures remain durable message gaps instead of placing the whole app in a global error state. Active work still shows syncing. A current connected session with a fresh completed delivery boundary and drained work can report `incomplete`, with a neutral connected header and quiet context in the expanded WhatsApp card. Verified completion without gaps retains the existing success label.

Real archive/journal writes, message-secret/session-store I/O, authentication failures, unknown causes, and terminal logout remain actionable. Only matching successful recovery clears the covered debt. Older failed records with absent or unknown classifications remain blocking because their metadata cannot prove the original cause; an existing installation may retain those warnings. No data was deleted or reset.

Review repairs preserve SDK error causes in the transient `DecryptError` field, keep ambiguous alias-store failures blocking, and let an authoritative current engine transition clear a recovered blocking error while other work remains active. Snapshots, lifecycle callbacks, stale events, and logout cannot authorize that recovery. The new field is excluded from event JSON and the recovery journal and adds no logging; existing SDK/application warnings are unchanged.

## Verification

The coherent final `./scripts/test.sh` run passed with exit code 0: **80 native tests**, backend race tests and vet, the full pinned-engine suite with and without FTS5 and vet, patched SDK race tests, and disposable packaging fixtures. The [final QA record](../../../build/sync-simplification/qa-final-verification.json) and [complete test output](../../../build/sync-simplification/full-tests.log) retain the evidence. All 136 source/document hashes stayed unchanged throughout that run; closure changes afterward are limited to this comment, the ticket metadata, and the plan's completion pointer.

Focused synthetic `App.Sync` and actual SDK regressions covered fresh pairing callbacks, missing/undecryptable content, exact recovery, failed persistence, message-secret/session/alias-store errors, pending replay, restart/reconnect, stale events, private/MCP status agreement, and terminal logout priority. The [implementation report](../../../build/sync-simplification/implementation-report.md) records the repairs and red/green checks. The [native report](../../../build/sync-simplification/native-results.md) records 29 focused native tests and visually checked expanded/collapsed in-memory demo states.

Independent Standards review finished with **0 remaining findings**; independent Spec review finished with **0 remaining findings** after the SDK, alias-lookup, and stale-error repairs. Both reviewers verified all 51 changed content-patch targets against the final manifest using the clean pinned checkout at `build/sync-simplification/fresh-final-apply/`. The [review packet](../../../build/sync-simplification/review-packet.md) identifies the task-only baseline, final source comparison, and review evidence. Earlier review findings and pre-fix snapshots remain preserved.

Final QA found no staged files, no source changes during testing, and a clean whitespace check. All 26 checked app/installer artifact hashes remained unchanged; the existing installed and distribution app versions remain 1.1.1 build 8. Prior uncommitted work was preserved, and this task remains uncommitted.

## Limits and handoff

Connection and recovery checks used synthetic accounts and disposable stores. No personal WhatsApp reconnection, profile reset, installation, release packaging, staging, commit, or push was performed. These results do not establish live WhatsApp compatibility or explain the separately observed server-side 401. No new installable app bundle was produced. The user decides the next step after implementation, review, and testing.

## Superseded by upstream-owned synchronization — 2026-10-29

After this implementation and its verification, the user explicitly chose to remove the custom completion/recovery layer entirely. [Remove custom synchronization tracking](../issues/108-upstream-sync.md) owns that later change; its [product decision](../../wayfinder/comments/001-product-contract.md#upstream-owned-sync--2026-10-29) supersedes this design. This preserves the earlier results as historical evidence and does not retroactively claim live compatibility or a known 401 cause.

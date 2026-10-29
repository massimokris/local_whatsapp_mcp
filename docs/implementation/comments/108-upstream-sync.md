# Upstream-owned synchronization

- Date: 2026-10-29
- Ticket: [Remove custom synchronization tracking](../issues/108-upstream-sync.md)
- Result: authorized and in progress; runtime verification pending

The user superseded the previously implemented isolated-message-gap design with a simpler ownership decision: the app observes connection state while the pinned engine and its unmodified WhatsApp SDK own synchronization. Removing the custom journal, replay/repair extension and completion indicators is an explicit product change, not a claim to have established the cause of the earlier 401.

The previous [sync-health completion record](107-sync-health.md) remains historical evidence, including its validation and limits. The current [connection contract](../interfaces.md#whatsapp-connection-status) describes the replacement. The [pre-edit documentation snapshot](../../../build/remove-sync-tracking/docs-baseline/) preserves the source being revised.

The removal audit distinguishes upstream app-state/contact recovery, receipt handling, session records and retries from the app's completion journal. The callback-lock workaround in the SDK delivery patch protects its added synchronous delivery-bookkeeping callbacks; removing those callbacks removes that integration-specific dependency. No independent SDK modification has been identified as necessary to keep once the custom completion layer is removed. App policy, process ownership, authentication and explicit-relink safeguards remain in scope.

No production/live result is asserted by this planning comment. Implementation, regression checks and independent review must be recorded below before closure.

## Completed — 2026-10-29

Result: implemented, independently reviewed and verified; ticket closed. The app now reports connection state while the pinned upstream engine and unmodified WhatsApp SDK own synchronization. The custom completion tracker, SDK delivery patch and module replacement, recovery journal/replay/repair extension, sync timestamps and native completion indicators have been removed. The retained engine patch preserves policy, authentication, process ownership and safe explicit relinking. Legacy custom journals are ignored without being read, rewritten or deleted; upstream protocol and session storage remain intact.

The [implementation record](../../../build/remove-sync-tracking/implementation-report.md), [removal audit](../../../build/remove-sync-tracking/removal-audit.md) and [final QA report](../../../build/remove-sync-tracking/final-qa-report.md) record the change and evidence:

- The full repository test script passed: 81 native tests, all four test-bearing backend packages with the race detector plus vet, all 14 test-bearing engine packages in both non-FTS5 and FTS5 configurations plus required vet, and packaging fixtures.
- A disposable build produced and verified an ad hoc signed arm64 app targeting macOS 13. Its newly compiled bundle passed 73 smoke checks using synthetic data, including connection-only private/MCP status, selected-chat filtering, archive access, pairing, permissions and retained data after disconnect.
- The actual bundle uses the exact pinned upstream WhatsApp SDK with its original module checksum and no replacement. All 28 generated engine patch targets match the reviewed source byte for byte.
- Independent final reviews: Standards — zero findings; Spec — zero findings. Both reviewed the completed removal against the approved contract and preserved safeguards.
- During QA, all 142 source paths, Git status and the complete diff remained unchanged. The 27 files in the existing distribution and installed app retained their hashes, sizes and modification times. This closure changes only this comment, the ticket status/resolution link and the plan's follow-up entry after that checkpoint.

The initial disposable smoke attempt failed before startup because its deeply nested Unix socket path was 133 bytes. The same binary and unchanged checks passed from a shorter temporary root; the [QA report](../../../build/remove-sync-tracking/final-qa-report.md) retains both the failure and the diagnostic evidence.

No personal WhatsApp connection, profile access or reset, installation, root installer overwrite, release packaging, staging, commit or push was performed. These synthetic checks validate the requested removal and retained behavior; they do not establish live WhatsApp compatibility or prove the cause or resolution of the historical server-side 401 revocation.

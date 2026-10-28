# LocalWhatsAppMCP implementation

- Status: original implementation complete; local-only revision on 2026-10-23, ngrok request-host allowance on 2026-10-27
- Integration branch: `codex/localwhatsappmcp-integration`
- Tracker operations: [local Markdown tracker](../agents/issue-tracker.md)

The user authorized end-to-end implementation, local commits, isolated implementation branches, verification, review, and installable packaging on 2026-10-23. This execution graph is separate from the earlier decision map. Resolve each task with dated evidence in `comments/`; close only after its deliverables are integrated and verified. Runtime checks requiring the user's WhatsApp QR scan or provider login need separate live verification.

The graph and completion records below describe the original implementation. The subsequent [local-only revision](../wayfinder/comments/001-product-contract.md#local-only-revision--2026-10-23) removes app-owned cloud connection; its task remains closed as historical evidence. The [ngrok host allowance](../wayfinder/comments/001-product-contract.md#ngrok-host-allowance--2026-10-27) later permits requests through user-managed tunnels without restoring provider setup or OAuth. Current inter-component contracts are in [interfaces](interfaces.md).

## Original task graph

| Task | Dependencies | Deliverable |
| --- | --- | --- |
| [Scoped WhatsApp engine](issues/101-engine.md) | none | Pinned source patch, reproducible build, policy fixtures |
| [Service and local agent access](issues/102-service.md) | none | Go service, policy, MCP, private control, pairing/helper |
| [Native menu bar app](issues/103-native.md) | none | SwiftUI/AppKit onboarding, management, lifecycle |
| [Cloud connection](issues/104-cloud.md) | service | OAuth, tunnel supervision, private provider setup |
| [Distribution tooling](issues/106-packaging.md) | native | Reproducible app/DMG build and bundle checks |
| [Distribution and acceptance](issues/105-distribution.md) | all preceding | Integrated tests, review, app/DMG, installation documentation |

## Original agreed test boundaries

The unattended implementation request authorized these behavior seams: pinned wacli ingestion/policy including side effects; the authenticated MCP/attachment surface; private app control and helper installation; OAuth/browser request binding; process supervision and restart; native chat selection and application lifecycle. Use the TDD skill with sequential red/green slices and deterministic fixtures. No real account messages, credentials, or media enter fixtures.

## Integration contracts

See [interfaces.md](interfaces.md) before implementing inter-component calls. Implementers own separate Git worktrees, branch from the integration tip, and merge its current tip before reporting completion. A merger integrates finished branches. The final review applies the code-review skill to the full branch.

## Completion

All six execution tasks are closed. The [distribution resolution](comments/105-distribution.md#completed--2026-10-23) records the final source snapshot, installer checksums, passing checks, review repairs, worktree cleanup, and limits of account-bound verification.

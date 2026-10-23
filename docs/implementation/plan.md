# LocalWhatsAppMCP implementation

- Status: in progress
- Integration branch: `codex/localwhatsappmcp-integration`
- Contract: [SPEC.md](../../SPEC.md)
- Tracker operations: [local Markdown tracker](../agents/issue-tracker.md)

The user authorized end-to-end implementation, local commits, isolated implementation branches, verification, review, and installable packaging on 2026-10-23. Product decisions remain as recorded in the specification and referenced conversations. This execution graph is separate from the earlier decision map. Resolve each task with dated evidence in `comments/`; close only after its deliverables are integrated and verified. Runtime checks requiring the user's WhatsApp QR scan or provider login remain explicitly identified as installation acceptance checks.

## Task graph

| Task | Dependencies | Deliverable |
| --- | --- | --- |
| [Scoped WhatsApp engine](issues/101-engine.md) | none | Pinned source patch, reproducible build, policy fixtures |
| [Service and local agent access](issues/102-service.md) | none | Go service, policy, MCP, private control, pairing/helper |
| [Native menu bar app](issues/103-native.md) | none | SwiftUI/AppKit onboarding, management, lifecycle |
| [Cloud connection](issues/104-cloud.md) | service | OAuth, tunnel supervision, private provider setup |
| [Distribution and acceptance](issues/105-distribution.md) | all preceding | Integrated tests, review, app/DMG, installation documentation |

## Agreed test boundaries

The specification and this unattended implementation request authorize these behavior seams: pinned wacli ingestion/policy including side effects; the authenticated MCP/attachment surface; private app control and helper installation; OAuth/browser request binding; process supervision and restart; native chat selection and application lifecycle. Use the TDD skill with sequential red/green slices and deterministic fixtures. No real account messages, credentials, or media enter fixtures.

## Integration contracts

See [interfaces.md](interfaces.md) before implementing inter-component calls. Implementers own separate managed worktrees, branch from the integration tip, and merge its current tip before reporting completion. A merger integrates finished branches. The final review applies the code-review skill to the full branch.

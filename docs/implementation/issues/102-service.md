# Service and local agent access

- ID: `LW-102`
- Status: closed
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: none
- Resolution: [Implementation and verification](../comments/102-service.md)

Implement Go backend, private control, safe persistence/scope transitions, process coordination, required MCP capabilities, scoped resources, request-safe sending, single-use pairing, Keychain setup helper, local Codex/Claude and stdio adapters. Follow [interfaces](../interfaces.md) and [pairing research](../../research/agent-pairing.md). Verify public security and restart seams with deterministic fixtures.

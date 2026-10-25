# Cloud connection

- ID: `LW-104`
- Status: closed
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: [Service and local agent access](102-service.md)
- Resolution: [Implementation and verification; subsequently superseded](../comments/104-cloud.md#superseded--2026-10-23)

Implement app-owned ngrok/cloudflared configuration, supervision and recovery, private credential import, OAuth discovery/PKCE/client registration and exact browser request binding. Preserve the accepted agent-assisted setup and account-free LocalWhatsAppMCP experience. Verify protocol behavior with fixtures, and explicitly record provider/client checks requiring real login.

The task above records the original completed work. Cloud connection was removed from the current product by the [local-only revision](../../wayfinder/comments/001-product-contract.md#local-only-revision--2026-10-23).

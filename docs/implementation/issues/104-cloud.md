# Cloud connection

- ID: `LW-104`
- Status: open
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: [Service and local agent access](102-service.md)

Implement app-owned ngrok/cloudflared configuration, supervision and recovery, private credential import, OAuth discovery/PKCE/client registration and exact browser request binding. Preserve the accepted agent-assisted setup and account-free LocalWhatsAppMCP experience. Verify protocol behavior with fixtures, and explicitly record provider/client checks requiring real login.

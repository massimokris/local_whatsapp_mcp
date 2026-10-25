# Preserve one-time pairing across MCP client connections

- ID: `LW-003`
- Status: closed
- Labels: `wayfinder:research`
- Mode: AFK
- Parent: [LocalWhatsAppMCP v1 decision map](../map.md)
- Assignee: massi
- Blocked by: [Define the v1 product contract](001-product-contract.md)
- Research branch: `research/local-whatsapp-mcp-pairing`
- Research asset: [Agent-pairing integration](../../research/agent-pairing.md)
- Latest comment: [MCP connection UI simplification](../comments/003-agent-pairing.md#mcp-connection-ui-simplification--2026-10-24)

## Question

How should the Go server and setup helper preserve the agreed experience—paste setup instructions and a single-use token, let the local agent install its own credential, immediately grant reading access—using actual MCP client configuration/authentication capabilities? Describe local clients and the optional tunnel/cloud path, including standard browser authorization only where the client requires it. Keep long-lived credentials out of conversation output, preserve separate revocable agent identities, and distinguish documented support from end-to-end verification. Do not require the user to configure OAuth, headers, or an identity provider.

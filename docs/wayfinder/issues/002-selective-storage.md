# Enforce selected chats across ingestion and MCP access

- ID: `LW-002`
- Status: closed
- Labels: `wayfinder:research`
- Mode: AFK
- Parent: [LocalWhatsAppMCP v1 decision map](../map.md)
- Assignee: massi
- Worker: `/root/wacli_facts`
- Blocked by: [Define the v1 product contract](001-product-contract.md)
- Research branch: `research/local-whatsapp-mcp-storage`
- Research asset: [Selective-storage integration](../../research/selective-storage.md)
- Resolution: [Ingestion and access boundary](../comments/002-selective-storage.md)

## Question

Where should a narrow patch to pinned wacli enforce the initial-capture-to-selected-scope transition, and how should the Go MCP wrapper enforce that same scope on existing records and attachments? Locate the relevant upstream ingestion/read surfaces, identity handling, and process interfaces. Recommend an integration boundary and identify meaningful acceptance tests without implementing the app or changing the agreed capture behavior.

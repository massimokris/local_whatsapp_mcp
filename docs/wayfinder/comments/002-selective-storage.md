# Resolution: Enforce selected chats across ingestion and MCP access

- Date: 2026-10-23
- Ticket: [Enforce selected chats across ingestion and MCP access](../issues/002-selective-storage.md)
- Research worker: `/root/wacli_facts`
- Result: closed; integration route identified, runtime acceptance checks not executed
- Evidence: [Selective-storage integration](../../research/selective-storage.md)

Use an explicit initial-capture/all-chats/selected-chats policy in patched wacli's app layer, shared by auth, live sync, backfill, and delegated media operations. Preserve catalog and verified identity updates, then apply the content predicate before payload persistence. Propagate retained/skipped/error outcomes so excluded content cannot survive in side records, webhooks, or media work after the main write is skipped.

The Go supervisor owns the policy and serializes its application. The recommended v1 cutoff stops old writers, publishes a policy revision, and restarts the existing session with a machine-readable acknowledgment; it does not require WhatsApp linking again. MCP access closes for removed chats immediately and must not reopen under an outdated policy.

Independently authorize every MCP result, search, update feed, and attachment against the current scope, including data saved during initial capture. Group membership does not authorize a participant's direct chat. Cached media must pass authorization before returning bytes.

This answers the technical question without changing the user's product decisions. The research asset contains pinned source links and the specific unexecuted tests needed during implementation.

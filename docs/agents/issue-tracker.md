# Issue tracker: Local Markdown

This project keeps its decision and implementation tracker inside `local_whatsapp_mcp`. These are local project artifacts; writing them publishes to this tracker only. No external issue service is required.

- Canonical map issue: [LocalWhatsAppMCP v1 decision map](../wayfinder/map.md).
- One decision issue per file in `docs/wayfinder/issues/`.
- Resolution comments in `docs/wayfinder/comments/`; research assets in `docs/research/`.

## Wayfinding operations

### Identity and metadata

Every issue has a stable ID, a human-readable title, Status (`open` or `closed`), Labels, Parent, Assignee, and Blocked by fields. Decision labels are `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, or `wayfinder:task`; the map uses `wayfinder:map`. Mode is AFK or HITL.

Refer to issues by linked title in prose. IDs identify records; they are not substitutes for readable names. Parent links make decision tickets children of the map.

### Find the frontier

Read the map once, then inspect child metadata without loading every question. From the project root:

```sh
rg -n '^# |^- (ID|Status|Labels|Parent|Assignee|Blocked by):' docs/wayfinder/issues
```

A frontier ticket is **open**, has **Assignee: unassigned**, and has every blocker closed. Evaluate the linked blocker records; do not infer readiness from numbering. Take frontier tickets in numeric filename order unless the user names one. An open assigned ticket is claimed, even though Status remains open.

### Claim

Before research or discussion, change Assignee from `unassigned` to the developer driving the effort (`massi` here); optionally record the worker agent. Re-read the file before saving to detect another session's claim. If already assigned, coordinate rather than duplicate the work.

### Dependencies

Local Markdown has no native dependency graph. `Blocked by` is the authoritative fallback, using links to ticket titles or `none`. Create all new issues first, then wire dependencies in a second pass. Keep the graph acyclic. Do not list open children in the map body; query them from metadata.

### Resolve

1. Work the ticket using its type's skill and the map's notes. HITL decisions require the user's real participation; do not simulate their approval.
2. Append a dated resolution comment in the corresponding comments file, with the result, evidence, remaining limits, and asset links. Keep the question in the issue body intact.
3. Link the comment from the issue and set Status to `closed` only when that question is answered. Research may answer an implementation question without claiming that runtime tests have passed.
4. Add one linked-title gist to the map's Decisions so far. The comment holds the reasoning; the map remains an index.
5. Create any newly precise questions, then wire blockers. Keep only genuinely unspecifiable in-scope uncertainty in Not yet specified.

Recording the conversation's existing agreement is historical capture, not a new simulated interview. In a normal work-through session, resolve at most one non-research decision ticket. Charting may dispatch and resolve multiple research tickets.

### Research context

Use an isolated `research/<name>` branch/worktree for external investigation. Record its name in the issue. Findings are delivered into the project's `docs/research/` directory and linked from the resolution comment. Preserve the delivered artifact before archiving the temporary worktree; the local note is the durable context pointer. Runtime checks not executed must remain explicit.

### Scope and completion

Wayfinder tickets resolve decisions; application implementation tasks belong to a later execution plan. If an issue lies outside this map's destination, close it with a scope explanation and link it under Out of scope rather than Decisions so far.

Close the map when its questions are answered and the recorded decisions provide a clear implementation route. A closed map means planning is complete, not that the app exists or has passed acceptance checks. If facts later invalidate a decision, preserve the prior comment and record a superseding decision.

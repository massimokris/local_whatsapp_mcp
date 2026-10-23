# Component interfaces

These are internal engineering contracts for concurrent implementation. Product behavior remains in [SPEC.md](../../SPEC.md).

## Layout and binaries

- `backend/`: Go module `github.com/massi/localwhatsappmcp`; main `cmd/localwhatsappmcp` builds `localwhatsappmcp` (service and setup helper).
- `native/`: Swift package producing executable `LocalWhatsAppMCP`.
- `vendor/wacli/`: upstream pin metadata, focused patch, and build integration; downloaded checkout is build output, not source-controlled wholesale.
- App bundle: `Contents/MacOS/LocalWhatsAppMCP`, `Contents/Helpers/localwhatsappmcp`, `Contents/Helpers/wacli`.
- Default runtime directory: `~/Library/Application Support/LocalWhatsAppMCP/` (0700). App control uses `control.sock` (0600), never the tunnel listener.

## Helper/private control boundary

Native app launches `localwhatsappmcp serve --owner-stdin --data-dir DIR --wacli PATH --helper PATH`. The app keeps stdin open as an ownership pipe; EOF cancels the service and awaits owned children, including after an app crash. SIGTERM also waits for owned child shutdown. Service stderr emits only a bounded, allowlisted `startup_error` code for startup failure; native UI maps it to recovery copy. It never displays arbitrary process logs. Helper calls use `localwhatsappmcp control --data-dir DIR --method METHOD --path PATH`; JSON request body comes from stdin; one JSON response goes to stdout; nonzero status indicates failure. Blocking process work stays off the main actor.

Private endpoints:

| Method/path | Request | Response |
| --- | --- | --- |
| `GET /state` | none | state described below |
| `POST /whatsapp/connect` | `{}` | state |
| `POST /lifecycle/wake` | `{}` | state |
| `GET /catalog` | none | `{"chats":[...]}` complete minimal local catalog |
| `PUT /scope` | `{"mode":"all" or "selected","chat_ids":[...]}` | state after policy transition |
| `POST /pairing` | `{"name":"Codex","client":"codex"}` | `{"instructions":"...","expires_at":"RFC3339"}` |
| `PATCH /agents/ID` | `{"send_enabled":true}` | state |
| `DELETE /agents/ID` | none | state |
| `POST /cloud/instructions` | `{"provider":"ngrok" or "cloudflared","name":"Cloud agent"}` | `{"instructions":"..."}` |
| `DELETE /cloud` | none | state |

State is JSON with `whatsapp` (`status`, `detail`, optional `qr`, `last_event_at`), `scope` (`mode`: `initial`, `all`, `selected`; `chat_ids`; `revision`), `agents` (array of `id`, `name`, `client`, `send_enabled`, `connected`, optional `last_seen_at`), `mcp` (`url`, `status`), `cloud` (`enabled`, `provider`, `hostname`, `status`, `detail`), and optional `error`. Chat rows contain `id`, verified `aliases` (optional array including the current and previous identities), `name`, `phone`, `kind` (`direct`/`group`), `last_activity` RFC3339. Errors use `{"error":"readable safe message"}`. Native decoder should tolerate missing optional fields.

Native UI keeps selection/search locally, filters the complete minimal catalog, and sends the selected stable IDs. It canonicalizes each verified alias group to a current catalog row, preferring a phone-number JID when present; removing that row removes all its aliases. Display names and formatted phone strings never prove identity. Private endpoints do not require bearer credentials because the user-owned socket is the access boundary; none is mounted on the HTTP MCP listener.

## Engine policy boundary

Patched wacli accepts global `--content-policy PATH`. A required, versioned policy JSON has `version:1`, `revision:integer`, `mode:"initial"|"all"|"selected"`, `chat_ids:[stable JIDs]`. Absence/corruption when flag is supplied fails closed. It emits `policy_loaded` with revision through NDJSON lifecycle events. Supervisor stops and awaits old writer before atomic policy replacement/restart and awaits acknowledgement before access resumes. Every app-owned wacli command supplies the same fixed store and policy path. Engine ownership remains upstream.

## Cloud extension

Cloud code lives in `backend/internal/cloud` and integrates after the service seam exists. It must expose only MCP, authenticated attachments, and required OAuth endpoints. Setup/provider credentials and grant changes use private control. Tunnel providers run in a separate helper worker: the service sends a bounded command on a private stdin pipe and keeps it open. The worker terminates the provider process group on EOF or signal, so abrupt service death also removes its public tunnel. Credentials travel only through that private pipe and provider environment, never arguments, logs, or files. Local bearer grants and OAuth grants both revalidate current scope and sending permission.

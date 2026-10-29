# Bundled WhatsApp engine

Upstream is [openclaw/wacli](https://github.com/openclaw/wacli), pinned at
[`5578d1b`](https://github.com/openclaw/wacli/tree/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b).
`UPSTREAM_REVISION` is authoritative. The upstream MIT notice is in
[LICENSE.upstream](LICENSE.upstream) and must accompany distributed binaries.

`content-policy.patch` adds LocalWhatsAppMCP's immutable process policy at the
upstream app boundary and adds explicit sync completion tracking. It does not
replace WhatsApp connection/session ownership,
SQLite migrations, message parsing, media recovery, or the command protocol.
See [selective storage research](../../docs/research/selective-storage.md).

`whatsmeow-delivery.patch` applies to the exact transitive dependency
`go.mau.fi/whatsmeow v0.0.0-20261017175727-b2b1e71d45e2`. It adds a per-socket
receive/handler barrier and holds for asynchronous app-state continuations. It
preserves upstream parsing, reconnection, acknowledgements, and store ownership.
The copied dependency retains its MPL-2.0 source and license notices.

## Build and verification

The pin declares Go 1.27.0 and toolchain 1.27.3. A C compiler and CGO are required;
FTS5 is built with `-tags sqlite_fts5`. Build on the target OS/architecture.
The macOS build explicitly targets macOS 13.0, matching the Go 1.27 linker
minimum; `MACOSX_DEPLOYMENT_TARGET` can raise that target. The installed build
was verified on Apple silicon; Intel cross-distribution has not been verified.

```sh
./vendor/wacli/build.sh /absolute/output/wacli
./vendor/wacli/test.sh
```

The scripts fetch the exact commit over HTTPS, verify its Git object, apply the
patch with `git apply --check`, and build from that source.
`prepare-whatsmeow.sh` verifies the exact Go module version, uses Go's verified
module download, copies it into the disposable checkout's `.local-whatsmeow/`,
checks/applies its delivery patch, and installs a local Go module replacement.
The shared Go module cache is never edited. Generated source and
executables live in ignored `.build/`. `prepare.sh` intentionally resets only
its disposable `.build/pinned-source` checkout; make patch changes elsewhere.
Do not place WhatsApp account data in a source/build checkout.

`build.sh` prints the executable path. `test.sh` runs the complete upstream Go
suite both with and without FTS5, then vet and whitespace checks. It also runs
the dependency's deterministic delivery-barrier tests with the Go race detector.
Policy and completion fixtures
use synthetic accounts and disposable stores; no login or network WhatsApp data
is required. Live QR/linking and actual phone media recovery require the user's
installation acceptance test.

## Integration contract

Every app-owned invocation supplies `--store DIR --content-policy PATH`.
The service sets `WACLI_DEVICE_LABEL=LocalWhatsAppMCP` for each child process,
overriding any inherited label so new links identify the app on the phone's
Linked devices screen. This uses the pinned engine's existing environment option;
no upstream patch is needed. Existing linked entries may require relinking to
display a changed label.
Long-running `auth`/`sync` also supply `--owner-stdin` with a pipe held open by
the service. Pipe EOF/read failure triggers graceful cancellation and an
`owner_lost` lifecycle event, so a crashed service cannot leave an unrestricted
old writer. Recovery awaits an exclusive nonblocking `flock` on `STORE/LOCK`;
the lock file's existence or recorded PID is not evidence of current ownership.
The required JSON snapshot has `version: 1`, `revision >= 1`, `mode` of `initial`,
`all`, or `selected`, and `chat_ids` containing stable PN/LID/group JIDs. Empty
selected means none. A missing, invalid, or unsupported snapshot prevents opening
the archive. Omitting the flag preserves upstream standalone behavior; the
companion must never omit it.

With `--events`, successful app initialization emits to stderr:

```json
{"event":"policy_loaded","data":{"revision":17,"mode":"selected"},"ts":1792713600000}
```

The policy stays fixed until exit. The supervisor must stop and await all old
writers before publishing a replacement and must await this exact revision's
acknowledgement before reopening access. Loading the file is not evidence of
WhatsApp connectivity or complete history.

### Explicit sync completion

The supervisor supplies a fresh `WACLI_SYNC_RUN_ID` for each owned process.
`sync_state` events and five-second `sync_snapshot` events carry `run_id`, the
immutable policy `revision`, a monotonically increasing per-socket
`connection_id`, and `state` (`unknown`, `syncing`, `caught_up`, or `error`).
`connected`, `disconnected`, and `connection_check_failed` carry the same identity.
Connection evidence is emitted before a completion transition. Ended socket
generations cannot reopen, even if their handlers finish late.

The dependency barrier counts work when a node arrives, before queue insertion,
and completes it only when its handler and retained-content continuations return.
An offline marker may arrive or finish before earlier work, but cannot establish
catch-up while any received work remains. Manual history download keeps received
history processing inside the application's tracked work. Ordered app-state
writes, startup replay, late recovery snapshots, and poll side effects hold work
until their persistence boundary finishes.

`store/sync-debts.json` is a private, atomically replaced recovery journal, synced
with its parent directory. It records work identities and conversation identities,
not message content. Selected message/history-download failures and interrupted
writes survive socket and process changes; unrelated successful work cannot
forgive them. A successful retry clears its own debt. Edits use an additional
mutation fingerprint so replaying an older original cannot recover an unsaved
edit. Scope exclusions and optional attachment downloads do not become message
sync errors. An unreadable/unwritable journal prevents starting sync and requires
repair while preserving the archive.

App-state work also records its event kind, required collections, and a durable
sequence. A verified full snapshot settles only the earlier work it covers after
ordered persistence succeeds; newer failures and unrelated work remain pending.
Startup reconstructs missing collection-recovery markers from that journal.
Older opaque event hashes are settled only by successful replay of the same
action, allowing for its full-sync delivery flag. Unmatched legacy failures stay
visible rather than being discarded on reconnect.

Receipt work retains its identifiers, receipt type and timestamp for local retry
after restart, without message bodies. Retry uses the current chat policy and
the saved account identity, updates only the local archive, and never sends a
receipt or message. Empty message IDs are ignored without dropping valid IDs in
the same receipt batch; real database errors remain recoverable failures.

The SDK also reports intentionally unavailable view-once content through its
undecryptable-message event. An explicit `is_unavailable` plus `view_once` type
is an expected absence and does not create a permanent sync failure. Its warning
remains visible; a matching event can settle only its exact older false failure.
Unknown unavailable types and hidden decryption failures still require recovery.
New unresolved decryption events retain their original timestamp, bounded
classification flags, and sender identities in the private journal so diagnosis
can distinguish replayed old work from a new failure after restart. Message
bodies, display names, keys and arbitrary error strings are not retained there.
Older entries without this evidence remain unclassified.

For operator recovery of older journals, `sync --repair-sync` requests full
app-state replay when opaque event failures remain and makes bounded requests
to the user's primary device for missing non-mutation messages. It neither
deletes archive data nor clears a failure merely because a request was accepted.
An incoming group message whose old journal lost its sender may remain
unrecoverable until the primary supplies a matching readable copy. Stop the
app-owned service before running a separate writer, retain the same store and
content policy, and keep the phone online. This engine option is not an MCP
operation; ordinary app startup does not enable it.

An explicitly approved fresh baseline is separate operator maintenance. Stop the
app and hold both the service and engine writer locks, take a complete private
backup, then durably archive the exact recovery journal and an acknowledgement
audit under `store/sync-baselines/` before atomically emptying only the active
journal. This acknowledges unverified historical work; it does not recover that
work or delete messages, media, session data, credentials or scope. Restart must
still establish catch-up from a fresh delivery boundary. Never perform this
reset automatically on reconnect or ordinary startup.

Only explicit completion events establish `caught_up`; periodic snapshots prove
liveness and can invalidate a failed connection check. Quiet healthy connections
remain caught up without a time expiry. `last_synced_at` remains the persisted
successful activity marker, independently of current completion. Catch-up means
selected chats' available updates were saved through the current WhatsApp delivery
boundary; it does not promise complete historical coverage. Synthetic barrier and
recovery tests do not establish live reconnect or sleep/wake compatibility.

Content skips are distinct from retained messages and storage failures. The
patch gates live/history messages, status, locations, reactions/edits, calls,
stars, polls (including cross-chat reference/fallback targets), message webhook
and media enqueue side effects, cached file copying, direct downloads, queued
media, phone retry, and final text/file send dispatch after recipient resolution. Minimal conversation/contact metadata and protocol
cleanup continue. Previously retained excluded records are not deleted.

The private adapter can ask for verified identity aliases without reading the
session database:

```sh
wacli --store DIR --content-policy PATH --read-only --json policy resolve --chat 15551234567@s.whatsapp.net
```

Repeat `--chat JID` to resolve all selected chats in one process. It returns the
ordinary upstream JSON envelope with a deduplicated `data.chat_ids` union in
first-seen order and `data.groups`, a mapping from each normalized input JID to its verified alias array. The private catalog uses these groups to keep chat selection correct after LID-to-phone migration. Only exact
stable JIDs are accepted; names are never authorization identities. LID/phone
pairs come from upstream's verified local mapping. Group membership does not
authorize a participant's direct chat.

The policy is an ingestion/media defense, not an authenticated agent API. The
companion independently enforces current scope, sending grants, response row
validation, revisions, and authenticated attachment handles across its MCP
surface. A user who directly invokes the local CLI owns the upstream archive.

### Offline media after disconnect

`media download --cache-only --chat JID --id MESSAGE --output PATH` copies a
verified cached attachment under the current content policy. It exits with
`media_not_cached` on a missing or corrupt cache, before opening a writable
session, contacting the CDN, or requesting phone recovery. It works with
`--read-only` and rejects `--retry-on-phone`. Ordinary `--read-only` media download
alone can contact the CDN; it is not an offline guarantee.

# Bundled WhatsApp engine

Upstream is [openclaw/wacli](https://github.com/openclaw/wacli), pinned at
[`5578d1b`](https://github.com/openclaw/wacli/tree/5578d1b6892f50a8f2329b11a6b6e27a1a81f88b).
`UPSTREAM_REVISION` is authoritative. The upstream MIT notice is in
[LICENSE.upstream](LICENSE.upstream) and must accompany distributed binaries.

`content-policy.patch` adds LocalWhatsAppMCP's immutable process policy at the
upstream app boundary. It does not replace WhatsApp connection/session ownership,
SQLite migrations, message parsing, media recovery, or the command protocol.
See [selective storage research](../../docs/research/selective-storage.md).

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
patch with `git apply --check`, and build from that source. Generated source and
executables live in ignored `.build/`. `prepare.sh` intentionally resets only
its disposable `.build/pinned-source` checkout; make patch changes elsewhere.
Do not place WhatsApp account data in a source/build checkout.

`build.sh` prints the executable path. `test.sh` runs the complete upstream Go
suite both with and without FTS5, then vet and whitespace checks. Policy fixtures
use synthetic accounts and disposable stores; no login or network WhatsApp data
is required. Live QR/linking and actual phone media recovery require the user's
installation acceptance test.

## Integration contract

Every app-owned invocation supplies `--store DIR --content-policy PATH`.
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

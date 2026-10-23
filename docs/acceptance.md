# Installation and acceptance

This is a local **0.1.0** implementation for installation testing. The checked build is Apple silicon (`arm64`), ad-hoc signed, with a macOS 13 deployment target verified in all three Mach-O executables. Runtime checks ran on the build Mac; neither Intel nor every supported macOS version has been exercised. Developer ID signing and notarization require the owner's release credentials.

## Install and first use

1. Open `dist/LocalWhatsAppMCP-0.1.0-arm64.dmg` and drag **LocalWhatsAppMCP** to **Applications**. Launch the installed app and click its menu bar icon.
2. Choose **Connect WhatsApp**, then scan the displayed QR from WhatsApp's **Linked Devices** screen on your phone. Content delivered during setup is captured immediately; agent access waits for your scope choice.
3. Choose **Continue with all chats** or **Select specific chats**. All chats includes future conversations. Selected chats includes only your saved selections; its search and Select all operate across every matching result.
4. Choose **Connect an agent**, name the connection, choose its client, and give the generated instructions to that local agent. The bundled helper saves its credential privately and verifies the service. Reload that client's MCP configuration if required. Sending starts disabled.
5. Enable sending only for connections that should send. Change shared chat scope or disconnect individual agents from the app. **Quit** stops the background processes; closing the panel keeps them running.

The archive and configuration live in `~/Library/Application Support/LocalWhatsAppMCP/`. App replacement preserves them. App-managed secrets live in Keychain; the upstream WhatsApp session has its own protected store. Existing excluded content can remain on disk, while current server policy denies access. Removing a chat does not promise to erase previously captured content or recall content already given to an agent.

Use [cloud setup](cloud-setup.md) only if you need a hosted agent. Mac sleep, loss of connectivity, or quitting the app makes its public endpoint unavailable. The app does not keep the Mac awake.

## Verified without a personal account

The [build instructions](building.md) provide repeatable commands. These checks use synthetic accounts, messages, media, provider commands, and OAuth clients; they do not establish live WhatsApp or provider compatibility.

| Boundary | Evidence |
| --- | --- |
| Initial capture, all/selected storage, fail-closed policy | Pinned-engine tests cover policy load before store opening, selected payload rejection, preserved minimal catalog, and initial/all modes. |
| Side records and alternate content paths | Engine fixtures cover FTS, reactions/edits, calls/stars, poll reference targets, webhook/media queues, phone retry, cached file copying, and final text/file dispatch. |
| Identity aliases | Engine fixtures use verified LID/phone mappings and reject identity guesses. Final review additionally requires picker canonicalization after migration. |
| Scope transitions and restart | Service fixtures exercise policy revision acknowledgement, a failed transition across restart, retained excluded archive, and current-scope checks. |
| Independent agents and sending | Actual MCP handler fixtures exercise one-time pairing, separate credentials, revocation, direct send-tool denial, unselected destinations, and request identifiers preventing duplicate sends. |
| Reading, updates, files | Tests cover bounded reads/search/context, scoped encrypted checkpoints, checkpoint invalidation after scope changes, cached attachment denial, and chunked attachment content. The real pinned engine is also exercised against a synthetic SQLite archive. |
| OAuth and cloud separation | Tests exercise discovery, dynamic registration and wrong-client rejection, the expected redirect flow, PKCE, CSRF, one-use private browser bootstrap, handoff expiry, hash-only persistence, restart, refresh rotation, and revocation. Private controls remain absent from the public listener. CIMD fetching is implemented but not verified against a live hosted client. |
| Tunnel management | Synthetic executable tests check loopback routing, credential-free process arguments, startup, and awaited shutdown. Actual provider login/DNS and provider retry behavior remain unexecuted. |
| Native UI | Swift model/runtime tests and seven rendered light/dark surfaces cover selection/search, service ownership, fail-closed state defaults, callbacks, and graceful demo Quit. The rendered demo QR was decoded through Vision. |
| Packaging | Real Mach-O fixtures verify helper presence, metadata, architectures, minimum OS load commands, dynamic library dependencies, signatures, and callback scheme. The DMG was mounted read-only and the ZIP extracted for bundle verification. |

The [integration review](implementation/reviews/2026-10-23.md) records independent Standards and Spec findings and resolution status. Final verification results are recorded in the [distribution resolution](implementation/comments/105-distribution.md).

The optional `python3 scripts/smoke-bundle.py` exercises the built service, real macOS Keychain storage, HTTP authentication, the stdio bridge, pairing replay, revocation, and owner-pipe shutdown. It uses a temporary directory under `build/`, deletes only its own Keychain entries, and does not link WhatsApp or edit any real client's configuration. It requires an unlocked usable Keychain.

## Live installation checks

These checks remain for the user's account. No real message was read or sent during the build.

- **Link and capture:** scan the QR, observe catalog/history arriving, interrupt setup and reopen, then confirm that agent access remains unavailable until scope is saved. Available history may be partial.
- **Scope:** select a test conversation, ask an agent to find/read it, and verify that another conversation is absent. Remove the selected chat and repeat reading and cached-attachment access. Add a new group and compare All chats with Selected chats.
- **Agents:** connect two clients through their generated instructions. Verify reading, enable sending for only one, send a test message with a fresh request ID, and revoke one connection while keeping the other usable. Inspect uncertain sends before retrying with a different ID.
- **Attachments:** retrieve a known small file, a file larger than one MCP chunk, and an older unavailable file. Expect content or an explicit recovery/unavailable result. Phone-mediated recovery depends on the phone retaining the file and responding.
- **Lifecycle:** close the panel, sleep/wake, reopen, and Quit. Confirm the app reports recovery honestly and that Quit stops its service, engine, and optional tunnel. Enable Open at login using any required macOS approval.
- **Cloud:** perform provider login and stable-hostname setup with either provider, finish the hosted client's normal browser OAuth connection in the same browser session, test authenticated tools/files, restart the app, and disconnect that agent. Do not infer client compatibility from a reachable tunnel alone.

Codex, Claude Code, and generic stdio installation paths are implemented. ChatGPT's documented OAuth client metadata is the hosted default; another hosted client needs its documented client ID and redirect URI. This is a description of implemented adapters, not a claim that every version or account configuration has passed these live checks.

## Recovery

Preserve the application data directory before investigating storage errors. A missing or invalid confirmed configuration blocks access; restore the matching settings instead of deleting them to reopen the archive. A failed client configuration install can resume using the connection ID printed by the helper. The helper's `doctor` command is redacted; its `headers` command is only for the MCP host and must not be run in an agent conversation.

If provider authentication expires, use fresh cloud setup instructions; background retries never open a login window. If WhatsApp revokes the linked device, reconnect from the app. No history-completeness claim is implied by a ready status or recent event timestamp.

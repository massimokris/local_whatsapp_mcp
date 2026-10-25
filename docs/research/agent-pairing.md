# Agent pairing and cloud connection

Date: 2026-10-23. Research for [Preserve one-time pairing across MCP client connections](../wayfinder/issues/003-agent-pairing.md).
Status: historical architecture research supported by documentation; no integration tests performed for this note. The [local-only revision](../wayfinder/comments/001-product-contract.md#local-only-revision--2026-10-23) supersedes its cloud/provider/OAuth recommendations. The [SQLite credential revision](../wayfinder/comments/003-agent-pairing.md#sqlite-credential-revision--2026-10-23) supersedes Keychain storage, and the [generic HTTP pairing revision](../wayfinder/comments/003-agent-pairing.md#generic-http-pairing-revision--2026-10-24) supersedes the client-specific installer, private redemption, credential-header helper, and stdio bridge below. Current setup uses generic instructions and a temporary code exchanged over loopback HTTP; each agent configures its own client. The following research is retained as decision history. The [component interfaces](../implementation/interfaces.md#local-mcp-boundary) define the current transport boundary.

## Answer

Preserve the pasted instructions plus single-use token as the setup interface.
Ship a setup helper with the app; it redeems the token privately, installs the credential, and configures the selected client.
Use authenticated Streamable HTTP locally, with a stdio adapter where a client requires it.
For hosted clients that require OAuth, implement a standard browser connection over the same app-owned grant.
Users configure neither headers nor OAuth infrastructure, and create no separate identity-provider account.
Successful redemption grants reading immediately; sending remains an independent, initially disabled app permission per agent.

## Documented facts

- MCP authorization is optional overall. HTTP implementations supporting it should follow the OAuth authorization specification; stdio should use its separate credential path.
- In that standard HTTP flow, the authorization server implements OAuth 2.1; clients discover metadata and send access tokens in the Authorization header.
- The authorization server may be co-hosted with the resource server. The specification does not mandate an external identity provider or an account sign-up screen. [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
- The official Go SDK provides `NewStreamableHTTPHandler`, bearer-token middleware, a custom token verifier, request identity, and protected-resource metadata helpers.
- Its documented server auth integration supplies resource-server building blocks, not the product's full authorization-server, pairing, credential-storage, and client-installation implementation. [Go SDK protocol guide](https://github.com/modelcontextprotocol/go-sdk/blob/12e768b461fd06ea80b0d87e14ac7fa0d24c606a/docs/protocol.md#authorization), [auth source](https://github.com/modelcontextprotocol/go-sdk/blob/12e768b461fd06ea80b0d87e14ac7fa0d24c606a/auth/auth.go)
- Codex local documents Streamable HTTP bearer tokens, OAuth, headers, and `http_headers_helper`; its helper support is for locally executed HTTP connections, not remote execution environments.
- Codex also supports stdio launch commands. These are documented mechanisms, not evidence that this proposed product already works. [Official OpenAI MCP documentation](https://learn.chatgpt.com/docs/extend/mcp)
- Claude Code documents stdio, HTTP, OAuth, static headers, and `headersHelper`; its helper runs at connection/reconnection and must return header JSON promptly. [Claude Code MCP](https://code.claude.com/docs/en/mcp)
- Hosted ChatGPT plugin authentication expects the MCP OAuth flow and does not accept customer-provided custom API keys. This statement concerns that hosted surface, not local Codex configuration. [Official OpenAI plugin authentication](https://developers.openai.com/plugins/build/auth)

## Proposed local flow

The commands and record names below are proposed product interfaces, not existing APIs.

1. The app generates a cryptographically random, single-use setup token with a short expiry, for example ten minutes.
2. Generated instructions identify the bundled helper by absolute path and the selected client; the user pastes them with the token into the local agent.
3. The agent invokes `localwhatsappmcp setup --client <adapter> --token-stdin`; the token is supplied on stdin, not a command-line argument.
4. The helper redeems it over private local IPC. The app checks expiry and atomically consumes it while creating one agent connection.
5. The connection starts with `read_enabled=true`, `send_enabled=false`; redemption requires no additional app approval.
6. The helper saves the new connection credential directly into macOS Keychain, then writes the client configuration. Its output contains only non-secret identifiers and setup status.
7. The helper verifies the configured client can discover tools and read connection status. The app shows that agent as connected.

Use an unguessable token, not a short numeric code, because this workflow pastes rather than types it.
Store the setup-token digest and consume it once; rate-limit failed attempts and expire abandoned setup records.
Do not expose redemption, agent creation, permission changes, or tunnel administration as ordinary MCP tools.
Use a user-owned Unix socket or equivalent private IPC for setup; do not forward it through the tunnel.
Treat possession of the setup token as the user's intentional authorization for the one designated connection.

## Credential delivery and client adapters

For documented local HTTP clients, configure the helper command with a non-secret connection ID.
The helper retrieves that connection's secret and returns Authorization-header JSON directly to the MCP host process.
Codex uses `http_headers_helper`; Claude Code uses `headersHelper`. Keep these adapters version-aware.
Never run the header-producing command as an agent-visible diagnostic; provide a separate redacted `doctor` command.
Prefer a short-lived access token obtained using a private rotating connection secret, rather than copying a permanent bearer token into configuration.
Only the helper and server handle those values; logs, terminal summaries, setup results, and MCP tool output omit them.

If a target supports stdio but lacks a suitable credential helper, configure a bundled stdio adapter with the connection ID.
That adapter retrieves its credential privately and forwards protocol operations to the same authenticated local service.
It does not start a second WhatsApp connection or own a separate message database.
Do not silently fall back to unauthenticated HTTP if an adapter or secret lookup fails.
Apply configuration changes atomically, preserve other servers, and make repeated setup resume the installed connection rather than duplicate it.
If setup fails after redemption, the saved connection credential permits a bounded repair; the consumed token never becomes reusable.

## Go service and permission enforcement

Wrap the MCP HTTP handler with `auth.RequireBearerToken` and supply a verifier backed by the app's credential/grant store.
Set expiry and connection identity in `TokenInfo`; validate token audience, revocation, and current permissions as app responsibilities.
Use the agent connection ID as the authenticated principal, rather than trusting MCP `clientInfo` as identity.
The Go middleware handles bearer extraction, expiration, and configured scope checks; tool handlers receive verified token context. [Go auth implementation](https://github.com/modelcontextprotocol/go-sdk/blob/12e768b461fd06ea80b0d87e14ac7fa0d24c606a/auth/auth.go)

Keep transport authorization and app capabilities distinct: a connection scope such as `whatsapp.access` means access to capabilities enabled for this connection.
The initial effective capability is read-only. The connection screen states that sending is disabled until the owner enables it in the app.
This avoids making the app's send toggle depend on manually editing OAuth scopes or client headers.
Every tool invocation checks the current connection policy; sending additionally checks `send_enabled` immediately before dispatch.
Revocation invalidates the connection secret, access/refresh tokens, and any live session; disabling sending takes effect for already-connected clients.
Advertising or hiding a send tool is presentation, not enforcement. The server must deny a direct send call when sending is disabled.
All message access remains limited by the app's configured chat selection, regardless of transport or client-supplied parameters.

## Optional cloud flow

The local agent performs setup for the chosen cloud connection; it does not gain a second message-access identity merely by installing it.
The app enables a separate public-facing listener behind the chosen tunnel, exposing only MCP and required OAuth endpoints.
Keep the private setup/admin listener separate so a catch-all proxy rule cannot publish it accidentally.

1. Reuse an existing provider installation/authentication where available; otherwise install the selected provider and guide its browser login.
2. Default to ngrok; use the account's persistent hostname. Cloudflare uses a named tunnel and configured hostname.
3. Save the provider, tunnel identifier, hostname, startup parameters, and private credential reference in the app's configuration.
4. Let the app own the tunnel process after setup, so it restarts with the app and can recover after interruption.
5. Verify the HTTPS endpoint from outside the local listener, including unauthenticated denial, metadata discovery, and a valid authenticated read.
6. Configure the chosen cloud MCP client using its actual supported connection UI/API; complete its normal browser Connect step when required.

For an OAuth-only hosted client, the app supplies discovery metadata, authorization-code flow with PKCE, token/refresh handling, and supported client registration.
Use a maintained OAuth server implementation within the Go service; the MCP SDK's resource-server helpers alone do not supply this component.
Support CIMD and compatible DCR where necessary; validate registered redirect URIs, resource/audience, issuer, PKCE, and client identity.
The browser connection associates a cloud client's OAuth credential with the already-created read-only agent grant.
No grant may be chosen merely because it is the latest pending setup or because an unauthenticated request names its ID.

Proposed owner-authentication mechanism: the setup helper establishes a short-lived owner browser session through a single-use local handoff.
That session is limited to this pending connection and expected client; it is not a general administration login.
The standard Connect page binds the exact OAuth request, client ID, redirect URI, and PKCE challenge to that session and grant.
Completing that browser flow issues the cloud client's credential without a second app approval or a new account.
The local handoff and browser session expire promptly; failed, mismatched, or unrelated requests fail closed.
Keep handoff secrets out of chat, logs, referrers, and persistent URLs; use private helper/browser communication and one-time exchange.
This browser-binding design is an implementation proposal that requires security and client testing, not a documented turnkey SDK feature.

## Provider constraints to preserve in implementation

- ngrok's documented macOS setup installs an authtoken; browser login alone is not documented as automatically importing it. [ngrok macOS setup](https://ngrok.com/download/mac-os)
- Reuse configured ngrok credentials without printing them. An app-owned config can use a separate file, and ngrok supports `NGROK_AUTHTOKEN`. [ngrok configuration](https://ngrok.com/docs/gateway/agent/config)
- For a new ngrok account, implement and test a private credential-import step after provider login; never ask the user to paste the long-lived token into agent chat. The exact browser-to-helper import path is not established by the reviewed documentation.
- Cloudflare documents `cloudflared tunnel login` opening the browser and creating a local account certificate; named-tunnel creation then generates a tunnel credential file. Its published-application flow requires a Cloudflare domain. [Cloudflare local setup](https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/create-local-tunnel/)
- Cloudflare Quick Tunnels change hostname and do not support SSE; use a named tunnel for this product's persistent connection. [Cloudflare Quick Tunnels](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/)
- Tunnel credentials authorize the tunnel provider; MCP credentials authorize message access. They are separate and never interchangeable.

## Acceptance checks not run

- Record exact client/SDK versions; verify protocol negotiation, discovery, tool calls, reconnect, and configuration reload for each advertised client.
- Verify both local HTTP helpers and the stdio adapter can access Keychain after login/restart without leaking credentials or introducing an extra app approval.
- Confirm duplicate redemption, expired tokens, replay, interrupted installation, and simultaneous setup cannot create unintended grants.
- Using only its MCP credential, confirm agent A cannot retrieve B's credential, reuse B's session, change capabilities, or mint another connection.
- Do not imply this creates OS isolation between arbitrary agents already able to execute as the same macOS user; that needs separate sandbox controls.
- Confirm initial sends fail, enabling the app toggle permits sends, and disabling/revoking immediately blocks existing connections.
- Exercise hosted browser binding, wrong-client requests, CSRF, PKCE failure, redirect validation, token refresh, and revocation end to end.
- Test first-time and existing-account ngrok credential import without agent-visible secrets; test the full named-Cloudflare path separately.
- Test both public endpoints, SSE where used, provider authentication/interstitial behavior, hostname stability, sleep/wake, and tunnel restart.
- Verify scheduled agent runs can actually reach this endpoint and use the credential; MCP installation does not itself guarantee scheduling support.

## Resolution boundary

The local pairing architecture is specified and fits documented client mechanisms; no additional product decision is needed.
The cloud architecture is specified, but documentation does not prove the proposed private browser binding or first-time ngrok credential import.
The map records this as a partial technical answer and leaves the pairing ticket open for those two mechanisms. Do not claim end-to-end or universal client support.
Further technical work must establish the handoffs while preserving the accepted experience; it does not require another product interview.

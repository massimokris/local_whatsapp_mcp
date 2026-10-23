# Research update: Preserve one-time pairing across MCP client connections

- Date: 2026-10-23
- Ticket: [Preserve one-time pairing across MCP client connections](../issues/003-agent-pairing.md)
- Research worker: `/root/mcp_pairing_facts`
- Result: partial technical answer; ticket remains open and available for follow-up
- Evidence: [Agent-pairing integration](../../research/agent-pairing.md)

The documented local-client mechanisms support a bundled helper that redeems the single-use invitation, creates a read-only agent grant, saves credentials privately, and installs a helper-backed HTTP connection or stdio adapter. App-controlled sending permission remains independent of transport credential configuration. This preserves immediate reading access and avoids an additional app approval.

For hosted clients requiring OAuth, keep standard authorization behind their ordinary browser Connect surface and bind it to the specific pre-paired agent grant. The Go MCP SDK supplies resource-server primitives, not a complete authorization server. The research proposes a private browser handoff and identifies client/protocol validation requirements.

The architecture is plausible, but two experience-critical mechanisms remain unverified:

1. A secure browser handoff that binds the cloud OAuth request to the correct pre-paired grant without adding another app approval or exposing a durable secret.
2. First-time ngrok credential import after provider login, with no long-lived token passing through agent conversation. Reusing existing credentials is documented; automatic import after a new browser login is not established by the reviewed sources.

Keep this ticket open until those mechanisms have a concrete documented route or focused implementation evidence. These are technical follow-up items for the implementer, not new product questions for the user. Do not claim that a hosted client works merely because the tunnel is reachable, or replace the agreed experience with manual OAuth/header setup.

Release the research claim after this update. No account access, provider setup, client configuration, or end-to-end test was performed. If evidence later proves the fixed experience impossible for a target, report that specific consequence rather than silently changing the specification.

## Implementation resolution — 2026-10-23

The end-to-end implementation request claimed this technical follow-up for `massi`. Both remaining mechanisms now have concrete implementation routes and focused verification, so the research question is answered.

The cloud helper opens a one-use loopback browser handoff that posts a short-lived secret to the public HTTPS bootstrap endpoint. The resulting browser session is bound to a specific pre-paired grant, expected client, and exact redirect URI. The ordinary hosted-client OAuth request adds its actual S256 challenge and resource audience; a CSRF-protected Connect submission consumes the session. Deterministic tests exercise incorrect clients, one-use bootstrap, expiration, PKCE exchange/replay, restart, rotating tokens, and grant revocation. No additional approval appears in the native app. See [cloud setup and protocol](../../cloud-setup.md) and [OAuth flow fixtures](../../../backend/internal/cloud/oauth_test.go).

For first-time ngrok setup, the helper explicitly opens the provider dashboard and a native hidden-input credential dialog. The user moves the provider authtoken directly into that dialog; the helper transfers it through the private Unix control route to Keychain. Existing ngrok configuration is imported privately when available. This is a provider credential step, not manual MCP headers or OAuth-server setup, and never passes the credential through the agent conversation. Cloudflare uses its normal browser login and named-tunnel credentials-file import. The [provider helper](../../../backend/cmd/localwhatsappmcp/cloud.go) implements both routes; background recovery never opens login windows.

Local Codex/Claude Code configuration and the generic stdio bridge use independent read-only-by-default grants and the private helper/Keychain path. Actual client reload, real provider authentication/DNS, and hosted browser compatibility remain [live installation checks](../../acceptance.md#live-installation-checks). No real account was connected while resolving this ticket. Closure records an implementation route and synthetic protocol evidence, not universal client compatibility.

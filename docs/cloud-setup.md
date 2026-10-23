# Optional hosted-agent connection

Local access works without a tunnel or provider account. To connect a hosted agent, confirm chat scope, choose **Connect cloud agent**, select **ngrok** (default) or **Cloudflare Tunnel**, and give the generated instructions to your local agent. The instructions contain a ten-minute, single-use setup invitation. A stable public hostname and the chosen provider's account capabilities are required.

The local agent installs the provider executable if needed, arranges the provider's normal login and stable hostname, then invokes the bundled helper with the invitation on standard input. The helper stores credential references in app configuration and secrets in macOS Keychain. It returns only redacted status and public connection information. The app owns the subsequent provider process, retries, shutdown, and startup restoration.

For ngrok, the helper privately imports an existing ngrok configuration when available. For a new account, it opens the provider dashboard and a native secure dialog: copy the provider authtoken directly from the dashboard into that dialog. The dialog is outside the agent conversation. This provider credential step is separate from the hosted client's OAuth connection.

For Cloudflare, the local agent uses the provider browser login, creates a named tunnel and DNS route for the stable hostname, and passes the resulting credentials-file path to the helper. The helper imports its contents privately. Normal app operation uses the Keychain copy. Provider-managed login/credential files remain the provider's responsibility.

After the public MCP endpoint passes discovery and unauthenticated-rejection checks, the helper opens a short-lived private browser setup session. In **that same browser**, add the resulting MCP URL through the hosted client's normal connection surface and finish its OAuth Connect step. There is no LocalWhatsAppMCP account or separate app approval. Generate fresh instructions if the browser setup session expires.

ChatGPT is the default hosted client. Other clients require the local agent to inspect their documented client ID and exact redirect URI and pass those values to the helper. Transport support, file handling, and account availability can vary; [live acceptance](acceptance.md#live-installation-checks) remains necessary.

## Protocol and ownership

The private Unix setup route binds a new read-only agent grant to the expected client and exact redirect URI. The helper serves one random loopback URL that posts a one-use handoff secret to the public HTTPS bootstrap endpoint. The secret stays out of URL query parameters and persistent logs. Bootstrap establishes a Secure, HttpOnly browser cookie. Authorization then binds the actual client, redirect, resource audience, S256 challenge, and request state to that session; a CSRF-protected Connect submission consumes it.

The Go OAuth manager implements authorization code/PKCE and rotating refresh tokens. Durable token records contain digests, and every exchange/refresh/resource request rechecks the active app grant. Disconnecting a cloud agent therefore revokes its access through the same app controls as local agents. Each cloud connection starts with sending disabled. Private control and provider configuration routes are absent from the public HTTP surface.

The tunnel exposes only the loopback public listener. ngrok request inspection is disabled; Cloudflare has a catch-all 404 ingress rule. Background authentication failures report a recovery action instead of opening a provider window. The archive and WhatsApp session remain on the Mac, and Mac sleep or Quit makes cloud access unavailable.

The native URL scheme is reserved and validated, but an unsolicited callback cannot grant access; current setup uses the private browser handoff above.

## Evidence and limits

Implementation: [cloud service](../backend/internal/cloud/extension.go), [OAuth flow](../backend/internal/cloud/oauth_flow.go), [provider helper](../backend/cmd/localwhatsappmcp/cloud.go). Deterministic tests exercise the complete authorization flow, expiry/replay, restart, and revocation, with synthetic clients/providers. These tests do not authenticate a real provider account, establish DNS, or reload a live hosted client.

Primary client contracts consulted on 2026-10-23 include [OpenAI's plugin authentication documentation](https://developers.openai.com/plugins/build/auth) and [ChatGPT's OAuth client metadata](https://chatgpt.com/oauth/client.json). The earlier [pairing research](research/agent-pairing.md) preserves its original evidence and limits; its unresolved mechanisms are resolved by this concrete helper/handoff implementation, with live acceptance still explicit.

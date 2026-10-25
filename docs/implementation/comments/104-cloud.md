# Cloud implementation and verification

2026-10-23: Integrated cloud implementation through `15d636a`. [Cloud setup](../../cloud-setup.md) documents the ngrok/Cloudflare helper routes, private provider credential import, app-owned stable-hostname configuration, and the exact browser handoff.

Backend race tests and vet passed. Synthetic fixtures exercise discovery, one-use/expiring owner handoff, expected-client/redirect binding, PKCE authorization code exchange, CSRF, code replay rejection, hash-only durable credentials, restart, refresh rotation, immediate grant revocation, public/private route separation, and provider command routing without credentials in arguments. Normal tunnel shutdown waits for owned processes. Final review adds cleanup after abrupt backend death, with process-level verification in the final distribution resolution.

No real provider login, DNS setup, or hosted-client connection was performed. Those account-bound checks are explicitly retained in [installation acceptance](../../acceptance.md#live-installation-checks). The integration direction also resolves the earlier [pairing research decision](../../wayfinder/comments/003-agent-pairing.md#implementation-resolution--2026-10-23).

Final review repairs `7ad3140` were integrated through `c6de190`; the implementer reran native, backend race/vet, complete upstream/FTS5, real-engine boundary, and packaging regression checks. This task is closed; final installer acceptance is tracked separately in the distribution task.

## Superseded — 2026-10-23

The user's [local-only revision](../../wayfinder/comments/001-product-contract.md#local-only-revision--2026-10-23) removes this feature from the current product. The cloud package, helper commands, public authorization routes, provider processes, and native setup surfaces are no longer part of the active implementation. The earlier integration and synthetic test results above remain historical evidence; provider login and hosted-client acceptance are no longer current acceptance requirements.

# Distribution and acceptance resolution

2026-10-23: Initial integration is built and packaged. Independent Standards and behavior reviews are retained in the [review report](../reviews/2026-10-23.md). The single review-fix implementer is repairing those findings before final packaging.

Final merge, test, package, and cleanup evidence will be appended when completed. Deterministic checks do not establish live-account compatibility.

## Completed — 2026-10-23

All implementation tasks are closed on `codex/localwhatsappmcp-integration`. The final runtime source snapshot is **`7d003a430d599049fd2ea229da789aef98eb37b2`**, clean when packaged; later commits only record this verification. Review repairs were implemented by one agent in `7ad3140` and merged by a separate merger in `c6de190`. The focused behavior recheck confirmed all three original findings resolved with no material regression.

Final checks on **macOS 26.5.2 / arm64**:

- `./scripts/test.sh` passed: ten native tests, backend race tests/vet, complete patched-engine tests both without and with FTS5, and packaging regression fixtures.
- `./scripts/package.sh` rebuilt all three executables, including the current alias-group engine patch. The actual Mach-O deployment target is macOS 13.0 throughout.
- `python3 scripts/smoke-bundle.py` passed **29 checks** against the packaged executables: private control, initial MCP denial, HTTP host/auth boundaries, real Keychain helper installation, one-use pairing, stdio discovery/status, real-engine catalog/FTS/pagination/context/changefeed, denied sending, scope removal/checkpoint invalidation, revocation, and owner-pipe shutdown. Temporary data and Keychain entries were removed with checked outcomes.
- A bounded network check obtained a **fresh WhatsApp QR** through the packaged service/engine in an isolated Application Support directory. The account remained unlinked and MCP remained setup-blocked. Owner EOF stopped the writer and released its store lock; the temporary store and Keychain item were removed. No QR value, session secret, message, or account data was logged or retained in this repository.
- The DMG mounted read-only; its bundle and Applications link verified. The ZIP extracted and verified. Both archive checksums matched. The packaged native demo launched, rendered, and exited cleanly; its screenshot was visually inspected.
- All six implementation worktrees were removed after checking clean state and that their commits were integrated. Only the integration checkout remains; implementation branches preserve history. No remote push or PR was created.

Deliverables under the ignored `dist/` directory:

| Artifact | SHA256 |
| --- | --- |
| `LocalWhatsAppMCP-0.1.0-arm64.dmg` | `920daf1274a098810d00f205d0b644de5ab3ec1b45155512edd1fce122aef85c` |
| `LocalWhatsAppMCP-0.1.0-arm64.zip` | `75a2f728fcefc56b78411213c5eb8a4ecb6ef6a407f1966c1d5461e0d620a2b8` |

The complete `LocalWhatsAppMCP.app` is also available there. Its bundled wacli SHA256 is `7ff6f23ee20bbe473d070c84f14ae3e51f190d55cd35c700426e4089e33e28ee`. `Contents/Resources/BuildInfo.txt` records source/toolchain/pin details. Local evidence remains in ignored `build/final-test.log`, `build/final-package.log`, `build/final-smoke.log`, `build/final-qr-smoke.log`, `build/final-artifact-qa.log`, and `build/final-demo.png`; earlier native previews were preserved under `build/native-previews/`.

The app is ready for local installation and account testing. It is **ad-hoc signed**, not Developer ID signed or notarized. Real QR scanning/history ingestion, phone media recovery, user-client configuration reload, provider login/DNS/hosted OAuth, OS login-item approval, and sleep/wake with a linked account were not verified. Intel and a full macOS-version matrix were not tested. These limits do not change the accepted product behavior or claim universal compatibility.

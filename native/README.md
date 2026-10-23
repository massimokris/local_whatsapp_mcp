# Native application

The Swift package builds the menu bar executable. Its deployment target is macOS 13 because login registration uses `SMAppService`. Verified with Apple Swift 6.2.4 on Apple silicon; additional architectures and OS versions require their own packaging checks.

```sh
swift test --package-path native
swift build -c release --package-path native
```

A complete app bundle supplies `Contents/Helpers/localwhatsappmcp` and `Contents/Helpers/wacli`, plus the `localwhatsappmcp` URL scheme. The executable communicates with the private service through the bundled helper, using the contract in [interfaces.md](../docs/implementation/interfaces.md). Running the bare executable without those helpers is only useful for the demo preview.

## Safe UI preview

`--demo` (or `LWMCP_DEMO=1`) uses deterministic sample data in memory. It does not launch a helper, link WhatsApp, register login, change saved preferences, or read the real archive. The banner identifies every demo surface.

```sh
LWMCP_DEMO=1 LWMCP_DEMO_PAGE=chats LWMCP_APPEARANCE=Dark native/.build/debug/LocalWhatsAppMCP
```

Preview pages are `home`, `qr`, `initial`, `chats`, `agents`, `pairing`, `cloud`, and `settings`. Appearance accepts `System`, `Light`, or `Dark`. For a native rendering, set `LWMCP_CAPTURE_PATH` to an absolute PNG destination; `LWMCP_CAPTURE_EXIT=1` quits through the normal app termination path after capture. These capture variables are ignored outside demo mode.

## Verification recorded 2026-10-23

Six public seam tests pass: search-scoped selection across 100 off-screen matches; preservation of other selections; contact/diacritic/normalized phone search; one owned service with graceful shutdown; rejection of requests after shutdown; fail-closed defaults for incomplete state; and strict native callback parsing. Some test methods cover multiple assertions at one seam.

Native renders were inspected for light chat selection, light initial-scope onboarding, dark QR, dark agents, light pairing, light cloud status, and dark settings. The demo QR round-tripped from its native PNG through macOS Vision's QR decoder. The demo capture runs exited successfully through the app Quit path. Layout uses the reference palette, 12 pt monospaced text, aligned rows, hairlines, and right-side chat checkboxes. Native controls provide keyboard focus and accessible labels.

Actual WhatsApp linking, client installation, provider login, login-item approval, and Mac sleep/wake remain integration/installation acceptance checks. Demo screenshots do not establish account or client compatibility.

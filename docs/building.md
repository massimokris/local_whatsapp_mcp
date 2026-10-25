# Build and package LocalWhatsAppMCP

The project produces a native menu bar app and two bundled executables. Build on
macOS with Xcode command line tools (including a valid macOS SDK), Swift 5.9 or
newer, Git, and Go 1.27.3. The pinned engine declares Go 1.27.0 and toolchain
1.27.3. Dependency fetching needs network access; building and tests do not need
a WhatsApp account or credentials.

```sh
./scripts/test.sh
./scripts/package.sh
```

`test.sh` runs native tests, backend race tests/vet, upstream engine tests with
and without FTS5, and packaging checks. `package.sh` first builds, signs, and
verifies the app; then produces a DMG, ZIP, and SHA256 checksums under `dist/`.
Use `./scripts/build.sh` for only the app or `./scripts/package.sh --no-build`
to repackage an already verified build.

The default architecture is the build Mac's architecture. `ARCH=arm64` and
`ARCH=x86_64` select a single target architecture; a successful cross-build alone
does not establish runtime compatibility. The deployment target is explicitly
macOS 13.0 for Swift and both CGO executables. The scripts reject a different
deployment override and verify the actual Mach-O load commands, not merely the
app's Info.plist. `SDKROOT` is resolved from the selected Xcode SDK; they do not
change the machine's Xcode selection or global environment.

The fixed layout is:

```text
LocalWhatsAppMCP.app/Contents/
  Info.plist
  MacOS/LocalWhatsAppMCP
  Helpers/localwhatsappmcp
  Helpers/wacli
  Resources/AppIcon.icns
  Resources/BuildInfo.txt
  Resources/wacli-LICENSE.txt
  Resources/ThirdPartyNotices.txt
  Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle/WhatsAppLogo.png
  Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle/AssetCredits.txt
```

The bundle declares menu bar behavior and includes an original generated icon.
The local-only app needs no browser callback scheme. Build information records
the source commit, tracked changes, engine pin/patch hash,
tool versions, SDK, and target. Inputs are pinned and commands repeatable;
code-signing timestamps, filesystem metadata, and toolchain output mean the
DMG is not promised to be byte-for-byte reproducible.

`Helpers/localwhatsappmcp` remains the Go service and private app-control
executable. Agent setup uses generic HTTP pairing instructions; the bundle no
longer provides client configuration installers, credential-header commands,
`doctor`, or a stdio bridge. The `Helpers` directory name describes bundle
layout, not an agent installation requirement.

`verify-bundle.sh APP_PATH` rejects missing helpers, malformed metadata,
different executable architectures, a helper that
requires a newer OS, development-machine dynamic library dependencies, and
invalid app/helper signatures. Its tests compile real disposable Mach-O
fixtures. Build outputs and prepared engine sources are ignored by Git. The
packaging scripts never open WhatsApp stores or copy runtime account data.

## Local installation

For reinstall testing, quit the running LocalWhatsAppMCP app, open
`dist/LocalWhatsAppMCP-0.1.30-arm64.dmg`, and drag the app to Applications,
replacing the existing copy. Alternatively, copy the complete `.app` from
`dist/`. Open the Applications copy and use its menu bar icon. A fresh
installation asks you to connect WhatsApp and choose shared chat scope; the
local MCP service starts automatically.
Open **Local MCP** to create setup instructions, manage sending permission, or
remove an agent’s access. Account state lives in
`~/Library/Application Support/LocalWhatsAppMCP/` and survives app replacement.
Keep that data directory when reinstalling.

The default build uses ad-hoc signing, so it is locally installable but is not
an Apple-notarized public release. A downloaded copy can trigger Gatekeeper;
after confirming its provenance, use the standard Privacy & Security approval
for that app. Do not disable Gatekeeper globally. Launch at login uses the
installed app and may require the operating system's approval.

For the verified boundaries and live-account checklist, see [installation and
acceptance](acceptance.md). After building, `python3 scripts/smoke-bundle.py`
checks the bundled service, local SQLite credential database, HTTP pairing exchange, and authenticated MCP using
disposable data. It removes its temporary data directory on exit and requires no
Keychain access.

Clients configured by older releases to invoke `headers` or `stdio` must be
reconfigured by their agent with the new setup instructions. Existing bearer
grants are retained, and the app does not rewrite client configuration files.

## Release signing and notarization

With a Developer ID Application certificate already installed in Keychain:

```sh
SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)' ./scripts/package.sh
```

This signs each helper before the enclosing app with hardened runtime and a
secure timestamp, then signs the DMG. For notarization, first save your own
notarytool credentials in Keychain using Apple's normal setup, then name that
profile without putting credentials in the shell command:

```sh
SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)' \
NOTARY_PROFILE='localwhatsappmcp-release' ./scripts/package.sh
```

The script submits and waits for app and DMG acceptance, staples both, and
validates the tickets. It refuses to notarize an ad-hoc build. No signing
identity, Apple account, or notarization credential is bundled
or stored in this repository. Release signing/notarization requires the owner's
credentials and is not established by local ad-hoc verification.

`APP_VERSION` (default `0.1.0`) and `APP_BUILD` (default `1`) set bundle versions.
Keep the same bundle identifier for updates so login registration continues to
identify this application.

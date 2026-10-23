# Distribution tooling

- ID: `LW-106`
- Status: open
- Labels: `wayfinder:task`
- Mode: AFK
- Parent: [Implementation plan](../plan.md)
- Assignee: massi
- Blocked by: [Native menu bar app](103-native.md)
- Resolution: [Implementation and verification](../comments/106-packaging.md)

Create reproducible build/test/package scripts for the accepted native app, Go helper/service, and patched engine; macOS 13 deployment target; app Info.plist, URL scheme, icon, helper layout, local ad-hoc signing, and DMG. Verify scripts on available integrated components and complete full packaging once engine/service merges. Provide release signing/notarization hooks without assuming a developer identity. Own scripts/packaging assets and build documentation; final acceptance owns the installed runtime check and tracker closure.

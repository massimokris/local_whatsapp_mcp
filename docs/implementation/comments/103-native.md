# Native implementation and verification

2026-10-23: Integrated `codex/lw-native` via merge `8a2bf38`, including implementation `7028855`. Six public model/runtime tests pass on the integration branch; SwiftPM debug and release builds succeed. The implementer rendered and inspected seven native surfaces in both appearances, decoded the rendered QR with Vision, and verified graceful demo Quit. [Native build and preview evidence](../../../native/README.md) records commands and safe fixture options.

The native app implements the accepted onboarding, full catalog selection, agent management, cloud instructions, settings, login registration, wake handling, and retained menu bar lifecycle. Production account QR scanning, operating-system login enablement, actual provider/client connections, and sleep/wake with a linked account were not verified by this deliverable. This closes the native implementation work, not those live checks.

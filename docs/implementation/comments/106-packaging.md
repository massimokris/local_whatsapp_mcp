# Distribution tooling and verification

2026-10-23: Integrated packaging implementation `9dca905` and `83336c5` through `e4d7b07`. [Build instructions](../../building.md) record verified build/test/package commands, toolchain and deployment requirements, bundle layout, and optional Developer ID/notarization hooks.

Built an Apple silicon application containing the native executable, Go helper/service, patched wacli, icon, build information, and license notices. Actual Mach-O deployment targets are macOS 13. Bundle tests reject missing helpers, invalid metadata/scheme/signatures, architecture mismatch, newer helper deployment targets, and non-system dynamic libraries. The ad-hoc signed DMG mounted read-only with the Applications link and verified app; the ZIP extracted and verified too. A packaged demo launch rendered and exited successfully.

The packaged application is a local installation build, not an Apple-notarized public release. No Intel runtime or macOS-version matrix was run. Final packaging after review repairs is recorded in the [distribution resolution](105-distribution.md).

Final review repairs `7ad3140` were integrated through `c6de190`; the implementer reran native, backend race/vet, complete upstream/FTS5, real-engine boundary, and packaging regression checks. This task is closed; final installer acceptance is tracked separately in the distribution task.

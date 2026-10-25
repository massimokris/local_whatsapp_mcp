#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$root/build"
scratch=$(mktemp -d "$root/build/package-test.XXXXXX")
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
app="$scratch/LocalWhatsAppMCP.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers"
expect_failure() {
  expected=$1
  if "$root/scripts/verify-bundle.sh" "$app" >"$scratch/result" 2>&1; then
    printf 'FAIL: bundle unexpectedly accepted (%s)\n' "$expected" >&2
    exit 1
  fi
  if ! grep -F "$expected" "$scratch/result" >/dev/null; then
    cat "$scratch/result" >&2
    printf 'FAIL: expected diagnostic: %s\n' "$expected" >&2
    exit 1
  fi
}
expect_failure 'Missing bundled executable: Contents/MacOS/LocalWhatsAppMCP'
printf 'int main(void) { return 0; }\n' >"$scratch/fixture.c"
SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
export SDKROOT
xcrun clang -mmacosx-version-min=13.0 "$scratch/fixture.c" -o "$scratch/fixture"
cp "$scratch/fixture" "$app/Contents/MacOS/LocalWhatsAppMCP"
cp "$scratch/fixture" "$app/Contents/Helpers/localwhatsappmcp"
expect_failure 'Missing bundled executable: Contents/Helpers/wacli'
cp "$scratch/fixture" "$app/Contents/Helpers/wacli"
expect_failure 'Missing bundle metadata: Contents/Info.plist'
cp "$root/packaging/Info.plist" "$app/Contents/Info.plist"
expect_failure 'Missing bundled resource: Contents/Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle/WhatsAppLogo.png'
resources="$app/Contents/Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle"
mkdir -p "$resources"
cp "$root/native/Sources/LocalWhatsAppMCP/Resources/WhatsAppLogo.png" "$resources/WhatsAppLogo.png"
expect_failure 'Missing bundled resource: Contents/Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle/AssetCredits.txt'
cp "$root/native/Sources/LocalWhatsAppMCP/Resources/AssetCredits.txt" "$resources/AssetCredits.txt"
# Exercise the production lookup from a relocated .app, independently of the
# SwiftPM build directory. The actual PNG must decode from shipped resources.
cat >"$scratch/main.swift" <<'SWIFT'
import AppKit

let resourceRoot = Bundle.main.resourceURL!.standardizedFileURL
precondition(AppAssets.bundle.bundleURL.deletingLastPathComponent().standardizedFileURL == resourceRoot)
let logo = AppAssets.whatsAppLogo
precondition(logo.isValid && logo.size.width > 0 && logo.size.height > 0)
print("Bundled logo loads from the relocated app.")
SWIFT
swiftc -target "$(uname -m)-apple-macosx13.0" "$root/native/Sources/LocalWhatsAppMCP/AppAssets.swift" "$scratch/main.swift" -o "$app/Contents/MacOS/LocalWhatsAppMCP"
"$app/Contents/MacOS/LocalWhatsAppMCP"
cp "$scratch/fixture" "$app/Contents/MacOS/LocalWhatsAppMCP"
expect_failure 'Bundle signature verification failed'
sign_fixture() {
  /usr/bin/codesign --force --sign - "$app/Contents/Helpers/localwhatsappmcp" 2>/dev/null
  /usr/bin/codesign --force --sign - "$app/Contents/Helpers/wacli" 2>/dev/null
  /usr/bin/codesign --force --sign - "$app" 2>/dev/null
}
sign_fixture
"$root/scripts/verify-bundle.sh" "$app"
# Runtime credentials must not reintroduce Keychain permission prompts.
printf '#include <Security/Security.h>\nint main(void) { return (int)SecItemDelete(0); }\n' >"$scratch/keychain-fixture.c"
xcrun clang -mmacosx-version-min=13.0 -framework Security "$scratch/keychain-fixture.c" -o "$app/Contents/Helpers/localwhatsappmcp"
sign_fixture
expect_failure 'App helper must use local SQLite credentials, without Keychain access'
cp "$scratch/fixture" "$app/Contents/Helpers/localwhatsappmcp"
# A helper compiled for a newer OS must never hide behind the app's older plist.
xcrun clang -mmacosx-version-min=14.0 "$scratch/fixture.c" -o "$app/Contents/Helpers/wacli"
sign_fixture
expect_failure 'requires macOS 14.0, newer than declared 13.0'
cp "$scratch/fixture" "$app/Contents/Helpers/wacli"
/usr/libexec/PlistBuddy -c 'Add :CFBundleURLTypes array' "$app/Contents/Info.plist"
sign_fixture
expect_failure 'Local-only bundle must not register callback URL schemes'
cp "$root/packaging/Info.plist" "$app/Contents/Info.plist"
case $(uname -m) in arm64) other=x86_64 ;; *) other=arm64 ;; esac
xcrun clang -arch "$other" -mmacosx-version-min=13.0 "$scratch/fixture.c" -o "$app/Contents/Helpers/wacli"
sign_fixture
expect_failure 'Architecture mismatch: Contents/Helpers/wacli'
printf 'int packaging_fixture(void) { return 0; }\n' >"$scratch/library.c"
xcrun clang -dynamiclib -mmacosx-version-min=13.0 "$scratch/library.c" -install_name "$scratch/local-only.dylib" -o "$scratch/local-only.dylib"
printf 'int packaging_fixture(void); int main(void) { return packaging_fixture(); }\n' >"$scratch/linked.c"
xcrun clang -mmacosx-version-min=13.0 "$scratch/linked.c" "$scratch/local-only.dylib" -o "$app/Contents/Helpers/wacli"
sign_fixture
expect_failure 'Non-system dynamic library dependency: Contents/Helpers/wacli'
printf 'Packaging checks passed.\n'

#!/bin/sh
set -eu
app=${1:?Usage: verify-bundle.sh APP_PATH}
fail() { printf '%s\n' "$*" >&2; exit 1; }
for relative in Contents/MacOS/LocalWhatsAppMCP Contents/Helpers/localwhatsappmcp Contents/Helpers/wacli; do
  [ -x "$app/$relative" ] || fail "Missing bundled executable: $relative"
done
[ -f "$app/Contents/Info.plist" ] || fail 'Missing bundle metadata: Contents/Info.plist'
/usr/bin/plutil -lint "$app/Contents/Info.plist" >/dev/null || fail 'Invalid bundle metadata'
minimum=$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$app/Contents/Info.plist")
if /usr/libexec/PlistBuddy -c 'Print :CFBundleURLTypes' "$app/Contents/Info.plist" >/dev/null 2>&1; then
  fail 'Local-only bundle must not register callback URL schemes'
fi
for resource in WhatsAppLogo.png AssetCredits.txt; do
  relative="Contents/Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle/$resource"
  [ -s "$app/$relative" ] || fail "Missing bundled resource: $relative"
done
architectures=$(xcrun lipo -archs "$app/Contents/MacOS/LocalWhatsAppMCP")
for relative in Contents/MacOS/LocalWhatsAppMCP Contents/Helpers/localwhatsappmcp Contents/Helpers/wacli; do
  binary="$app/$relative"
  [ "$(xcrun lipo -archs "$binary")" = "$architectures" ] || fail "Architecture mismatch: $relative"
  targets=$(xcrun vtool -show-build "$binary" | awk '$1 == "cmd" { legacy = ($2 == "LC_VERSION_MIN_MACOSX") } $1 == "minos" || (legacy && $1 == "version") { print $2 }')
  [ -n "$targets" ] || fail "Missing Mach-O deployment target: $relative"
  for target in $targets; do
    if ! awk -v actual="$target" -v declared="$minimum" 'BEGIN { split(actual,a,"."); split(declared,d,"."); for (i=1;i<=3;i++) { if (a[i]+0 < d[i]+0) exit 0; if (a[i]+0 > d[i]+0) exit 1 } }'; then
      fail "$relative requires macOS $target, newer than declared $minimum"
    fi
  done
  dependencies=$(xcrun otool -L "$binary" | awk '/^[[:space:]]/ { print $1 }')
  for dependency in $dependencies; do
    case "$dependency" in /usr/lib/*|/System/Library/*|@rpath/libswift*.dylib) ;; *) fail "Non-system dynamic library dependency: $relative ($dependency)" ;; esac
  done
done
if xcrun nm -u "$app/Contents/Helpers/localwhatsappmcp" | grep -E '_SecItem(Add|CopyMatching|Update|Delete)$' >/dev/null; then
  fail 'App helper must use local SQLite credentials, without Keychain access'
fi
/usr/bin/codesign --verify --deep --strict "$app" 2>/dev/null || fail 'Bundle signature verification failed'
for helper in localwhatsappmcp wacli; do
  /usr/bin/codesign --verify --strict "$app/Contents/Helpers/$helper" 2>/dev/null || fail "Helper signature verification failed: $helper"
done
printf 'Verified %s (%s; macOS %s or newer).\n' "$app" "$architectures" "$minimum"

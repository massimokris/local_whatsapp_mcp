#!/bin/sh
# Build the complete application. No account data or client credentials are required.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ "$(uname -s)" = Darwin ] || { printf 'Build requires macOS and Xcode command line tools.\n' >&2; exit 1; }
architecture=${ARCH:-$(uname -m)}
case "$architecture" in arm64) go_arch=arm64 ;; x86_64) go_arch=amd64 ;; *) printf 'Unsupported architecture: %s\n' "$architecture" >&2; exit 1 ;; esac
version=${APP_VERSION:-1.0.0}
build_number=${APP_BUILD:-1}
for value in "$version" "$build_number"; do
  printf '%s\n' "$value" | grep -Eq '^[0-9]+(\.[0-9]+){0,2}$' || { printf 'APP_VERSION and APP_BUILD must contain one to three numeric version components.\n' >&2; exit 1; }
done
minimum=${MACOSX_DEPLOYMENT_TARGET:-13.0}
case "$minimum" in 13.0) ;; *) printf 'This release is verified for a macOS 13.0 deployment target; remove the MACOSX_DEPLOYMENT_TARGET override.\n' >&2; exit 1 ;; esac
SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
MACOSX_DEPLOYMENT_TARGET=$minimum
GOOS=darwin
GOARCH=$go_arch
CGO_ENABLED=1
CGO_CFLAGS="-arch $architecture -mmacosx-version-min=$minimum ${CGO_CFLAGS:-}"
CGO_LDFLAGS="-arch $architecture -mmacosx-version-min=$minimum ${CGO_LDFLAGS:-}"
export SDKROOT MACOSX_DEPLOYMENT_TARGET GOOS GOARCH CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS
for command in go swift xcrun codesign iconutil ditto; do
  command -v "$command" >/dev/null || { printf 'Missing build tool: %s\n' "$command" >&2; exit 1; }
done
mkdir -p "$root/build" "$root/dist"
stage=$(mktemp -d "$root/build/app-stage.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
app="$stage/LocalWhatsAppMCP.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers" "$app/Contents/Resources"
printf 'Building native app (%s, macOS %s)...\n' "$architecture" "$minimum"
swift build --package-path "$root/native" -c release --arch "$architecture"
native_bin=$(swift build --package-path "$root/native" -c release --arch "$architecture" --show-bin-path)
cp "$native_bin/LocalWhatsAppMCP" "$app/Contents/MacOS/LocalWhatsAppMCP"
ditto "$native_bin/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle" "$app/Contents/Resources/LocalWhatsAppMCP_LocalWhatsAppMCP.bundle"
printf 'Building local service...\n'
(cd "$root/backend" && go build -trimpath -tags sqlite_fts5 -ldflags '-s -w' -o "$app/Contents/Helpers/localwhatsappmcp" ./cmd/localwhatsappmcp)
printf 'Building pinned WhatsApp engine...\n'
"$root/vendor/wacli/build.sh" "$app/Contents/Helpers/wacli"
cp "$root/packaging/Info.plist" "$app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" "$app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $build_number" "$app/Contents/Info.plist"
swift "$root/packaging/make-icon.swift" "$stage/AppIcon.iconset"
iconutil -c icns "$stage/AppIcon.iconset" -o "$app/Contents/Resources/AppIcon.icns"
cp "$root/vendor/wacli/LICENSE.upstream" "$app/Contents/Resources/wacli-LICENSE.txt"
GOARCH=$(go env GOHOSTARCH) go run "$root/packaging/notices.go" "$root/backend" "$root/vendor/wacli/.build/pinned-source" >"$app/Contents/Resources/ThirdPartyNotices.txt"
{
  printf 'LocalWhatsAppMCP %s (%s)\nArchitecture: %s\nMinimum macOS: %s\n' "$version" "$build_number" "$architecture" "$minimum"
  printf 'Source commit: '; git -C "$root" rev-parse HEAD
  printf 'Source tree: '; if [ -z "$(git -C "$root" status --porcelain)" ]; then printf 'clean\n'; else printf 'modified\n'; fi
  printf 'Engine upstream: '; cat "$root/vendor/wacli/UPSTREAM_REVISION"
  printf 'Engine patch SHA256: '; shasum -a 256 "$root/vendor/wacli/content-policy.patch" | awk '{ print $1 }'
  printf 'Delivery dependency: '; (cd "$root/vendor/wacli/.build/pinned-source" && go list -m -f '{{.Path}} {{.Version}}' go.mau.fi/whatsmeow)
  printf 'Delivery patch SHA256: '; shasum -a 256 "$root/vendor/wacli/whatsmeow-delivery.patch" | awk '{ print $1 }'
  go version
  swift --version 2>&1
  printf 'SDK version: '; xcrun --sdk macosx --show-sdk-version
} >"$app/Contents/Resources/BuildInfo.txt"
chmod 755 "$app/Contents/MacOS/LocalWhatsAppMCP" "$app/Contents/Helpers/localwhatsappmcp" "$app/Contents/Helpers/wacli"
/usr/bin/xattr -cr "$app"
identity=${SIGNING_IDENTITY:--}
sign() {
  if [ "$identity" = - ]; then
    codesign --force --sign - "$1"
  else
    codesign --force --options runtime --timestamp --sign "$identity" "$1"
  fi
}
sign "$app/Contents/Helpers/localwhatsappmcp"
sign "$app/Contents/Helpers/wacli"
sign "$app"
"$root/scripts/verify-bundle.sh" "$app"
rm -rf "$root/dist/LocalWhatsAppMCP.app"
mv "$app" "$root/dist/LocalWhatsAppMCP.app"
printf 'Built %s/dist/LocalWhatsAppMCP.app\n' "$root"

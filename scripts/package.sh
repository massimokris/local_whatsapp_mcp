#!/bin/sh
# Optional release signing is configured through SIGNING_IDENTITY and NOTARY_PROFILE.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "${1:-}" in '') "$root/scripts/build.sh" ;; --no-build) ;; *) printf 'Usage: package.sh [--no-build]\n' >&2; exit 1 ;; esac
app="$root/dist/LocalWhatsAppMCP.app"
"$root/scripts/verify-bundle.sh" "$app"
version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app/Contents/Info.plist")
architecture=$(xcrun lipo -archs "$app/Contents/MacOS/LocalWhatsAppMCP" | tr ' ' '-')
name="LocalWhatsAppMCP-$version-$architecture"
stage=$(mktemp -d "$root/build/package-stage.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
if [ -n "${NOTARY_PROFILE:-}" ]; then
  codesign -dv --verbose=4 "$app" 2>&1 | grep -q 'Authority=Developer ID Application:' || { printf 'Notarization requires a Developer ID Application signed app.\n' >&2; exit 1; }
  ditto -c -k --keepParent "$app" "$stage/notarize.zip"
  xcrun notarytool submit "$stage/notarize.zip" --keychain-profile "$NOTARY_PROFILE" --no-s3-acceleration --wait
  xcrun stapler staple "$app"
  xcrun stapler validate "$app"
fi
mkdir "$stage/image"
ditto "$app" "$stage/image/LocalWhatsAppMCP.app"
ln -s /Applications "$stage/image/Applications"
hdiutil create -volname LocalWhatsAppMCP -srcfolder "$stage/image" -ov -format UDZO "$stage/$name.dmg"
if [ -n "${SIGNING_IDENTITY:-}" ] && [ "$SIGNING_IDENTITY" != - ]; then
  codesign --sign "$SIGNING_IDENTITY" --timestamp "$stage/$name.dmg"
fi
if [ -n "${NOTARY_PROFILE:-}" ]; then
  xcrun notarytool submit "$stage/$name.dmg" --keychain-profile "$NOTARY_PROFILE" --no-s3-acceleration --wait
  xcrun stapler staple "$stage/$name.dmg"
  xcrun stapler validate "$stage/$name.dmg"
fi
hdiutil verify "$stage/$name.dmg"
ditto -c -k --sequesterRsrc --keepParent "$app" "$stage/$name.zip"
mv -f "$stage/$name.dmg" "$root/dist/$name.dmg"
mv -f "$stage/$name.zip" "$root/dist/$name.zip"
(cd "$root/dist" && shasum -a 256 "$name.dmg" "$name.zip" >"$name.sha256")
printf 'Packaged %s/dist/%s.dmg\n' "$root" "$name"

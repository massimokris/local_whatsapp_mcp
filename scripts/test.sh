#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
export SDKROOT
export MACOSX_DEPLOYMENT_TARGET=13.0
swift test --package-path "$root/native"
(cd "$root/backend" && go test -race ./... && go vet ./...)
"$root/vendor/wacli/test.sh"
"$root/scripts/test-packaging.sh"

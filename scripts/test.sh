#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
export SDKROOT
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_ENABLED=1
export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=13.0"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=13.0"
swift test --package-path "$root/native"
(cd "$root/backend" && go test -race ./... && go vet ./...)
"$root/vendor/wacli/test.sh"
"$root/scripts/test-packaging.sh"

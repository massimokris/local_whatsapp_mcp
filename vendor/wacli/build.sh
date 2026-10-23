#!/bin/sh
set -eu
vendor_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
output=${1:-"$vendor_dir/.build/wacli"}
mkdir -p "$(dirname -- "$output")"
output_dir=$(CDPATH= cd -- "$(dirname -- "$output")" && pwd)
output="$output_dir/$(basename -- "$output")"
source_dir=$("$vendor_dir/prepare.sh")
cd "$source_dir"
build_cflags="${CGO_CFLAGS:-} -Wno-error=missing-braces"
build_ldflags="${CGO_LDFLAGS:-}"
if [ "$(go env GOOS)" = darwin ]; then
  deployment_target=${MACOSX_DEPLOYMENT_TARGET:-13.0}
  build_cflags="$build_cflags -mmacosx-version-min=$deployment_target"
  build_ldflags="$build_ldflags -mmacosx-version-min=$deployment_target"
fi
CGO_ENABLED=1 CGO_CFLAGS="$build_cflags" CGO_LDFLAGS="$build_ldflags" go build -trimpath -tags sqlite_fts5 -ldflags '-s -w -X main.version=0.24.0-localwhatsappmcp.1' -o "$output" ./cmd/wacli
printf '%s\n' "$output"

#!/bin/sh
# Prepare a disposable pinned source tree. This directory never contains runtime data.
set -eu
vendor_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
revision=$(cat "$vendor_dir/UPSTREAM_REVISION")
source_dir="$vendor_dir/.build/pinned-source"
mkdir -p "$vendor_dir/.build"
if [ ! -d "$source_dir/.git" ]; then
  git init --quiet "$source_dir"
  git -C "$source_dir" remote add origin https://github.com/openclaw/wacli.git
fi
if ! git -C "$source_dir" cat-file -e "$revision^{commit}" 2>/dev/null; then
  git -C "$source_dir" fetch --quiet --depth=1 origin "$revision"
fi
# Only the generated source checkout is reset. No app data lives under .build.
git -C "$source_dir" checkout --quiet --detach --force "$revision"
git -C "$source_dir" clean -ffd --quiet
git -C "$source_dir" apply --check "$vendor_dir/content-policy.patch"
git -C "$source_dir" apply "$vendor_dir/content-policy.patch"
"$vendor_dir/prepare-whatsmeow.sh" "$source_dir"
printf '%s\n' "$source_dir"

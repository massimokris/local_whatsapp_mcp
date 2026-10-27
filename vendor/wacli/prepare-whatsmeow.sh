#!/bin/sh
# Copy only the exact pinned Go dependency; never mutate the shared module cache.
set -eu
vendor_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
source_dir=$1
cd "$source_dir"
expected_version=v0.0.0-20261017175727-b2b1e71d45e2
actual_version=$(go list -m -f '{{.Version}}' go.mau.fi/whatsmeow)
if [ "$actual_version" != "$expected_version" ]; then
  echo "unexpected whatsmeow dependency revision" >&2
  exit 1
fi
go mod download "go.mau.fi/whatsmeow@$expected_version"
dependency_cache=$(go env GOMODCACHE)/go.mau.fi/whatsmeow@$expected_version
dependency_source="$source_dir/.local-whatsmeow"
mkdir "$dependency_source"
cp -R "$dependency_cache/." "$dependency_source/"
chmod -R u+w "$dependency_source"
# A temporary repository makes patch path handling independent of the parent's
# Git root and checks every hunk before modifying this disposable dependency.
git -C "$dependency_source" init --quiet
git -C "$dependency_source" apply --check "$vendor_dir/whatsmeow-delivery.patch"
git -C "$dependency_source" apply "$vendor_dir/whatsmeow-delivery.patch"
go mod edit -replace=go.mau.fi/whatsmeow=./.local-whatsmeow

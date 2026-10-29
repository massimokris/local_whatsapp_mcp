#!/bin/sh
set -eu
vendor_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
source_dir=$("$vendor_dir/prepare.sh")
cd "$source_dir"
# Upstream maintains separate SQLite and FTS5 paths. Exercise both, including
# the policy fixtures at the public app/CLI integration seams.
go test ./...
go test -tags sqlite_fts5 ./...
go vet -tags sqlite_fts5 ./...
git diff --check

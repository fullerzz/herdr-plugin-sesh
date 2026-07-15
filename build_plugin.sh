#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
version=$(awk -F'"' '
  /^version = / { print $2; exit }
' "$repo_root/herdr-plugin.toml")
if [ -z "$version" ]; then
  echo 'Could not resolve version from herdr-plugin.toml' >&2
  exit 1
fi

mkdir -p "$repo_root/bin"
cd "$repo_root"
exec go build \
  -ldflags "-X github.com/fullerzz/herdr-plugin-sesh/internal/app.Version=$version" \
  -o bin/herdr-sesh \
  ./cmd/herdr-sesh

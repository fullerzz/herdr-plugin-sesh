#!/usr/bin/env bash

set -euo pipefail

ref=${1:-}
case "$ref" in
  refs/tags/v*) ;;
  *) echo "Release ref must be an explicit refs/tags/v* ref, got: ${ref:-<empty>}" >&2; exit 1 ;;
esac
tag=${ref#refs/tags/}
if [[ ! "$tag" =~ ^v[0-9A-Za-z][0-9A-Za-z._+-]*$ ]]; then
  echo "Release tag must match project-safe syntax: $tag" >&2
  exit 1
fi
if ! git check-ref-format "$ref" >/dev/null; then
  echo "Invalid release tag ref: $ref" >&2
  exit 1
fi

if ! git show-ref --verify --quiet "$ref"; then
  echo "Requested release tag is missing: $ref" >&2
  exit 1
fi
if ! commit=$(git rev-parse --verify "${ref}^{commit}" 2>/dev/null); then
  echo "Requested release tag does not peel to a commit: $ref" >&2
  exit 1
fi

git checkout --detach --quiet "$commit"
head=$(git rev-parse HEAD)
if [ "$head" != "$commit" ]; then
  echo "Checked-out HEAD $head does not match requested tag commit $commit" >&2
  exit 1
fi

printf '%s\n' "$commit"

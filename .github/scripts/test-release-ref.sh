#!/usr/bin/env bash

set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
workflow="$repo_root/.github/workflows/release.yml"
helper="$repo_root/.github/scripts/checkout-release-ref.sh"

# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'bash .github/scripts/checkout-release-ref.sh "$RELEASE_REF"' "$workflow"; then
  echo 'release workflow must resolve the explicit tag ref through checkout-release-ref.sh' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'if [ "$remote_commit" != "$EXPECTED_COMMIT" ]; then' "$workflow"; then
  echo 'publish must reject a tag that moved after build verification' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match the workflow's literal GitHub expression.
if ! grep -Fq 'GH_REPO: ${{ github.repository }}' "$workflow"; then
  echo 'publish must provide repository context to gh release commands' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'if [[ ! "$tag" =~ ^v[0-9A-Za-z][0-9A-Za-z._+-]*$ ]]; then' "$workflow"; then
  echo 'release workflow must reject tags outside the project-safe syntax' >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git -C "$tmp" init -q
git -C "$tmp" config user.email test@example.com
git -C "$tmp" config user.name 'Release workflow test'

printf 'tag\n' >"$tmp/version"
git -C "$tmp" add version
git -C "$tmp" commit -qm 'tag commit'
tag_commit=$(git -C "$tmp" rev-parse HEAD)
git -C "$tmp" tag -am 'annotated release tag' v1.2.3
git -C "$tmp" tag v1.2.4

git -C "$tmp" switch -qc v1.2.3
printf 'branch\n' >"$tmp/version"
git -C "$tmp" commit -qam 'same-named branch commit'
branch_commit=$(git -C "$tmp" rev-parse HEAD)

resolved=$(cd "$tmp" && bash "$helper" refs/tags/v1.2.3)
head=$(git -C "$tmp" rev-parse HEAD)

if [ "$resolved" != "$tag_commit" ] || [ "$head" != "$tag_commit" ]; then
  echo "expected explicit tag commit $tag_commit, resolved $resolved with HEAD $head" >&2
  exit 1
fi
if [ "$resolved" = "$branch_commit" ]; then
  echo "resolved same-named branch commit $branch_commit instead of tag" >&2
  exit 1
fi

lightweight=$(cd "$tmp" && bash "$helper" refs/tags/v1.2.4)
if [ "$lightweight" != "$tag_commit" ]; then
  echo "expected lightweight tag commit $tag_commit, resolved $lightweight" >&2
  exit 1
fi

if missing=$(cd "$tmp" && bash "$helper" refs/tags/v9.9.9 2>&1); then
  echo 'expected a missing release tag to fail' >&2
  exit 1
fi
case "$missing" in
  *'Requested release tag is missing: refs/tags/v9.9.9'*) ;;
  *) echo "missing release tag failed unclearly: $missing" >&2; exit 1 ;;
esac

for unsafe_tag in 'v#probe' 'v%2Fprobe' 'v/path' 'v?probe'; do
  if unsafe=$(cd "$tmp" && bash "$helper" "refs/tags/$unsafe_tag" 2>&1); then
    echo "expected unsafe release tag $unsafe_tag to fail" >&2
    exit 1
  fi
  case "$unsafe" in
    *"Release tag must match project-safe syntax: $unsafe_tag"*) ;;
    *) echo "unsafe release tag $unsafe_tag failed unclearly: $unsafe" >&2; exit 1 ;;
  esac
done

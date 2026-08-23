#!/usr/bin/env bash

set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
workflow="$repo_root/.github/workflows/release.yml"
changelog_workflow="$repo_root/.github/workflows/changelog.yml"
helper="$repo_root/.github/scripts/checkout-release-ref.sh"
justfile="$repo_root/justfile"

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
# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'test "$("$binary" --version)" = "herdr-sesh ${version}"' "$workflow"; then
  echo 'release workflow must verify the packaged binary version' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'test "$packaged_version" = "$version"' "$workflow"; then
  echo 'release workflow must verify the packaged manifest version' >&2
  exit 1
fi
if ! grep -Fq 'manifest_build_flag="-ldflags=-X=github.com/fullerzz/herdr-plugin-sesh/internal/app.Version=${version}"' "$workflow" ||
  ! grep -Fq 'if ! grep -Fq "\"${manifest_build_flag}\"," herdr-plugin.toml; then' "$workflow"; then
  echo 'release workflow must verify the manifest build version' >&2
  exit 1
fi
if ! grep -Fq 'cp README.md LICENSE herdr-plugin.toml "dist/work/${name}/"' "$workflow"; then
  echo 'release archives must include the plugin manifest' >&2
  exit 1
fi
if ! grep -Fq 'echo "herdr plugin install fullerzz/herdr-plugin-sesh --ref ${tag}"' "$workflow"; then
  echo 'generated install instructions must pin the release tag' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match the workflow's literal shell variables.
if ! grep -Fq 'if [ "$manifest_version" != "$version" ]; then' "$changelog_workflow"; then
  echo 'changelog workflow must reject tags that do not match the manifest version' >&2
  exit 1
fi
if ! grep -Fq '  pull-requests: read' "$changelog_workflow"; then
  echo 'changelog workflow must allow git-cliff to read pull request metadata' >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fake_bin="$tmp/bin"
mkdir -p "$fake_bin"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\n" "$*"' >"$fake_bin/herdr"
chmod +x "$fake_bin/herdr"
manifest_version=$(awk -F'"' '
  /^version = / { print $2; exit }
' "$repo_root/herdr-plugin.toml")
manifest_build_flag="-ldflags=-X=github.com/fullerzz/herdr-plugin-sesh/internal/app.Version=${manifest_version}"
if ! grep -Fq '  "go",' "$repo_root/herdr-plugin.toml" ||
  ! grep -Fq "  \"${manifest_build_flag}\"," "$repo_root/herdr-plugin.toml"; then
  echo 'manifest build must run go build with the manifest version' >&2
  exit 1
fi
(cd "$repo_root" && go build "$manifest_build_flag" -o bin/ ./cmd/herdr-sesh)
if [ "$("$repo_root/bin/herdr-sesh" --version)" != "herdr-sesh ${manifest_version}" ]; then
  echo 'manifest build must inject the manifest version into the binary' >&2
  exit 1
fi
install_command=$(cd "$tmp" && PATH="$fake_bin:$PATH" "$repo_root/install_plugin.sh")
expected_install="plugin install fullerzz/herdr-plugin-sesh --ref v${manifest_version} --yes"
if [ "$install_command" != "$expected_install" ]; then
  echo "installer must pin the manifest release: got $install_command" >&2
  exit 1
fi

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

just_bin=$(mise which just)
git_cliff_bin=$(mise which git-cliff)

"$git_cliff_bin" --context --offline >"$tmp/changelog-context.json"
sed 's/"github":{"contributors":\[\]}/"github":{"contributors":[{"username":"first-timer","pr_title":"First contribution","pr_number":79,"pr_labels":[],"is_first_time":true},{"username":"direct-contributor","pr_title":null,"pr_number":null,"pr_labels":[],"is_first_time":true}]}/g' \
  "$tmp/changelog-context.json" >"$tmp/changelog-context-with-contributors.json"
"$git_cliff_bin" --from-context "$tmp/changelog-context-with-contributors.json" \
  --output "$tmp/rendered-changelog.md"
if grep -Eq '.## v[0-9]' "$tmp/rendered-changelog.md"; then
  echo 'first-time contributor entries must preserve the next release heading newline' >&2
  exit 1
fi
if ! grep -Fxq '* @direct-contributor made their first contribution' "$tmp/rendered-changelog.md"; then
  echo 'first-time contributor entries without a pull request must omit the PR suffix' >&2
  exit 1
fi

release_tools="$tmp/release-tools"
mkdir -p "$release_tools"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$release_tools/just"
# shellcheck disable=SC2016 # Write literal shell variables into the fake mise script.
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [ "$1" != exec ] || [ "$2" != -- ] || [ "$3" != git-cliff ]; then' \
  '  echo "unexpected mise invocation: $*" >&2' \
  '  exit 1' \
  'fi' \
  'shift 3' \
  'exec "$RELEASE_TEST_GIT_CLIFF" "$@"' \
  >"$release_tools/mise"
chmod +x "$release_tools/just" "$release_tools/mise"

if preview_error=$(
  cd "$repo_root"
  env -u GITHUB_TOKEN \
    GIT_CLIFF_OFFLINE=true \
    PATH="$release_tools:$PATH" \
    RELEASE_TEST_GIT_CLIFF="$git_cliff_bin" \
    "$just_bin" preview-changelog 2>&1
); then
  echo 'changelog preview must require an authenticated GitHub metadata request' >&2
  exit 1
fi
case "$preview_error" in
  *'GITHUB_TOKEN is required for GitHub changelog metadata'*) ;;
  *) echo "missing changelog preview token failed unclearly: $preview_error" >&2; exit 1 ;;
esac

setup_release_repo() {
  release_repo=$1
  release_remote=$2
  git init -q -b main "$release_repo"
  git init -q --bare "$release_remote"
  git -C "$release_repo" config user.email test@example.com
  git -C "$release_repo" config user.name 'Release recipe test'
  git -C "$release_repo" config commit.gpgsign false
  cp "$justfile" "$repo_root/cliff.toml" "$release_repo/"
  mkdir -p "$release_repo/bin"
  printf 'version = "1.2.3"\n' >"$release_repo/herdr-plugin.toml"
  printf '## v1.2.2 (2026-08-20)\n' >"$release_repo/CHANGELOG.md"
  printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$release_repo/bin/herdr-sesh"
  chmod +x "$release_repo/bin/herdr-sesh"
  git -C "$release_repo" add .
  git -C "$release_repo" commit -qm 'chore(release): release v1.2.3'
  git -C "$release_repo" remote add origin "$release_remote"
}

run_release_recipe() {
  release_repo=$1
  (
    cd "$release_repo"
    GITHUB_TOKEN=test-token \
      GIT_CLIFF_OFFLINE=true \
      PATH="$release_tools:$PATH" \
      RELEASE_TEST_GIT_CLIFF="$git_cliff_bin" \
      "$just_bin" --yes release v1.2.3
  )
}

missing_token_repo="$tmp/release-missing-token"
missing_token_remote="$tmp/release-missing-token.git"
setup_release_repo "$missing_token_repo" "$missing_token_remote"
if missing_token_error=$(
  cd "$missing_token_repo"
  env -u GITHUB_TOKEN \
    GIT_CLIFF_OFFLINE=true \
    PATH="$release_tools:$PATH" \
    RELEASE_TEST_GIT_CLIFF="$git_cliff_bin" \
    "$just_bin" --yes release v1.2.3 2>&1
); then
  echo 'release must require an authenticated GitHub metadata request' >&2
  exit 1
fi
case "$missing_token_error" in
  *'GITHUB_TOKEN is required for GitHub changelog metadata'*) ;;
  *) echo "missing release token failed unclearly: $missing_token_error" >&2; exit 1 ;;
esac

success_repo="$tmp/release-success"
success_remote="$tmp/release-success.git"
setup_release_repo "$success_repo" "$success_remote"
run_release_recipe "$success_repo"
success_head=$(git -C "$success_repo" rev-parse HEAD)
success_parent=$(git -C "$success_repo" rev-parse HEAD^)
success_tag=$(git -C "$success_repo" rev-parse 'refs/tags/v1.2.3^{commit}')
remote_head=$(git --git-dir="$success_remote" rev-parse refs/heads/main)
remote_tag=$(git --git-dir="$success_remote" rev-parse 'refs/tags/v1.2.3^{commit}')
if [ "$success_tag" != "$success_parent" ] ||
  [ "$remote_head" != "$success_head" ] ||
  [ "$remote_tag" != "$success_tag" ]; then
  echo 'release recipe must atomically push the changelog commit and its tagged parent' >&2
  exit 1
fi
if [ "$(git -C "$success_repo" diff-tree --no-commit-id --name-only -r HEAD)" != CHANGELOG.md ]; then
  echo 'release recipe must commit only CHANGELOG.md' >&2
  exit 1
fi
(
  cd "$success_repo"
  "$git_cliff_bin" --output "$tmp/post-tag-CHANGELOG.md"
)
if ! cmp -s "$success_repo/CHANGELOG.md" "$tmp/post-tag-CHANGELOG.md"; then
  echo 'tag-triggered changelog generation must be a no-op on main' >&2
  exit 1
fi

branch_repo="$tmp/release-branch"
branch_remote="$tmp/release-branch.git"
setup_release_repo "$branch_repo" "$branch_remote"
git -C "$branch_repo" switch -qc release/v1.2.3
if branch_error=$(run_release_recipe "$branch_repo" 2>&1); then
  echo 'release recipe must reject non-main branches' >&2
  exit 1
fi
case "$branch_error" in
  *'Releases must be created from main, got: release/v1.2.3'*) ;;
  *) echo "non-main release failed unclearly: $branch_error" >&2; exit 1 ;;
esac
if git -C "$branch_repo" rev-parse --verify --quiet refs/tags/v1.2.3 >/dev/null; then
  echo 'rejected non-main release must not create a tag' >&2
  exit 1
fi

failure_repo="$tmp/release-commit-failure"
failure_remote="$tmp/release-commit-failure.git"
setup_release_repo "$failure_repo" "$failure_remote"
printf '%s\n' '#!/usr/bin/env bash' 'exit 1' >"$failure_repo/.git/hooks/pre-commit"
chmod +x "$failure_repo/.git/hooks/pre-commit"
if run_release_recipe "$failure_repo" >/dev/null 2>&1; then
  echo 'release recipe must propagate changelog commit failures' >&2
  exit 1
fi
if git -C "$failure_repo" rev-parse --verify --quiet refs/tags/v1.2.3 >/dev/null; then
  echo 'failed changelog commit must remove the unpushed release tag' >&2
  exit 1
fi
if ! git -C "$failure_repo" diff --quiet -- CHANGELOG.md ||
  ! git -C "$failure_repo" diff --cached --quiet -- CHANGELOG.md; then
  echo 'failed changelog commit must restore CHANGELOG.md in the worktree and index' >&2
  exit 1
fi
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$failure_repo/.git/hooks/pre-commit"
if ! run_release_recipe "$failure_repo" >/dev/null 2>&1; then
  echo 'release recipe must be retryable after a transient commit failure' >&2
  exit 1
fi

# Plan 001: Harden release workflow shell inputs

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1a2235e..HEAD -- .github/workflows/release.yml`
> If the in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `1a2235e`, 2026-06-27

## Why this matters

The release workflow runs with `contents: write`, so shell steps should not
directly interpolate event-controlled values into script text. The current
workflow assigns `${{ inputs.tag }}` and `${{ github.ref_name }}` inside a
`run:` block before the tag validation happens. Moving those values through
step `env:` keeps the shell script static and leaves validation in one place.

## Current state

- `.github/workflows/release.yml` owns tag validation, release asset builds,
  release notes, and GitHub release publication.
- The repo uses GitHub Actions YAML directly, not generated workflow sources.
- Existing commits use Conventional Commit-style subjects such as
  `fix: pad native picker top border`.

Current excerpts:

```yaml
# .github/workflows/release.yml:56-79
- name: Resolve version
  id: meta
  shell: bash
  run: |
    set -euo pipefail
    if [ "${{ github.event_name }}" = "workflow_dispatch" ]; then
      tag='${{ inputs.tag }}'
    else
      tag='${{ github.ref_name }}'
    fi
    case "$tag" in
      v*) ;;
      *) echo "Release tag must start with v, got: $tag" >&2; exit 1 ;;
    esac
    version="${tag#v}"
```

```yaml
# .github/workflows/release.yml:81-86
- name: Build release assets
  shell: bash
  run: |
    set -euo pipefail
    version='${{ steps.meta.outputs.version }}'
    module='github.com/fullerzz/herdr-plugin-sesh/internal/app'
```

```yaml
# .github/workflows/release.yml:111-116
- name: Generate release notes
  shell: bash
  run: |
    set -euo pipefail
    tag='${{ steps.meta.outputs.tag }}'
    version='${{ steps.meta.outputs.version }}'
```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Drift check | `git diff --stat 1a2235e..HEAD -- .github/workflows/release.yml` | empty output, or reviewed drift |
| Static grep | `rg -n "tag='\\$\\{\\{|version='\\$\\{\\{|\\[ \"\\$\\{\\{" .github/workflows/release.yml` | exit 1, no matches |
| YAML diff sanity | `git diff --check -- .github/workflows/release.yml` | exit 0 |
| Repo checks | `just lint` | exit 0, `0 issues.` |
| Repo checks | `just fmt-check` | exit 0 |
| Repo tests | `just test` | exit 0, all tests pass |

## Scope

**In scope**:

- `.github/workflows/release.yml`

**Out of scope**:

- `.github/workflows/test.yml`; Plan 004 owns pull request CI.
- `justfile`, `mise.toml`, Go source files, and release artifact contents.
- Publishing a release or pushing a tag.

## Git workflow

- Branch: `cdx/001-harden-release-workflow-shell-inputs`
- Commit message: `ci: harden release workflow shell inputs`
- Do not push or open a PR unless the operator asks.

## Steps

### Step 1: Move event values into the Resolve version environment

Edit `.github/workflows/release.yml` so the `Resolve version` step passes
GitHub context values through `env:` and reads normal shell variables inside
the script.

Target shape:

```yaml
- name: Resolve version
  id: meta
  shell: bash
  env:
    EVENT_NAME: ${{ github.event_name }}
    INPUT_TAG: ${{ inputs.tag }}
    REF_NAME: ${{ github.ref_name }}
  run: |
    set -euo pipefail
    if [ "$EVENT_NAME" = "workflow_dispatch" ]; then
      tag="$INPUT_TAG"
    else
      tag="$REF_NAME"
    fi
```

Keep the existing `case "$tag"` validation and manifest-version comparison.

**Verify**: `rg -n "\\$\\{\\{ (inputs\\.tag|github\\.ref_name|github\\.event_name) \\}\\}" .github/workflows/release.yml` should show only the `env:` entries for this step, not shell assignments inside `run:`.

### Step 2: Move validated workflow outputs into step environments

In the `Build release assets` step, pass the version via `env:`:

```yaml
env:
  VERSION: ${{ steps.meta.outputs.version }}
run: |
  set -euo pipefail
  version="$VERSION"
```

In the `Generate release notes` step, pass both values via `env:`:

```yaml
env:
  TAG: ${{ steps.meta.outputs.tag }}
  VERSION: ${{ steps.meta.outputs.version }}
run: |
  set -euo pipefail
  tag="$TAG"
  version="$VERSION"
```

Do not change the `Publish GitHub release` step; it already uses `env:`.

**Verify**: `rg -n "tag='\\$\\{\\{|version='\\$\\{\\{|\\[ \"\\$\\{\\{" .github/workflows/release.yml` exits 1 with no matches.

### Step 3: Run the local checks

Run the repository checks even though this is YAML-only, because this repo's
review convention expects them before PRs.

**Verify**: `git diff --check -- .github/workflows/release.yml`, `just lint`,
`just fmt-check`, and `just test` all exit 0.

## Test plan

- No Go tests are added; this is workflow-only hardening.
- The regression guard is the static grep that ensures event/output contexts
  are no longer embedded in shell assignments.
- Existing repo verification still runs: `just lint`, `just fmt-check`,
  `just test`.

## Done criteria

- [ ] `.github/workflows/release.yml` no longer assigns `tag='${{ ... }}'` or
  `version='${{ ... }}'` inside `run:` blocks.
- [ ] The tag validation behavior remains: tags must start with `v`, and the
  manifest version must match the tag without `v`.
- [ ] `git diff --check -- .github/workflows/release.yml` exits 0.
- [ ] `just lint`, `just fmt-check`, and `just test` exit 0.
- [ ] No files outside `.github/workflows/release.yml` and `plans/README.md`
  are modified.
- [ ] `plans/README.md` status row for Plan 001 is updated.

## STOP conditions

Stop and report back if:

- `.github/workflows/release.yml` no longer has the release steps shown above.
- The fix appears to require changing release semantics or artifact names.
- A GitHub Actions linter or CI run rejects `env:` on these steps.
- Any verification command fails twice after a reasonable fix attempt.

## Maintenance notes

Reviewers should check that all untrusted GitHub context values used in shell
steps enter through `env:`. Future workflow edits should keep that convention,
especially for release workflows with write permissions.

# Plan 004: Run the local check gate in pull request CI

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1a2235e..HEAD -- .github/workflows/test.yml justfile mise.toml .golangci.yml`
> If any in-scope context file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `1a2235e`, 2026-06-27

## Why this matters

The contributor guide and `justfile` make `just check` the local validation
gate, but pull request CI currently runs a lighter, hand-written sequence.
That lets changes pass CI while failing `golangci-lint`, `goimports`, or the
race-enabled test suite. CI should use the same repo-native commands that
contributors and agents run locally.

## Current state

- `.github/workflows/test.yml` runs on `push`, `pull_request`, and manual
  dispatch.
- `justfile` defines the canonical local checks.
- `mise.toml` pins the shared toolchain: Go, `just`, `golangci-lint`,
  `gotestsum`, and external picker/preview tools.

Current excerpts:

```yaml
# .github/workflows/test.yml:27-46
- name: Download modules
  run: go mod download

- name: Check formatting
  run: |
    files=$(gofmt -l .)
    if [ -n "$files" ]; then
      echo "gofmt required for:"
      echo "$files"
      exit 1
    fi

- name: Vet
  run: go vet ./...

- name: Test
  run: go test ./...

- name: Build
  run: go build -o bin/herdr-sesh ./cmd/herdr-sesh
```

```make
# justfile:23-45
lint:
    mise exec -- golangci-lint run ./...

fmt-check:
    mise exec -- golangci-lint fmt --diff ./...

test:
    gotestsum --format-icons=octicons --format=pkgname -- -race ./...

check: lint fmt-check test
```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Drift check | `git diff --stat 1a2235e..HEAD -- .github/workflows/test.yml justfile mise.toml .golangci.yml` | empty output, or reviewed drift |
| Local gate | `just check` | exit 0 |
| Build | `just build` | exit 0, `bin/herdr-sesh` exists |
| Smoke | `./bin/herdr-sesh --version` | exit 0, prints version |
| Smoke | `./bin/herdr-sesh list --json --config testdata/sesh.toml` | exit 0, JSON includes `sesh` |
| Diff sanity | `git diff --check -- .github/workflows/test.yml` | exit 0 |

## Scope

**In scope**:

- `.github/workflows/test.yml`

**Read-only context**:

- `justfile`
- `mise.toml`
- `.golangci.yml`

**Out of scope**:

- `.github/workflows/release.yml`; Plan 001 owns release workflow changes.
- Changing `justfile`, `mise.toml`, or `.golangci.yml`.
- Adding new lint tools or CI-only commands.

## Git workflow

- Branch: `cdx/004-run-local-check-gate-in-pull-request-ci`
- Commit message: `ci: run local check gate in pull request workflow`
- Do not push or open a PR unless the operator asks.

## Steps

### Step 1: Install the repo toolchain in CI with mise

Update `.github/workflows/test.yml` so the job installs the `mise.toml`
toolchain before running `just`.

Use a maintained mise setup action. Target shape:

```yaml
- name: Set up mise
  uses: jdx/mise-action@v2
  with:
    install: true
    cache: true
```

Place it after checkout and before any `just` invocation. Keep
`actions/setup-go@v5` only if the workflow still needs it for Go module caching;
otherwise prefer mise as the source of tool versions.

**Verify**: `git diff -- .github/workflows/test.yml` shows a mise setup step
before checks run.

### Step 2: Replace hand-written format/vet/test steps with `just check`

Replace the separate `go mod download`, `gofmt -l`, `go vet`, and `go test`
steps with a single step:

```yaml
- name: Check
  run: just check
```

Keep build and CLI smoke checks as separate steps after `just check`, because
`just check` does not build the binary or run CLI smoke tests.

**Verify**: `rg -n "gofmt -l|go vet ./\\.\\.\\.|go test ./\\.\\.\\." .github/workflows/test.yml` exits 1.

### Step 3: Keep build and smoke checks repo-native

Prefer `just build` over raw `go build` so CI follows the repository recipe.
Keep the existing smoke commands:

```yaml
- name: Build
  run: just build

- name: CLI smoke test
  run: |
    ./bin/herdr-sesh --version
    ./bin/herdr-sesh list --json \
      --config testdata/sesh.toml \
      >/tmp/herdr-sesh-list.json
    test -s /tmp/herdr-sesh-list.json
```

**Verify**: `rg -n "just check|just build|herdr-sesh list --json" .github/workflows/test.yml` shows all three.

### Step 4: Run local verification

Run the same commands locally that CI should now run.

**Verify**: `just check`, `just build`,
`./bin/herdr-sesh --version`, and
`./bin/herdr-sesh list --json --config testdata/sesh.toml` all exit 0.

## Test plan

- No Go tests are added; this is workflow-only.
- Local verification must run the exact commands CI now delegates to:
  `just check`, `just build`, and CLI smoke checks.
- A GitHub Actions run is the final confirmation after pushing; do not fake it
  locally.

## Done criteria

- [ ] `.github/workflows/test.yml` installs the `mise.toml` toolchain.
- [ ] Pull request CI runs `just check`.
- [ ] Pull request CI still builds `bin/herdr-sesh` and runs the two CLI smoke
  checks.
- [ ] `rg -n "gofmt -l|go vet ./\\.\\.\\.|go test ./\\.\\.\\." .github/workflows/test.yml` exits 1.
- [ ] `just check`, `just build`, and both smoke commands exit 0 locally.
- [ ] `git diff --check -- .github/workflows/test.yml` exits 0.
- [ ] No files outside `.github/workflows/test.yml` and `plans/README.md` are
  modified.
- [ ] `plans/README.md` status row for Plan 004 is updated.

## STOP conditions

Stop and report back if:

- The repo has switched away from `mise` or `just` since this plan was written.
- `jdx/mise-action@v2` is unavailable or rejected by the workflow environment.
- Running `just check` in CI requires secrets, network services, or live Herdr.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

When the local validation gate changes, update CI in the same PR. Reviewers
should treat hand-written CI command lists as drift-prone unless there is a
clear reason they cannot call `just`.

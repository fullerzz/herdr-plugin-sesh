# Plan 002: Merge nested config tables field by field

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1a2235e..HEAD -- internal/config/load.go internal/config/load_test.go`
> If either in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `1a2235e`, 2026-06-27

## Why this matters

Config imports are intended to compose Sesh-style TOML files. Top-level slices
already merge, but nested tables currently replace the whole struct when any
field is set. That means a small local override like `[tui] placeholder = "X"`
can silently discard an imported prompt, and a partial `[default_session]`
override can discard imported windows or startup settings.

## Current state

- `internal/config/load.go` loads imports first, then merges the current file
  over the imported config.
- `internal/config/load_test.go` already has an import-order test; add the new
  regression near that test.
- Keep the package style simple: standard `testing`, `t.TempDir()`,
  `mustWrite`, and direct field assertions.

Current excerpts:

```go
// internal/config/load.go:112-143
func merge(dst *Config, src Config) {
    if src.Cache {
        dst.Cache = true
    }
    ...
    if src.TUI != (TUIConfig{}) {
        dst.TUI = src.TUI
    }
    if !reflect.DeepEqual(src.DefaultSessionConfig, DefaultSessionConfig{}) {
        dst.DefaultSessionConfig = src.DefaultSessionConfig
    }
    dst.SessionConfigs = append(dst.SessionConfigs, src.SessionConfigs...)
    dst.WindowConfigs = append(dst.WindowConfigs, src.WindowConfigs...)
    dst.WildcardConfigs = append(dst.WildcardConfigs, src.WildcardConfigs...)
}
```

```go
// internal/config/load_test.go:48-63
func TestLoadImportOrder(t *testing.T) {
    d := t.TempDir()
    mustWrite(t, filepath.Join(d, "extra.toml"), `[[session]]
name="extra"
path="/extra"
`)
    p := filepath.Join(d, "sesh.toml")
    mustWrite(t, p, "import=[\"extra.toml\"]\n[[session]]\nname=\"main\"\npath=\"/main\"\n")
    cfg, _, err := Load(LoadOptions{Path: p})
    ...
}
```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Drift check | `git diff --stat 1a2235e..HEAD -- internal/config/load.go internal/config/load_test.go` | empty output, or reviewed drift |
| Focused tests | `go test ./internal/config` | exit 0 |
| Full lint | `just lint` | exit 0, `0 issues.` |
| Format check | `just fmt-check` | exit 0 |
| Full tests | `just test` | exit 0, all tests pass |

## Scope

**In scope**:

- `internal/config/load.go`
- `internal/config/load_test.go`

**Out of scope**:

- `internal/config/config.go`; do not rename config structs or TOML tags.
- `internal/sources/config_sessions.go`; this plan only fixes config merging.
- Docs changes. The existing docs already promise these keys.
- Any implementation of wildcard runtime behavior; that was source finding #1
  and was not selected in this batch.

## Git workflow

- Branch: `cdx/002-merge-nested-config-tables-field-by-field`
- Commit message: `fix: merge nested config tables field by field`
- Do not push or open a PR unless the operator asks.

## Steps

### Step 1: Add a failing regression test for nested table composition

Add a test to `internal/config/load_test.go`, near `TestLoadImportOrder`.
Create an imported config that sets multiple nested fields, then a main config
that overrides only one field in each nested table.

Required case:

- `extra.toml` sets:
  - `[default_session] startup_command = "git status"`
  - `[default_session] preview_command = "printf extra {}"`
  - `[default_session] windows = ["git"]`
  - `[tui] prompt = "Extra> "`
  - `[tui] placeholder = "Extra search"`
- `sesh.toml` imports `extra.toml` and sets:
  - `[default_session] startup_command = "make test"`
  - `[tui] placeholder = "Search workspaces"`

Expected loaded config:

- `DefaultSessionConfig.StartupCommand == "make test"`
- `DefaultSessionConfig.PreviewCommand == "printf extra {}"`
- `DefaultSessionConfig.Windows == []string{"git"}`
- `TUI.Prompt == "Extra> "`
- `TUI.Placeholder == "Search workspaces"`

Use `reflect.DeepEqual` in the test if needed; `load_test.go` already imports
only `os`, `path/filepath`, and `testing`, so add imports only if the test uses
them.

**Verify**: `go test ./internal/config` fails before the implementation with a
clear assertion showing the lost nested field.

### Step 2: Merge `TUIConfig` field by field

In `internal/config/load.go`, replace the whole-struct `dst.TUI = src.TUI`
assignment with field-level merge behavior:

- If `src.TUI.ShowIcons` is true, set `dst.TUI.ShowIcons = true`.
- If `src.TUI.Prompt` is non-empty, set `dst.TUI.Prompt`.
- If `src.TUI.Placeholder` is non-empty, set `dst.TUI.Placeholder`.

Do not add a "clear this inherited value" feature; this repo's bool/string
config style already treats zero values as unset.

**Verify**: `go test ./internal/config` still fails only if
`DefaultSessionConfig` is not fixed yet.

### Step 3: Merge `DefaultSessionConfig` field by field

In `internal/config/load.go`, replace the `reflect.DeepEqual` block for
`DefaultSessionConfig` with field-level merge behavior:

- Non-empty `StartupCommand`, `Tmuxp`, `Tmuxinator`, and `PreviewCommand`
  override destination fields.
- Non-empty `Windows` overrides destination windows.

After this change, the `reflect` import should be unused. Remove it from
`internal/config/load.go`.

**Verify**: `go test ./internal/config` exits 0.

### Step 4: Run repo checks

Run the standard checks expected by this repo.

**Verify**: `just lint`, `just fmt-check`, and `just test` all exit 0.

## Test plan

- Add one focused import-merging regression in `internal/config/load_test.go`.
- Pattern it after `TestLoadImportOrder`, using temp TOML files and direct
  assertions.
- Verification: `go test ./internal/config` passes with the new regression.

## Done criteria

- [ ] `internal/config/load.go` merges `TUIConfig` field by field.
- [ ] `internal/config/load.go` merges `DefaultSessionConfig` field by field.
- [ ] `internal/config/load.go` no longer imports `reflect` unless another
  live use exists.
- [ ] `go test ./internal/config` exits 0.
- [ ] `just lint`, `just fmt-check`, and `just test` exit 0.
- [ ] No files outside `internal/config/load.go`,
  `internal/config/load_test.go`, and `plans/README.md` are modified.
- [ ] `plans/README.md` status row for Plan 002 is updated.

## STOP conditions

Stop and report back if:

- The live `merge` function no longer matches the current-state excerpt.
- Existing tests show that whole-struct replacement is intentional behavior.
- Correct behavior requires a way to clear inherited string fields.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

Future config fields added to `TUIConfig` or `DefaultSessionConfig` need an
explicit merge rule. Reviewers should reject reintroducing whole-struct
replacement unless the config model gains explicit "clear inherited value"
semantics.

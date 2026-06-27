# Plan 003: Add explicit config support to preview

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1a2235e..HEAD -- internal/app/app.go internal/app/app_test.go`
> If either in-scope file changed since this plan was written, compare the
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

The docs describe `--config PATH` as the first config lookup source, and list,
connect, and picker already expose a `--config` flag. The `preview` command
loads config but cannot accept an explicit config path, so users cannot preview
with a fixture or alternate Sesh config. This is a small CLI consistency fix.

## Current state

- `internal/app/app.go` owns CLI routing and command behavior.
- Existing command parsers use `flag.NewFlagSet(..., flag.ContinueOnError)`
  and set `fs.SetOutput(a.Err)`.
- `internal/app/app_test.go` uses standard `testing` and in-memory buffers.

Current excerpts:

```go
// internal/app/app.go:251-274
func (a *App) preview(ctx context.Context, args []string) error {
    if len(args) < 1 {
        return errors.New("preview requires target")
    }
    target := args[0]
    cfg, err := a.loadConfig("")
    if err != nil {
        return err
    }
    sessions, err := a.collect(ctx, cfg, target)
    ...
    out, err := preview.Render(ctx, s, cfg.DefaultSessionConfig.PreviewCommand)
    ...
}
```

```go
// internal/app/app_test.go:90-99
func TestPickerJSONCommand(t *testing.T) {
    var out bytes.Buffer
    a := &App{Out: &out, Err: &bytes.Buffer{}}
    if err := a.Run(context.Background(), []string{"picker", "--json", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")}); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(out.String(), `"name": "sesh"`) {
        t.Fatalf("output = %q", out.String())
    }
}
```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Drift check | `git diff --stat 1a2235e..HEAD -- internal/app/app.go internal/app/app_test.go` | empty output, or reviewed drift |
| Focused tests | `go test ./internal/app` | exit 0 |
| Full lint | `just lint` | exit 0, `0 issues.` |
| Format check | `just fmt-check` | exit 0 |
| Full tests | `just test` | exit 0, all tests pass |

## Scope

**In scope**:

- `internal/app/app.go`
- `internal/app/app_test.go`

**Out of scope**:

- `internal/config/*`; this plan only passes an explicit path through.
- `internal/preview/*`; rendering behavior is Plan 005.
- README or docs changes; they already describe `--config PATH`.
- Adding a global flag framework.

## Git workflow

- Branch: `cdx/003-add-explicit-config-support-to-preview`
- Commit message: `fix: accept explicit config for preview`
- Do not push or open a PR unless the operator asks.

## Steps

### Step 1: Add a focused regression test

Add a test to `internal/app/app_test.go` near `TestPickerJSONCommand`.

Test shape:

1. Create a temp directory to preview.
2. Write a temp `sesh.toml` that sets:

   ```toml
   [default_session]
   preview_command = "printf configured:%s {}"
   ```

3. Run:

   ```go
   a.Run(context.Background(), []string{"preview", "--config", cfgPath, targetDir})
   ```

4. Assert stdout contains `configured:` and the target directory path.

Use `t.TempDir()`, `os.WriteFile(..., 0600)`, `bytes.Buffer`, and
`strings.Contains`, matching existing tests in the file.

**Verify**: `go test ./internal/app` fails before the implementation because
`preview` treats `--config` as the target.

### Step 2: Parse preview flags like the other commands

Change `preview` in `internal/app/app.go` to use a `flag.FlagSet`:

- `fs := flag.NewFlagSet("preview", flag.ContinueOnError)`
- `fs.SetOutput(a.Err)`
- `cfgPath := fs.String("config", "", "")`
- Parse `args`.
- After parsing, require `fs.NArg() >= 1`.
- Set `target := fs.Arg(0)`.
- Load config with `a.loadConfig(*cfgPath)`.

Keep all existing collection, resolve, render, and output behavior unchanged.

**Verify**: `go test ./internal/app` exits 0.

### Step 3: Run repo checks

Run the standard checks expected by this repo.

**Verify**: `just lint`, `just fmt-check`, and `just test` all exit 0.

## Test plan

- Add `TestPreviewCommandUsesExplicitConfig` in `internal/app/app_test.go`.
- Use `TestPickerJSONCommand` as the structural pattern for invoking `App.Run`
  with buffers.
- The test should prove `preview --config <path> <target>` uses the configured
  `[default_session].preview_command`.

## Done criteria

- [ ] `herdr-sesh preview --config PATH TARGET` parses correctly.
- [ ] Existing `herdr-sesh preview TARGET` behavior still works.
- [ ] `go test ./internal/app` exits 0.
- [ ] `just lint`, `just fmt-check`, and `just test` exit 0.
- [ ] No files outside `internal/app/app.go`, `internal/app/app_test.go`, and
  `plans/README.md` are modified.
- [ ] `plans/README.md` status row for Plan 003 is updated.

## STOP conditions

Stop and report back if:

- The live `preview` command already has a different flag parser.
- Adding `--config` would require changing `preview.Render` or config loading.
- The regression cannot avoid calling real `herdr`, `zoxide`, `eza`, or other
  external tools.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

Future CLI commands that load config should expose `--config` consistently or
document why they cannot. Reviewers should check that this change does not
introduce a new command framework for one flag.

# Plan 005: Return stable native preview text when path is missing

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1a2235e..HEAD -- internal/preview/preview.go internal/preview/preview_test.go internal/picker/tea.go internal/picker/fzf.go`
> If any in-scope or read-only context file changed since this plan was
> written, compare the "Current state" excerpts against the live code before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `1a2235e`, 2026-06-27

## Why this matters

The native picker routes previews through `preview.Render`, which runs the
configured preview command before considering workspace-only fallback text. If
Herdr returns a workspace without a path, the default preview command receives
an empty path and the UI shows raw command failure text. The fzf picker already
handles no-path rows with a stable message; the native path should do the same
without changing normal path previews.

## Current state

- `internal/preview/preview.go` centralizes native preview rendering.
- `internal/picker/tea.go` intentionally converts preview errors into display
  text; avoid changing picker state for this fix.
- `internal/picker/fzf.go` already has the desired user-facing no-path wording.
- `internal/preview/preview_test.go` has focused tests for configured commands
  and directory fallback.

Current excerpts:

```go
// internal/preview/preview.go:18-29
func Render(ctx context.Context, s model.Session, fallbackCommand string) (string, error) {
    cmd := s.PreviewCommand
    if cmd == "" {
        cmd = fallbackCommand
    }
    if cmd != "" {
        return runShell(ctx, config.SubstitutePath(cmd, s.Path))
    }
    if s.WorkspaceID != "" {
        return fmt.Sprintf("workspace: %s\nid: %s\npath: %s\n", s.Name, s.WorkspaceID, s.Path), nil
    }
    return directoryFallback(s.Path)
}
```

```go
// internal/picker/tea.go:364-374
func previewCommand(key string, s sessionmodel.Session, defaultPreviewCommand string) tea.Cmd {
    return func() tea.Msg {
        text, err := renderPreview(context.Background(), s, defaultPreviewCommand)
        if err != nil {
            text = err.Error()
        }
        text = strings.TrimRight(text, "\n")
        if text == "" {
            text = "No preview available"
        }
        return previewMsg{key: key, text: text}
    }
}
```

```sh
# internal/picker/fzf.go:102-105
if [ -z "$item_path" ] || [ ! -d "$item_path" ]; then
  printf 'No item path available\n'
  exit 0
fi
```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Drift check | `git diff --stat 1a2235e..HEAD -- internal/preview/preview.go internal/preview/preview_test.go internal/picker/tea.go internal/picker/fzf.go` | empty output, or reviewed drift |
| Focused tests | `go test ./internal/preview` | exit 0 |
| Picker regression safety | `go test ./internal/picker` | exit 0 |
| Full lint | `just lint` | exit 0, `0 issues.` |
| Format check | `just fmt-check` | exit 0 |
| Full tests | `just test` | exit 0, all tests pass |

## Scope

**In scope**:

- `internal/preview/preview.go`
- `internal/preview/preview_test.go`

**Read-only context**:

- `internal/picker/tea.go`
- `internal/picker/fzf.go`

**Out of scope**:

- Changing Bubble Tea layout or picker state.
- Changing fzf preview behavior.
- Changing the default preview command.
- Adding a new preview configuration option.

## Git workflow

- Branch: `cdx/005-return-stable-native-preview-text-when-path-is-missing`
- Commit message: `fix: handle native previews without paths`
- Do not push or open a PR unless the operator asks.

## Steps

### Step 1: Add focused no-path preview tests

Add tests to `internal/preview/preview_test.go` near
`TestRenderUsesPreviewCommand`.

Required cases:

1. `Render(context.Background(), model.Session{Name: "api"}, "printf %s {}")`
   returns no error and includes `No item path available`.
2. `Render(context.Background(), model.Session{Name: "api", WorkspaceID: "ws1"}, "printf %s {}")`
   returns no error and includes `workspace: api` and `id: ws1`.

The second case preserves useful workspace context when an ID exists but path
does not.

**Verify**: `go test ./internal/preview` fails before the implementation.

### Step 2: Guard empty paths before running preview commands

Change `Render` in `internal/preview/preview.go` so it handles empty
`s.Path` before running a configured command:

- If `s.Path == ""` and `s.WorkspaceID != ""`, return the existing workspace
  summary text.
- If `s.Path == ""` and no workspace ID exists, return
  `"No item path available\n", nil`.
- Otherwise preserve the existing command selection and directory fallback.

Do not move this logic into `internal/picker/tea.go`; fixing `preview.Render`
covers native picker previews and `herdr-sesh preview` consistently.

**Verify**: `go test ./internal/preview` exits 0.

### Step 3: Run picker and repo checks

Run picker tests because the native picker consumes `preview.Render` output.

**Verify**: `go test ./internal/picker`, `just lint`, `just fmt-check`, and
`just test` all exit 0.

## Test plan

- Add no-path coverage in `internal/preview/preview_test.go`.
- Use the existing preview tests as the structural pattern: call `Render`
  directly and assert returned text.
- Keep external tools out of the test; do not require `eza`, `bat`, `herdr`, or
  `zoxide`.

## Done criteria

- [ ] `preview.Render` does not run shell preview commands when `s.Path == ""`.
- [ ] No-path sessions without workspace IDs return `No item path available`.
- [ ] No-path sessions with workspace IDs return workspace summary text.
- [ ] Existing path-based preview command behavior still works.
- [ ] `go test ./internal/preview` and `go test ./internal/picker` exit 0.
- [ ] `just lint`, `just fmt-check`, and `just test` exit 0.
- [ ] No files outside `internal/preview/preview.go`,
  `internal/preview/preview_test.go`, and `plans/README.md` are modified.
- [ ] `plans/README.md` status row for Plan 005 is updated.

## STOP conditions

Stop and report back if:

- `preview.Render` no longer centralizes native preview command execution.
- A no-path session is expected to run a configured preview command for some
  documented reason.
- The fix requires changing picker layout or fzf behavior.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

Reviewers should check that no-path handling stays in `preview.Render`, not in
one UI caller. Future preview behavior changes should keep native and fzf
fallback wording aligned unless there is a deliberate UX reason to diverge.

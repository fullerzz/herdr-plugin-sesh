# Previews

Users inspect a directory, command output or running pane before switching.

## Sub-features

- `preview-local`: the configured command receives the selected path through `{}`.
- `preview-remote`: listing and Git status come from the selected machine.
- `preview-pane`: active terminal output comes from the requested workspace.

## How to get to it (user POV)

Run `herdr-sesh preview TARGET`, view the native picker's preview, or press
the preview-cycle key (Ctrl+O by default) in the picker to switch between command
and pane mode. The fzf preview is a distinct
entry point and shows metadata only for remote rows.

## Driving it with verify.py

Use the private launch for local CLI and picker previews. Reading a running
pane needs its live workspace. A remote listing also needs a registered,
reachable machine. Capture each entry point separately.

```sh
uv run --frozen python "$VERIFY" cli "$EVIDENCE" local-preview preview verify-alpha
```

Require `README.md` in stdout and exit 0; compare to the actual fixture file.
For picker preview, change the disposable fixture's `show_preview` to true,
launch the native picker in a PTY, highlight alpha and capture its preview.
Add enough files to alpha to fill the preview. Scroll the wheel over it and check
that the first visible filename changes while alpha stays highlighted. Ctrl+O
must report pane preview unavailable because this configured session is unopened.

With the [machine fixture](workspaces.md) in the private config, run
`preview NAME` through the CLI helper. Expect exit 0, the fixture's name,
machine and path, and a message that remote previews need the Herdr app.

For a remote CLI preview, outside the private helper, obtain an existing qualified
ID from `herdr-app ls` and save it in `REMOTE_SPACE`. Run
`HERDR_SESH_BACKEND=app ./bin/herdr-sesh preview "$REMOTE_SPACE"`, recording argv,
stdout, stderr and exit. Independently compare the listing using
`hc exec --on "$REMOTE_MACHINE" --cwd "$REMOTE_FOLDER" --timeout 3 -- ls -A -p -1`
with machine/folder from that same listing record. For `~`, omit `--cwd` and
its value. For `~/project`, pass `--cwd project` so hc resolves it from the
remote home. For pane mode, capture the
picker screen and compare against `hc read` of that workspace's active terminal.

## Gotchas

The seed has no remote space. Missing registration/connectivity means blocked,
not successful metadata-only remote preview. Do not substitute a local folder
with the same path. Remote preview has a 3-second total budget; unavailable
machines should give metadata plus a reason. Git absence/non-repositories omit
Git status, other Git failures retain the listing with a reason. Paths containing
`..` currently differ between creation and preview; record that limitation.

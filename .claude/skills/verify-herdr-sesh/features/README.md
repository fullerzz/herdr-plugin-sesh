# herdr-sesh verification map

This directory is the maintained source for verifying the user-facing behavior of herdr-sesh. Read the index before driving, then use the matching feature file as the recipe.

## Baseline preconditions

- `RUN=$("$HSV" up "$TARGET_REPO")` started an isolated Herdr; `"$HSV" doctor "$RUN"` prints only `ok` lines.
- Fixtures from `up`: plugin config copied from `testdata/herdr-sesh.toml`; `~/projects/sesh` (git repo, configured session `sesh` with tab `git`, startup `git status`); `~/projects/zoxed` (zoxide entry). `~` means `$RUN/home`.
- A focused workspace `base` exists: `"$HSV" x "$RUN" herdr workspace create --cwd "$RUN/home" --label base --focus`.
- Every `herdr` / `herdr-sesh` command runs as `"$HSV" x "$RUN" …`, `"$HSV" cli "$RUN" <label> …`, or `"$HSV" tui`. Never `( . "$RUN/env.sh"; … )` (fails open to the user's Herdr), never a bare `herdr`. Never drive an instance this run did not start.

## Driving conventions

- Overlay panes: right after `"$HSV" x "$RUN" herdr plugin action invoke …`, `P=$("$HSV" overlay "$RUN" "Sesh Picker")` (or `"Sesh Settings"`). It polls until the focused pane has that label, prints its ID, and dies on timeout.
- Workspace IDs: read them, never assume `w1`/`w2`: `S=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="sesh").workspace_id')`. Tabs: `.result.tabs[]` (`tab_id`, `label`, `focused`); panes: `.result.panes[]` (`pane_id`).
- Wait on screen text, not sleeps: `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match '<text>' --timeout 5000`, or `"$HSV" wait` for tmux sessions.
- Screen rows are column-padded, so row text never matches literally. Use `--regex 'config +sesh'` for rows; `--match` only for strings that render verbatim (`Sesh> ses`, `1/3 workspaces`, `pending changes`, `SETTINGS — REVIEW`, `HERDR SESH / SETTINGS`).
- Wait for focus (Herdr state, no screen): poll, do not sleep a fixed time:

  ```sh
  "$HSV" x "$RUN" sh -c 'for i in $(seq 25); do herdr workspace list | jq -e --arg l "$1" ".result.workspaces[]|select(.focused and .label==\$l)" >/dev/null && exit 0; sleep 0.2; done; exit 1' _ base
  ```

- Herdr key names: `enter`, `esc`, `up`, `down`, `ctrl+x`, `ctrl+o`, `ctrl+r`, `ctrl+s`, `f2`, `y`. Text: `"$HSV" x "$RUN" herdr pane send-text "$P" <text>`.
- tmux key names: `Enter`, `Escape`, `C-x`, `C-o`, `F2`; literal text with `-l`.
- Restore state between recipes by closing workspaces you created (`"$HSV" x "$RUN" herdr workspace close "$S"` with `$S` read from `workspace list`) or by starting a fresh run.

## Proof and skip reporting

- Capture the user action and the resulting state: screen before, screen after, and a read-only second view (`workspace list`, `tab list`, `pane read`, config `diff`).
- CLI proof includes command, stdout, stderr, and exit code (`"$HSV" cli`).
- Save every artifact in `$HSV_EVIDENCE` named `<feature-id>-<step>` (labels unique across features): `"$HSV" cli "$RUN" <label> herdr …` or `"$HSV" x "$RUN" sh -c '… > "$HSV_EVIDENCE/<label>.json"'`. Record which entry point produced each.
- Report an unreachable path with the attempted command and the unmet precondition. Do not report a skipped entry point as verified through another.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing user-visible behavior, then exactly four H2 sections in order: `Sub-features`, `How to get to it (user POV)`, `Driving it with hsv + herdr pane API`, `Gotchas`. The driving section starts with `Preconditions:` and pairs each user action with an exact command and observable result. Keep implementation details out.

## Features

- [Sesh picker](./picker.md) covers opening the overlay, filtering, selecting to create or focus a workspace, closing workspaces, sort and preview toggles, and the fzf variant.
- [Connect](./connect.md) covers `herdr-sesh connect` by name, path, and ID, with tabs and startup commands, and `--no-focus`.
- [List sessions](./list.md) covers merged herdr/config/zoxide sources, blacklist, and JSON output.
- [Previous workspace](./last.md) covers the `last` action toggling focus via plugin-recorded history.
- [Settings editor](./settings.md) covers the settings overlay, F2 from the picker, standalone `config edit`, review, and save.

Not yet mapped: `preview`, `clone`, `root`, `window`, `config path|init|validate|migrate`, pane layouts (`docs/config/layouts.md`).

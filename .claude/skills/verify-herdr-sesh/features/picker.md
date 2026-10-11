# Sesh picker

The Sesh picker is a searchable overlay listing running Herdr workspaces, configured sessions, and zoxide directories; Enter focuses an existing workspace or creates one with the session's tabs and startup command.

## Sub-features

- `picker-open` opens the overlay from each entry point with all sources listed.
- `picker-filter` narrows rows as the user types; header shows `matches/total workspaces`.
- `picker-create` selecting a configured session or directory creates and focuses a workspace with its tabs and startup command.
- `picker-focus` selecting a running workspace focuses it without rerunning startup.
- `picker-close-ws` Ctrl+X closes the selected running workspace and refreshes the list.
- `picker-sort` Ctrl+R cycles workspace → recent → agent sorting.
- `picker-preview` Ctrl+O switches command preview and active-pane preview.
- `picker-cancel` Esc exits without changing focus.
- `picker-fzf` experimental fzf backend (`--fzf` or `HERDR_SESH_PICKER=fzf`).

## How to get to it (user POV)

- Herdr action `fullerzz.sesh.open-picker` (usually bound to a key such as `prefix+shift+t`).
- Herdr CLI: `"$HSV" x "$RUN" herdr plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay` (users run it without the wrapper).
- From a shell: `herdr-sesh picker`, or `herdr-sesh picker --fzf`.

## Driving it with hsv + herdr pane API

Preconditions:

- Baseline from [README](./README.md): doctor all `ok`, workspace `base` focused, no workspace named `sesh`.

- **Open via action.** `"$HSV" x "$RUN" herdr plugin action invoke fullerzz.sesh.open-picker >/dev/null`, then `P=$("$HSV" overlay "$RUN" "Sesh Picker")`. `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match '3 workspaces' --timeout 5000`. Screen shows `Sesh> Search workspaces`, `3 workspaces`, and rows matching regexes `herdr +base`, `config +sesh +~/projects/sesh`, `zoxide +zoxed +~/projects/zoxed`. Save: `"$HSV" cli "$RUN" picker-open herdr pane read "$P" --source visible`.
- **Filter.** `"$HSV" x "$RUN" herdr pane send-text "$P" ses`, then `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match '1/3 workspaces' --timeout 5000`. Screen shows `Sesh> ses`, `1/3 workspaces` (matches/total), one row matching `config +sesh`, and a preview listing `.git` and `README.md`. Save `picker-filtered` (same `cli … herdr pane read` form).
- **Create.** `"$HSV" cli "$RUN" picker-ws-before herdr workspace list`, then `"$HSV" x "$RUN" herdr pane send-keys "$P" enter`. Poll focus on `sesh` (README snippet with `_ sesh`). `"$HSV" cli "$RUN" picker-ws-after herdr workspace list` shows a new focused workspace labeled `sesh`; `"$HSV" x "$RUN" herdr pane list` no longer has `$P`.
- **Side effect.** `S=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="sesh").workspace_id')`. `"$HSV" cli "$RUN" picker-tabs herdr tab list --workspace "$S"` shows tabs `1` and `git` (focused). Git pane: `T=$("$HSV" x "$RUN" herdr tab list --workspace "$S" | jq -r '.result.tabs[]|select(.label=="git").tab_id')`, `G=$("$HSV" x "$RUN" herdr pane list --workspace "$S" | jq -r --arg t "$T" '.result.panes[]|select(.tab_id==$t).pane_id')`. `"$HSV" x "$RUN" herdr pane wait-output "$G" --match 'No commits yet' --timeout 5000`; `"$HSV" cli "$RUN" picker-git-pane herdr pane read "$G" --source recent` shows `git status` output: a line starting `On branch `, `No commits yet`, untracked `README.md`.
- **Focus existing.** Reopen the picker (action + `overlay`), `"$HSV" x "$RUN" herdr pane send-text "$P" base`, `"$HSV" x "$RUN" herdr pane send-keys "$P" enter`, poll focus on `base`. `workspace list` count unchanged; `"$HSV" x "$RUN" herdr pane read "$G"` gains no new `git status`. Save `picker-focus-ws`.
- **Close workspace.** Reopen, `"$HSV" x "$RUN" herdr pane send-text "$P" sesh`, `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --regex 'herdr +sesh' --timeout 5000`, select that row (`up`/`down` as needed), `"$HSV" x "$RUN" herdr pane send-keys "$P" ctrl+x`. `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --regex 'config +sesh' --timeout 5000`: the `herdr sesh` row is gone, the `config sesh` row is back. `"$HSV" cli "$RUN" picker-close-ws herdr workspace list` has no `sesh`.
- **Sort / preview.** `"$HSV" x "$RUN" herdr pane send-keys "$P" ctrl+r` changes the footer sort label (`ctrl+r recent` → next mode); `ctrl+o` (same form) changes the preview heading between command and pane preview. Save `picker-sort-before/after`, `picker-preview-before/after`.
- **Cancel.** `"$HSV" x "$RUN" herdr pane send-keys "$P" esc`. Overlay closes (`$P` gone from `pane list`); focused workspace unchanged (`picker-cancel-ws`).
- **Direct binary / fzf.** `"$HSV" tui "$RUN" pick herdr-sesh picker` (or `herdr-sesh picker --fzf`), `"$HSV" wait "$RUN" pick 'Sesh>'`, `"$HSV" keys "$RUN" pick -l ses`, `"$HSV" keys "$RUN" pick Enter`, `"$HSV" snap "$RUN" pick picker-tmux-exit`. Pane shows `[hsv exit 0]`; `"$HSV" cli "$RUN" picker-tmux-ws herdr workspace list` gains `sesh`.

## Gotchas

- `open-picker` with no workspaces fails with `no active workspace` (see `"$HSV" x "$RUN" herdr plugin log list --plugin fullerzz.sesh`); create `base` first.
- Ctrl+X closes the real workspace immediately, without confirmation.
- A new workspace keeps Herdr's initial shell tab `1` alongside `git`; that is expected, not a duplicate.
- An empty directory previews as `No preview available` (`eza -la` prints nothing); fixtures include a README for this reason.
- Selecting an existing workspace never reruns startup commands. Proving `picker-create` needs a session that is not already open.
- Paths in Herdr output appear as `/private/tmp/hsv.*` on macOS.

# Settings editor

The settings editor is a TUI form over global plugin settings (Picker, Lists, Naming, Keys, Workspace Defaults) that stages edits as pending changes, shows a review, and writes only edited keys to the config file after the user confirms with `y`.

## Sub-features

- `settings-open` opens the editor showing the resolved config path and `0 pending changes`.
- `settings-toggle` Enter toggles a boolean and marks it `*` with a pending count.
- `settings-review` Ctrl+S opens `SETTINGS — REVIEW` listing `<label> (<key>)` and `old → new`.
- `settings-save` `y` writes only the edited keys; `n`/Esc returns to editing.
- `settings-discard` Esc with pending changes asks before discarding.
- `settings-from-picker` F2 in the picker opens settings; returning applies saved changes to the picker.

## How to get to it (user POV)

- Herdr action `fullerzz.sesh.open-settings`.
- F2 in the native picker (Ctrl+, when preview cycling is bound to F2).
- Standalone: `herdr-sesh config edit [--config PATH]` (no Herdr needed).

## Driving it with hsv + herdr pane API

Preconditions:

- Baseline from [README](./README.md); fixture config has `show_icons = true`.
- Snapshot the config first: `"$HSV" x "$RUN" sh -c 'cp "$HERDR_PLUGIN_CONFIG_DIR/config.toml" "$HSV_EVIDENCE/settings-before.toml"'`.

- **Open via action.** `"$HSV" x "$RUN" herdr plugin action invoke fullerzz.sesh.open-settings >/dev/null`, then `P=$("$HSV" overlay "$RUN" "Sesh Settings")`. `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match 'HERDR SESH / SETTINGS' --timeout 5000`. Screen shows `HERDR SESH / SETTINGS`, the path `…/fullerzz.sesh/config.toml` under `$RUN/cfg/herdr/plugins/config/`, `0 pending changes`, and a row matching regex `› +Show icons +on`. Save: `"$HSV" cli "$RUN" settings-open herdr pane read "$P" --source visible`.
- **Toggle.** `"$HSV" x "$RUN" herdr pane send-keys "$P" enter`, then `"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match '1 pending changes' --timeout 5000`. Row matches `› +\* +Show icons +off`. Save `settings-toggle`.
- **Review.** `"$HSV" x "$RUN" herdr pane send-keys "$P" ctrl+s`, wait for `--match 'SETTINGS — REVIEW'`. Screen shows `Show icons (picker.show_icons)`, `on → off`, `↑/↓ scroll · y save · n/esc keep editing`. Save `settings-review`.
- **Save.** `"$HSV" x "$RUN" herdr pane send-keys "$P" y`, wait for `--match '0 pending changes'`. Save `settings-saved`.
- **Side effect.** `"$HSV" x "$RUN" sh -c 'diff "$HSV_EVIDENCE/settings-before.toml" "$HERDR_PLUGIN_CONFIG_DIR/config.toml" > "$HSV_EVIDENCE/settings-diff.txt"'` (exit `1` = differences) shows only `show_icons = true` → `show_icons = false`.
- **From picker.** Open the picker (`open-picker` action, `P=$("$HSV" overlay "$RUN" "Sesh Picker")`), `"$HSV" x "$RUN" herdr pane send-keys "$P" f2`, wait for `--match 'HERDR SESH / SETTINGS'` in the same overlay. `esc` returns to the picker (wait for `--match 'Sesh>'`); with `show_icons = false` the source icons are gone from rows. Save `settings-from-picker-settings` and `settings-from-picker-back`.
- **Standalone.** Re-snapshot right before: `"$HSV" x "$RUN" sh -c 'cp "$HERDR_PLUGIN_CONFIG_DIR/config.toml" "$HSV_EVIDENCE/settings-standalone-before.toml"'`. `"$HSV" tui "$RUN" set herdr-sesh config edit`, `"$HSV" wait "$RUN" set 'pending changes'`, then `Enter`, `C-s`, `y` via `"$HSV" keys "$RUN" set …`, `"$HSV" snap "$RUN" set settings-standalone-after`. `"$HSV" x "$RUN" sh -c 'diff "$HSV_EVIDENCE/settings-standalone-before.toml" "$HERDR_PLUGIN_CONFIG_DIR/config.toml" > "$HSV_EVIDENCE/settings-standalone-diff.txt"'` shows only `show_icons = false` → `show_icons = true` (the action steps above already saved `false`).

## Gotchas

- Text and list editors use Ctrl+S twice: once to apply the edit to the draft, again from the form to open review. Enter inserts a newline there.
- Only `y` in review writes. A screen showing the new value with `*` is a draft, not proof.
- Workspaces, tabs, and rules are file-only; the editor never shows them.
- Below 60x18 the form shows resize instructions; `hsv.sh tui` uses 120x32.
- `just settings-demo` edits a disposable copy of `testdata/herdr-sesh.toml` but runs outside the isolated env; use `hsv.sh tui` for evidence.

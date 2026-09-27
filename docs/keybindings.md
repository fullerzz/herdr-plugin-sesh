---
icon: lucide/keyboard
---

# Keybindings

## Native picker controls

| Key | Action |
| --- | --- |
| ++enter++ | Focus the selected workspace, or create it for a configured session or directory. |
| ++esc++ / ++ctrl+c++ | Close the picker without selecting a workspace. |
| ++down++ / ++ctrl+n++ / ++ctrl+j++ | Move from the filter into the list, then move down. |
| ++up++ / ++ctrl+p++ / ++ctrl+k++ | Move up while the list is focused; moving above its first row returns to the filter. In the filter, ++ctrl+k++ keeps its text-editing behavior. |
| ++right++ | Return from the list to the filter; within the filter, move the text cursor. |
| ++ctrl+u++ | Clear the filter and focus it. |
| ++ctrl+v++ | Paste into the filter. |
| ++ctrl+r++ | Cycle workspace → recent → agent sorting for Herdr workspaces. |
| ++ctrl+o++ | Switch command/pane previews, unless overridden by `keys.cycle_preview_mode`. |
| ++f2++ | Open settings; uses **Ctrl+,** when preview cycling is bound to F2. |
| ++ctrl+x++ | Close the selected running Herdr workspace and refresh the list. |

Typing updates the filter even when the list is focused. These controls describe
the native picker; the experimental fzf picker uses its own controls.

!!! warning "Closing a workspace"

    ++ctrl+x++ requests a workspace close immediately, without a picker
    confirmation. It affects the running workspace, not just its row. It does
    nothing for configured sessions or directory results. During the request,
    selection is disabled; ++esc++ requests cancellation and exits once the
    request returns. Cancellation cannot undo a close that already completed.

## Native picker previews

By default, press ++ctrl+o++ to switch between command and active-pane previews. The heading
shows the current shortcut; configure another key or disable cycling through
[`keys.cycle_preview_mode`](config/picker.md#keys). Switching modes does not save
the choice; set [`picker.preview_mode`](config/picker.md#preview-controls) to
change the initial mode. Cycling is disabled when `show_preview = false`.

For side-by-side previews, drag the vertical divider with the left mouse button.
Both panels keep a minimum width; the chosen width resets when the picker closes.
Narrow terminals stack the preview below the list. These controls apply to the
native picker, not fzf.

## Herdr actions

The `fullerzz.sesh.last` action switches to the previously focused workspace,
including switches made outside the picker. History is separate for each Herdr
session and closed workspaces are pruned automatically. See
[Workspace history](config/picker.md#workspace-history) for details.

!!! note "Prerequisite"

    Build and link this checkout, or install a published release with
    `herdr plugin install fullerzz/herdr-plugin-sesh --ref <release-tag>` using
    a tag from the
    [GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases)
    page.

Example Herdr keybinding once the plugin is linked:

Add this to **Herdr's** `~/.config/herdr/config.toml` (or the file selected by
`HERDR_CONFIG_PATH` / `XDG_CONFIG_HOME`), not the plugin's `config.toml`.

```toml
[keys]
# If using "prefix+shift+t" to open the herdr-sesh plugin picker, the rename_tab keybind needs to be changed.
rename_tab = "prefix+shift+,"

[[keys.command]]
key = "prefix+shift+t"
type = "plugin_action"
command = "fullerzz.sesh.open-picker"
description = "open Sesh picker"

[[keys.command]]
key = "prefix+shift+b"
type = "plugin_action"
command = "fullerzz.sesh.last"
description = "switch to previous Sesh workspace"
```

Manual picker open:

```bash
herdr plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay
```

## Settings controls

| Key | Action |
| --- | --- |
| Up / Down, Tab / Shift+Tab | Select a setting. |
| Enter / Space | Toggle a boolean, cycle a choice, or open an editor. |
| Left / Right | Cycle choices backward / forward. |
| Ctrl+R | Revert the selected setting to its loaded value. |
| Ctrl+S | Apply text/list edits to the draft; from the form, review all changes. |
| Enter in a text editor | Insert a newline; use Ctrl+S to apply. |
| `\t` / `\\` in a text editor | Represent a tab / literal backslash. Pasted text is escaped automatically. |
| A / Enter / D in a list | Add / edit / delete an item. |
| Ctrl+Up / Ctrl+Down in a list | Reorder items. |
| Y in save review | Confirm writing the reviewed changes. |
| Esc | Cancel the current edit, close review, or return; confirm before discarding unsaved changes. |

Open settings directly with:

```bash
herdr plugin action invoke fullerzz.sesh.open-settings
```

See [Settings editor](config.md#settings-editor) for file handling and migration.

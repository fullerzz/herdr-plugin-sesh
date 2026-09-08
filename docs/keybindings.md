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

In the native picker, press ++ctrl+o++ by default to switch between the configured preview
and **Pane** mode. Pane mode shows the visible terminal contents of the selected
Herdr workspace's active pane, in its active tab, and refreshes once per second.
It works for background workspaces without changing focus. Press ++ctrl+o++ again
to return to the configured preview. ++ctrl+v++ pastes into the filter.

Override the shortcut in the herdr-sesh `config.toml`:

```toml
[keys]
cycle_preview_mode = "alt+p" # default: "ctrl+o"; "" disables cycling
```

The preview heading shows the configured shortcut, or no shortcut when disabled.
See [picker keys](config.md#keys) for key syntax and binding precedence.

Set `[picker].preview_mode = "pane"` in `config.toml` to start in Pane mode;
the default is `"command"`. Switching modes in the picker does not change the file.

Configured sessions and directories have no running pane and display an
unavailable message in Pane mode. The toggle is disabled when
`[picker].show_preview = false` and does not affect the fzf picker.

When the preview is beside the list, drag the vertical divider with the left
mouse button to resize it. The width resets when the picker closes; narrow
terminals show a stacked preview instead.

## Herdr actions

The `fullerzz.sesh.last` action switches to the previously focused workspace,
including switches made outside the picker. History is separate for each Herdr
session and closed workspaces are pruned automatically. See
[Workspace history](config.md#workspace-history) for details.

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

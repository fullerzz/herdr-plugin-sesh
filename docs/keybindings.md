# Keybindings

## Native picker previews

In the native picker, press ++ctrl+o++ to switch between the configured preview
and **Pane** mode. Pane mode shows the visible terminal contents of the selected
Herdr workspace's active pane, in its active tab, and refreshes once per second.
It works for background workspaces without changing focus. Press ++ctrl+o++ again
to return to the configured preview. ++ctrl+v++ pastes into the filter.

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

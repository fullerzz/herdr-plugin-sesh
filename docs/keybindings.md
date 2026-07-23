# Keybindings

Prerequisite: build and link this checkout, or install a published release with
`herdr plugin install fullerzz/herdr-plugin-sesh --ref <release-tag>` using a
tag from the [GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases)
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

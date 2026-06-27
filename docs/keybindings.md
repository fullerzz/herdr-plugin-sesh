# Keybindings

Prerequisite: build and link this checkout, or install the published plugin with
`herdr plugin install fullerzz/herdr-plugin-sesh --ref v0.1.0` when a release
is published.

Example Herdr keybinding once the plugin is linked:

```toml
[keys]
rename_tab = ""

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

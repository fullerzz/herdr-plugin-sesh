# Keybindings

Example Herdr keybinding once the plugin is linked:

```toml
[[keys.command]]
key = "prefix+s"
type = "plugin_action"
command = "fullerzz.sesh.open-picker"
description = "open Sesh picker"
```

Manual picker open:

```bash
herdr plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay
```

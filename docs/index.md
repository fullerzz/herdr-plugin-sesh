---
icon: lucide/panels-top-left
---

# herdr-sesh

A [Sesh](https://github.com/joshmedeski/sesh)-inspired workspace picker and session manager for
[Herdr](https://herdr.dev/).

![Herdr Sesh picker demo](assets/picker-demo-42.gif){ loading=lazy width=700 }

`herdr-sesh` combines running Herdr workspaces, configured sessions, and
zoxide history in one searchable overlay. Selecting an item focuses its
existing workspace or creates a new one with the configured startup command
and tabs.

## Install

```bash
herdr plugin install fullerzz/herdr-plugin-sesh
```

Open the picker through the installed plugin action:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

!!! note "Configuration is optional"

    Without a config file, the picker still includes running Herdr workspaces
    and zoxide results when zoxide is available.

## Documentation

- [Configuration](config.md) explains config discovery, picker behavior,
  workspaces, tabs, and legacy Sesh migration.
- [Keybindings](keybindings.md) shows how to invoke the picker and related
  actions from Herdr.
- [GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases)
  contains versioned source and release notes.

## Requirements

- Herdr 0.8.0 or newer
- Linux or macOS
- Git and Go 1.26.4 or newer for Herdr's source-based plugin installation
- Optional: `zoxide` for directory history and `eza` for the default preview
- Optional: `fzf` and `bat` for the experimental fzf picker

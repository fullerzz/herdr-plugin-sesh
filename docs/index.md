---
icon: lucide/panels-top-left
---

# herdr-sesh

A [Sesh](https://github.com/joshmedeski/sesh)-inspired workspace picker and session manager for
[Herdr](https://herdr.dev/).

![Herdr Sesh picker demo](assets/picker-demo-42.gif){ loading=lazy width=700 }

`herdr-sesh` combines running Herdr workspaces, configured sessions, and
zoxide history in one searchable overlay. Selecting an item focuses its
existing workspace or creates a new one with the configured startup command,
tabs, and pane layouts.

## Choose your Herdr integration

This repository provides a **herdr-tui plugin** for Herdr's terminal UI and
**herdr-app mods** for the desktop app. Both use the same `herdr-sesh` binary
and native TOML configuration.

| Capability | Herdr TUI plugin | Herdr app mods |
| --- | --- | --- |
| Installation | [Plugin installation](#install) | [App mod setup](herdr-app.md#install) |
| Open the picker | `fullerzz.sesh.open-picker` plugin action | `mod.open:sesh` |
| Remote workspaces | Unavailable | Select existing remote spaces; create configured ones |
| Track navigation outside Sesh | Automatic through plugin hooks | Requires a [`sesh-last` layout block](herdr-app.md#switch-to-the-last-space) |
| Pane split ratios | Supported | Splits evenly; ignores `ratio` |

Remote workspace support is available only through the Herdr app integration,
including direct CLI commands targeting the app. Configured sessions and
directory entries create local workspaces in both integrations. With the app,
a configured workspace that names a `machine` creates its space on that machine.

| Sesh term | Herdr TUI term | Herdr app term |
| --- | --- | --- |
| Session | Workspace | Space |
| Window | Tab | Tab |
| Picker | Overlay pane | Mod dialog |

The picker focuses a workspace that is already running. Selecting a configured
workspace or zoxide directory creates one if it is not open. Startup commands,
tabs, and pane layouts run only when a workspace is created.

!!! tip "Open a complete project layout from one picker entry"

    Define reusable tabs with panes split right or down, custom split ratios,
    and a working directory, environment, and startup command for each pane.
    Open your editor, development server, and logs together; reconnecting keeps
    the running layout intact. See the
    [pane layout walkthrough and diagram](config/layouts.md#pane-layout-walkthrough)
    for a complete configuration.

## Requirements

These requirements apply to the Herdr TUI plugin. For the desktop app, follow
the [Herdr app setup](herdr-app.md#install).

- Herdr 0.8.2 or newer
- Linux or macOS
- Git and Go 1.26.4 or newer for Herdr's source-based plugin installation
- Optional: `zoxide` for directory history and `eza` for the default preview
- Optional: `fzf` and `bat` for the experimental fzf picker

## Install

For Herdr TUI, install the plugin:

```bash
herdr plugin install fullerzz/herdr-plugin-sesh
```

## First use

From a running Herdr TUI session, open the picker through the installed plugin action:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

In the Herdr app, open the [`sesh` mod](herdr-app.md#install) instead.

You should see your running Herdr workspaces. If zoxide has recorded directories,
those appear too. Type part of a name or path, then press ++enter++ to focus an
existing workspace or create one from a directory. No config file is needed.

To add your own named project, follow [Create your configuration](config.md#create-your-configuration).
That walkthrough shows a complete, small config file and ends by opening the
new workspace from the picker.

### Everyday tasks

- [Bind the picker and previous-workspace actions to keys](keybindings.md#herdr-actions).
- [Switch previews or change their shortcut](keybindings.md#native-picker-previews).
- [Open a project with named tabs and split panes](config/layouts.md#pane-layout-walkthrough).
- [Use direct CLI commands](commands.md#command-reference).
- [Diagnose a missing workspace or unexpected result](troubleshooting.md).

## Documentation versions

Use the version selector in the header to choose a release's documentation.
`latest` follows the `main` branch and may describe changes not yet released.
Release snapshots are available starting with `v0.10.0`, when the wiki was added.

For changes in each release, see the
[release notes](https://github.com/fullerzz/herdr-plugin-sesh/releases).

### Reference

- [Configuration](config.md) explains config discovery, workspaces, and legacy
  Sesh migration. [Picker](config/picker.md) covers appearance, sorting, and
  shortcuts. [Layouts](config/layouts.md) covers named tabs and split panes.
- [Keybindings](keybindings.md) shows how to invoke the picker and related
  actions from Herdr.
- [Commands](commands.md) covers direct CLI use and installed-binary setup.
- [Contributing](development/index.md) covers local development, tests, docs,
  and releases.
- [GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases)
  contains versioned source and release notes.

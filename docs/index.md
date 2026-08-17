# Sesh for Herdr

`herdr-plugin-sesh` is a Sesh-inspired workspace picker and session manager
for [Herdr](https://herdr.dev/). It combines running Herdr workspaces,
configured sessions, and zoxide history in one searchable overlay.

## Features

- Search active workspaces, configured sessions, and zoxide history.
- Focus an existing workspace or create one with configured commands and tabs.
- Filter, sort, deduplicate, and optionally cache session results.
- Jump directly to the previously focused workspace.
- Use the native picker by default or opt into the experimental fzf picker.

## Requirements

- Herdr 0.8.0 or newer
- Linux or macOS
- Git and Go 1.26 or newer for source-based plugin installation
- Optional: `zoxide` for directory history and `eza` for previews
- Optional: `fzf` and `bat` for the experimental fzf picker

## Installation

Install the plugin directly from GitHub:

```bash
herdr plugin install fullerzz/herdr-plugin-sesh
```

To skip the confirmation in a non-interactive environment, add `--yes`. To
pin a release, add `--ref <release-tag>` using a tag from the
[GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases) page.

## Quick start

Open the picker through the installed plugin action:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

You can also open its overlay pane directly:

```bash
herdr plugin pane open \
  --plugin fullerzz.sesh \
  --entrypoint picker \
  --placement overlay
```

Configuration is optional. See the [configuration reference](config.md) for
the lookup order, supported settings, and migration instructions. See
[keybindings](keybindings.md) to bind the picker and previous-workspace actions
in Herdr.

## CLI

| Command | Purpose |
| --- | --- |
| `herdr-sesh picker` | Open the native workspace picker. |
| `herdr-sesh picker --fzf` | Open the experimental fzf picker. |
| `herdr-sesh list --json` | List merged session sources as JSON. |
| `herdr-sesh connect TARGET` | Focus or create a workspace. |
| `herdr-sesh preview TARGET` | Render the configured preview. |
| `herdr-sesh clone REPOSITORY` | Clone a repository and connect to it. |
| `herdr-sesh root --connect` | Connect to the current Git repository root. |
| `herdr-sesh last` | Focus the previously used workspace. |
| `herdr-sesh config path` | Print the resolved plugin config path. |
| `herdr-sesh config init` | Create a starter config. |
| `herdr-sesh config migrate` | Convert a legacy Sesh-style config. |

The plugin actions are the normal entry points for day-to-day use.

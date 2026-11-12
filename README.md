# Sesh for Herdr

[![Latest release](https://img.shields.io/github/v/release/fullerzz/herdr-plugin-sesh)](https://github.com/fullerzz/herdr-plugin-sesh/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/fullerzz/herdr-plugin-sesh)](go.mod)
[![License: MIT](https://img.shields.io/github/license/fullerzz/herdr-plugin-sesh)](LICENSE)
[![Test](https://github.com/fullerzz/herdr-plugin-sesh/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/fullerzz/herdr-plugin-sesh/actions/workflows/test.yml)
[![Lint](https://github.com/fullerzz/herdr-plugin-sesh/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/fullerzz/herdr-plugin-sesh/actions/workflows/lint.yml)

A [Sesh](https://github.com/joshmedeski/sesh)-inspired workspace picker and
session manager for [Herdr](https://herdr.dev/).

This README describes `main`, including unreleased changes. Use the
[documentation version selector](https://fullerzz.github.io/herdr-plugin-sesh/)
for the documentation matching your installed release.

`herdr-plugin-sesh` combines running Herdr workspaces, configured sessions, and
zoxide history in one searchable overlay. Selecting an item focuses its
existing workspace or creates a new one with the configured startup command and
tabs.

![Herdr Sesh picker demo](docs/assets/picker-demo-42.gif)

## Choose your Herdr integration

This repository provides a **herdr-tui plugin** for Herdr's terminal UI and
**herdr-app mods** for the desktop app. Both use the same `herdr-sesh` binary
and native TOML configuration.

| Capability | Herdr TUI plugin | Herdr app mods |
| --- | --- | --- |
| Installation | `herdr plugin install fullerzz/herdr-plugin-sesh` | [App setup](docs/herdr-app.md#install), including `just install-app-mod` |
| Open the picker | `fullerzz.sesh.open-picker` plugin action | `mod.open:sesh` |
| Remote workspaces | Unavailable | Select existing remote spaces; create configured ones |
| Track navigation outside Sesh | Automatic through plugin hooks | Requires a [`sesh-last` layout block](docs/herdr-app.md#switch-to-the-last-space) |
| Pane split ratios | Supported | Splits evenly; ignores `ratio` |

Remote workspace support is available only through the Herdr app integration,
including direct CLI commands targeting the app. Configured sessions and
directory entries create local workspaces in both integrations. With the app,
a configured workspace that names a `machine` creates its space on that machine.

## Features

- Search active Herdr workspaces, Sesh-style TOML sessions, and zoxide history
  from a native terminal picker, with name matches ahead of path-only matches
  and configurable home-directory priority.
- Inherit colors from Herdr TUI's theme configuration in both integrations,
  with a toggle to retain the picker's built-in colors.
- Group linked Git worktree workspaces beneath their open parent workspace,
  replace the Herdr sheep icon with a configurable purple `↳ herdr` type, and
  draw tree branches in the native picker.
- Focus an existing workspace or create one from a configured session or
  directory.
- Apply startup commands, previews, named Herdr tabs, and
  [pane layouts](docs/config/layouts.md#tabpane) to new workspaces.
- Toggle between command previews and live active-pane previews with `Ctrl+O`,
  and drag the divider to resize side-by-side previews.
- Filter, deduplicate, and optionally cache session results, with native
  [workspace, recent, and agent-priority sorting](docs/config/picker.md#picker).
- Jump directly to the previously focused workspace. Track navigation outside
  Sesh through TUI plugin hooks or the app's `sesh-last` layout block.
- Clone a Git repository and connect to it in one command.
- Use the built-in picker by default or opt into the experimental fzf picker.

Sesh concepts map onto Herdr as follows:

| Sesh | Herdr TUI | Herdr app |
| --- | --- | --- |
| Session | Workspace | Space |
| Window | Tab | Tab |
| Picker | Overlay pane | Mod dialog |

## Settings TUI

Press **F2** in the native picker (**Ctrl+,** when preview cycling uses F2).
In Herdr TUI, you can also invoke
`herdr plugin action invoke fullerzz.sesh.open-settings`. The editor uses the
same colors and Herdr theme inheritance as the picker.

From this checkout, run `just settings-demo` to try it with a disposable config,
or `just settings-demo /absolute/path/to/config.toml` to edit a real file.
The standalone command is `herdr-sesh config edit [--config PATH]` and does not
require a running Herdr instance.

**Enter** edits a setting; **Ctrl+S** applies text/list edits to the draft, then
opens the save review from the main form. Only **Y** in the review saves.
**Esc** cancels an edit or returns, with confirmation before discarding a draft.
Changes apply when returning to the picker. Workspaces, tabs, and rules remain
file-only. See [the settings editor guide](docs/config.md#settings-editor) for
migration, file preservation, and conflict handling.

## Requirements

These requirements apply to the Herdr TUI plugin. For the desktop app, follow
the [Herdr app setup](docs/herdr-app.md#install).

- [Herdr](https://herdr.dev/docs/installation/) 0.8.2 or newer
- Linux or macOS
- Git and Go 1.26.4 or newer for Herdr's source-based plugin installation
- Optional: `zoxide` for directory history and `eza` for the default preview
- Optional: `fzf` and `bat` for the experimental fzf picker

## Installation

For Herdr TUI, install the plugin directly from GitHub:

```bash
herdr plugin install fullerzz/herdr-plugin-sesh
```

Herdr previews the plugin manifest and build command before installation. To
skip the confirmation in a non-interactive environment, add `--yes`. To pin a
release, add `--ref <release-tag>` using a tag from the
[GitHub releases](https://github.com/fullerzz/herdr-plugin-sesh/releases) page.

This repository is also discoverable through the
[Herdr plugin marketplace](https://herdr.dev/plugins/) via the `herdr-plugin`
GitHub topic. Marketplace listings are automatic and are not endorsements or
security reviews.

### Herdr app

The [Herdr app](https://herdr.dev) uses mods instead of plugins. Install the
`sesh` and `sesh-last` mods from this repository with `just install-app-mod`,
then bind `mod.open:sesh` to open the picker and `mod.open:sesh-last` to switch
to the last space. See [Herdr app](docs/herdr-app.md).

## Quick start

In Herdr TUI, open the picker through the installed plugin action:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

For the desktop app, use [`mod.open:sesh`](docs/herdr-app.md#install).

You can also open its overlay pane directly:

```bash
herdr plugin pane open \
  --plugin fullerzz.sesh \
  --entrypoint picker \
  --placement overlay
```

See [Keybindings](docs/keybindings.md) to bind the picker and previous-workspace
actions in your Herdr configuration.

## Configuration

Configuration is optional. Without a config file, the picker still includes
running Herdr workspaces and zoxide results when zoxide is available.

`herdr-sesh` reads its versioned native TOML config from the first available
location:

1. `--config PATH`
2. `HERDR_SESH_CONFIG`
3. `${HERDR_PLUGIN_CONFIG_DIR}/config.toml`
4. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml` as a legacy fallback
5. `~/.config/herdr-sesh/config.toml`
6. `~/.config/herdr-sesh/sesh.toml` as a legacy fallback
7. `~/.config/sesh/sesh.toml` as a legacy fallback

Legacy Sesh-compatible files still load during the migration period and print a
deprecation warning on stderr. Explicit paths may hold either schema; a
top-level `version` key selects native decoding.

For the TUI plugin, ask Herdr for the managed configuration directory:

```bash
herdr plugin config-dir fullerzz.sesh
```

For the app mods, use `~/.config/herdr-sesh/config.toml` or set
`HERDR_SESH_CONFIG` to an explicit path.

See the [configuration reference](docs/config.md) for supported settings and a
complete example.

## CLI

The shared binary also exposes its underlying operations directly. Commands
target either the TUI server or the app, according to the
[backend selection rules](docs/commands.md#choose-the-herdr-backend).

| Command | Purpose |
| --- | --- |
| `herdr-sesh picker` | Open the native workspace picker. |
| `herdr-sesh picker --fzf` | Open the experimental fzf picker. |
| `herdr-sesh list --json` | List merged session sources as JSON. |
| `herdr-sesh connect TARGET` | Focus or create a workspace for a name, path, or ID. |
| `herdr-sesh last` | Switch to the previous workspace using the selected integration's history. |
| `herdr-sesh preview TARGET` | Render the configured preview for a session. |
| `herdr-sesh clone REPOSITORY` | Clone a repository and connect to its workspace. |
| `herdr-sesh root --connect` | Connect to the current Git repository root. |
| `herdr-sesh window [PATH]` | List tabs or create one for a path. |
| `herdr-sesh config path` | Print the resolved plugin config path. |
| `herdr-sesh config init` | Create a starter config if one does not exist. |
| `herdr-sesh config edit [--config PATH]` | Edit global settings with a review before saving. |
| `herdr-sesh config validate [PATH]` | Validate the active or specified config and print its resolved path. |
| `herdr-sesh config migrate` | Convert a legacy Sesh-style config to the native format. |

For the TUI plugin, the binary lives inside Herdr's managed plugin checkout
after installation; plugin actions are the normal entry points for day-to-day use.
The app mods require `herdr-sesh` on your login shell's `PATH`.

TUI previous-workspace history depends on plugin-managed state and the active
Herdr session. Use `herdr plugin action invoke fullerzz.sesh.last` unless that
environment is already established. The app uses separate history and the
[`sesh-last` mod](docs/herdr-app.md#switch-to-the-last-space).

## Local development

See the [contributor guide](docs/development/index.md) for setup, the code map,
verification, and the documentation workflow.

Tool versions are pinned in [`mise.toml`](mise.toml), and common tasks live in
the [`justfile`](justfile).

```bash
mise install
just check
just install-plugin
```

`just install-plugin` builds the binary and links the current checkout into
Herdr. Local builds (`just build` and `just install-plugin`) embed a Git-derived
version, for example `v0.10.1-5-g0686c01`: the nearest reachable version tag,
commits since that tag, and the commit hash. Uncommitted tracked changes append
`-dirty`; an exact clean tag uses the tag alone. Without a reachable version tag,
the commit hash is used; without Git metadata, the version falls back to `dev`.
Release builds continue to use the release version.

Verify the local plugin with:

```bash
herdr plugin action list --plugin fullerzz.sesh
herdr plugin log list --plugin fullerzz.sesh
```

## Release

Release tags must begin with `v` and match `version` in
[`herdr-plugin.toml`](herdr-plugin.toml). Create and publish a release with:

```bash
just release vX.Y.Z
```

The recipe must run from `main` with a clean working tree. It validates the tag
against the manifest version, runs the repository checks and CLI smoke tests,
generates `CHANGELOG.md`, tags the release source commit, commits the changelog
as a follow-up, and atomically pushes `main` and the tag.

## License

Licensed under the [MIT License](LICENSE).

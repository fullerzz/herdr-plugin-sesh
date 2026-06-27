# herdr-plugin-sesh

Sesh-style smart workspace/session management for Herdr.

This plugin maps Sesh concepts onto Herdr:

- Sesh session -> Herdr workspace
- Sesh window -> Herdr tab
- Sesh picker -> Herdr overlay pane

## Build

```bash
mise install
just check
just build
```

## Install for local use

Build the plugin binary, link this checkout into Herdr, then initialize the
plugin-owned config file:

```bash
just install-plugin
herdr plugin config-dir fullerzz.sesh
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config init
```

Herdr creates the plugin config and state directories during `plugin link`.
`herdr-sesh config init` writes `sesh.toml` under `HERDR_PLUGIN_CONFIG_DIR`
when Herdr invokes the plugin; when run manually it falls back to
`~/.config/herdr-sesh/sesh.toml`.

Verify the linked plugin:

```bash
herdr plugin action list --plugin fullerzz.sesh
herdr plugin action invoke fullerzz.sesh.open-picker
herdr plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay
herdr plugin log list --plugin fullerzz.sesh
```

## Commands

```bash
herdr-sesh list --json
herdr-sesh connect /path/to/project
herdr-sesh preview /path/to/project
herdr-sesh clone git@github.com:owner/repo.git
herdr-sesh root --connect
herdr-sesh last
herdr-sesh window
herdr-sesh window /path/to/project
herdr-sesh plugin open-picker
herdr-sesh picker
herdr-sesh picker --fzf
herdr-sesh config path
herdr-sesh config init
```

Use `herdr-sesh picker --fzf` or `HERDR_SESH_PICKER=fzf herdr-sesh picker`
to try the fzf-backed picker prototype. The native picker defaults to
`eza --icons=always -la {}` and honors configured `preview_command` values; the fzf
prototype uses a `bat` preview for items with an existing directory path.

## Configuration

The plugin reads Sesh-compatible TOML from:

1. `--config PATH`
2. `HERDR_SESH_CONFIG`
3. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml`
4. `~/.config/sesh/sesh.toml` as a migration fallback

Initialize a starter config:

```bash
herdr-sesh config init
```

For Herdr-managed plugin config, use the directory printed by:

```bash
herdr plugin config-dir fullerzz.sesh
```

## Repository

Canonical source lives at <https://github.com/fullerzz/herdr-plugin-sesh>.
Herdr's managed `plugin install` command accepts GitHub shorthand sources; use
the `herdr-plugin` topic and matching `v*` tags for Herdr marketplace
discovery.

## Release

Release tags must start with `v` and match `version` in `herdr-plugin.toml`.
Before tagging, run the same validation expected in CI:

```bash
just check
just build
./bin/herdr-sesh --version
./bin/herdr-sesh list --json --config testdata/sesh.toml
```

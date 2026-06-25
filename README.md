# herdr-plugin-sesh

Sesh-style smart workspace/session management for Herdr.

This plugin maps Sesh concepts onto Herdr:

- Sesh session -> Herdr workspace
- Sesh window -> Herdr tab
- Sesh picker -> Herdr overlay pane

## Build

```bash
go test ./...
go build -o bin/herdr-sesh ./cmd/herdr-sesh
```

## Install for local use

Build the plugin binary, link this checkout into Herdr, then initialize the
plugin-owned config file:

```bash
go build -o bin/herdr-sesh ./cmd/herdr-sesh
herdr plugin link "$PWD"
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
herdr-sesh root --connect
herdr-sesh last
herdr-sesh plugin open-picker
herdr-sesh picker --fzf
```

Use `herdr-sesh picker --fzf` or `HERDR_SESH_PICKER=fzf herdr-sesh picker`
to try the fzf-backed picker prototype. Existing Herdr workspaces show a
right-side preview powered by `bat`.

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

## Forgejo

Canonical source is intended to live in the user's self-hosted Forgejo instance.
Herdr's managed `plugin install` command accepts GitHub shorthand sources, so a
public GitHub mirror should use the `herdr-plugin` topic and matching `v*` tags
if this plugin is published through Herdr marketplace discovery.

## Release

Release tags must start with `v` and match `version` in `herdr-plugin.toml`.
Before tagging, run the same validation as the Forgejo workflow:

```bash
gofmt -l .
go vet ./...
go test ./...
go build -o bin/herdr-sesh ./cmd/herdr-sesh
./bin/herdr-sesh --version
./bin/herdr-sesh list --json --config testdata/sesh.toml
```

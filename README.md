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

## Commands

```bash
herdr-sesh list --json
herdr-sesh connect /path/to/project
herdr-sesh preview /path/to/project
herdr-sesh root --connect
herdr-sesh last
herdr-sesh plugin open-picker
```

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

## Forgejo

Canonical source is intended to live in the user's self-hosted Forgejo instance.

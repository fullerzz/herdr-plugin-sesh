---
icon: lucide/terminal
---

# Commands

The same `herdr-sesh` binary serves the Herdr TUI plugin and the Herdr app
mods. For daily use in Herdr TUI, invoke the installed plugin actions:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
herdr plugin action invoke fullerzz.sesh.last
herdr plugin action invoke fullerzz.sesh.open-settings
```

In the Herdr app, bind `mod.open:sesh` for the picker and `mod.open:sesh-last`
for the previous space. See [Herdr app setup](herdr-app.md#install).

## Choose the Herdr backend

Workspace commands target one host at a time:

- `HERDR_SESH_BACKEND=herdr` selects the Herdr TUI server.
- `HERDR_SESH_BACKEND=app` selects the Herdr app.
- Without an override, a Herdr TUI socket (`HERDR_SOCKET_PATH`) selects the
  TUI server. Otherwise, an hc terminal socket (`HC_TERMINAL_SOCKET`) selects
  the app. With neither socket set, commands use the TUI server.

The app mods explicitly select the app backend. Remote workspace support is
available only with that backend; the TUI plugin cannot list or select the
app's remote spaces. For example, to select an existing remote space:

```bash
HERDR_SESH_BACKEND=app herdr-sesh connect buntu26/sp1
```

Use the qualified space ID from `herdr-app ls`. Sesh cannot create remote
spaces from configured sessions. See
[Remote spaces](herdr-app.md#select-an-existing-remote-space) for registration
and preview behavior.

## Run the binary directly

The following shell setup is for the Herdr TUI plugin. App mod users put
`herdr-sesh` on their login shell's `PATH` during [app setup](herdr-app.md#install)
and can invoke it directly with `HERDR_SESH_BACKEND=app`.

The installed TUI plugin binary is inside Herdr's managed checkout; installation
does not put `herdr-sesh` on your shell's `PATH`. Set up the following variables
in the shell where you will run the examples. The installed-plugin example
requires `jq`.

=== "Installed plugin"

    ```bash
    sesh_root="$(herdr plugin list --plugin fullerzz.sesh --json | jq -r '.result.plugins[0].plugin_root')"
    sesh_bin="$sesh_root/bin/herdr-sesh"
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    ```

=== "Local checkout"

    ```bash
    just build
    sesh_bin="$PWD/bin/herdr-sesh"
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    ```

Run TUI workspace operations from the intended Herdr session so they inherit its
socket context. `HERDR_SESH_CONFIG`, when set, still overrides the plugin config
directory.

!!! note "Herdr TUI history requires the plugin environment"

    These shell examples set the config directory, but do not supply
    `HERDR_PLUGIN_STATE_DIR`. Running `last` directly with that variable unset
    returns `no previous workspace recorded`, even when the installed plugin
    has history. Use `herdr plugin action invoke fullerzz.sesh.last` to inherit
    the plugin's state and session context. Use the `open-picker` action above
    when you need the picker's history footer and recent sorting as well.

    The app backend uses separate history under
    `$XDG_STATE_HOME/herdr-sesh/herdr-app/` (default `~/.local/state`). It does
    not require `HERDR_PLUGIN_STATE_DIR`. Recording app navigation outside
    Sesh requires a [`sesh-last` layout block](herdr-app.md#switch-to-the-last-space).

## Command reference

Use `"$sesh_bin"` after the TUI shell setup, or `herdr-sesh` from the app's
`PATH`, followed by a command from this table:

| Command | Purpose |
| --- | --- |
| `--version` or `version` | Print the build version. |
| `picker [--config PATH] [--fzf] [--json]` | Open the native picker, use the experimental fzf picker, or print selectable sessions as JSON. |
| `list [--config PATH] [--json] [--blacklisted] [--hide-duplicates=false]` | List merged session sources, including optional blacklist and duplicate views. |
| `connect [--config PATH] [--no-focus] TARGET` | Focus or create a workspace by name, path, or workspace ID; `--no-focus` preserves TUI focus or restores the app's previously shown tab after creation. |
| `last` | Switch to the most recent other open workspace in the selected backend's history. |
| `preview [--config PATH] TARGET` | Run the configured command preview for a target. |
| `clone [--cmdDir PATH] [--dir PATH] REPOSITORY` | Clone a repository and connect to it, optionally choosing Git's working directory and the destination. |
| `root [--connect]` | Print the current Git repository root, or connect to it. |
| `window [PATH]` | List tabs, or create a tab for a path. |
| `config path` | Print the resolved config path, or the destination when none exists; a missing explicit override is an error. |
| `config init` | Create a native starter config only when no config exists. |
| `config edit [--config PATH]` | Open the global settings editor; review and confirm before saving. |
| `config validate [PATH]` | Validate the active or specified configuration. |
| `config migrate [PATH] [--force]` | Convert the active or specified legacy Sesh config without deleting its source. |
| `config migrate --config PATH [--force]` | Convert a legacy Sesh config selected with the equivalent explicit-path flag. |

For example:

```bash
"$sesh_bin" list --json
"$sesh_bin" connect my-project
"$sesh_bin" config validate
```

## Configuration commands

Commands follow the [configuration lookup order](config.md#configuration-file-lookup),
not the current directory. After the [shell setup](#run-the-binary-directly), use
`"$sesh_bin"` with:

- `config path`: print the active file, or the native `config.toml` destination
  when none exists. If `HERDR_SESH_CONFIG` selects a missing file, the command
  errors instead; the explicit override must exist.
- `config init`: create a native starter only when no config exists anywhere in
  the lookup order. Otherwise print the existing path, including legacy files.
  A missing `HERDR_SESH_CONFIG` path is created at that exact location.
- `config validate [PATH]`: strictly validate the active or specified file and
  print its resolved path. No file is an error; legacy files remain valid with
  a deprecation warning.
- `config edit --config PATH`: open a particular file in the settings editor.
  A missing path can be created after review and confirmation, unlike normal
  loading, which rejects missing explicit paths.

For conversion and overwrite rules, see [Legacy migration](config.md#legacy-migration).

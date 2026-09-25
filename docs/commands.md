---
icon: lucide/terminal
---

# Commands

For daily use, open the picker or return to the previous workspace through
Herdr's installed actions:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
herdr plugin action invoke fullerzz.sesh.last
herdr plugin action invoke fullerzz.sesh.open-settings
```

## Run the binary directly

The installed binary is inside Herdr's managed plugin checkout; installation
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

Run workspace operations from the intended Herdr session so they inherit its
socket context. `HERDR_SESH_CONFIG`, when set, still overrides the plugin config
directory.

!!! note "Use the Herdr action for previous-workspace history"

    These shell examples set the config directory, but do not supply
    `HERDR_PLUGIN_STATE_DIR`. Running `last` directly with that variable unset
    returns `no previous workspace recorded`, even when the installed plugin
    has history. Use `herdr plugin action invoke fullerzz.sesh.last` to inherit
    the plugin's state and session context. Use the `open-picker` action above
    when you need the picker's history footer and recent sorting as well.

## Command reference

Use `"$sesh_bin"` followed by a command from this table:

| Command | Purpose |
| --- | --- |
| `--version` or `version` | Print the build version. |
| `picker [--config PATH] [--fzf] [--json]` | Open the native picker, use the experimental fzf picker, or print selectable sessions as JSON. |
| `list [--config PATH] [--json] [--blacklisted] [--hide-duplicates=false]` | List merged session sources, including optional blacklist and duplicate views. |
| `connect [--config PATH] [--no-focus] TARGET` | Focus or create a workspace by name, path, or workspace ID; `--no-focus` preserves the current focus. |
| `preview [--config PATH] TARGET` | Run the configured command preview for a target. |
| `clone [--cmdDir PATH] [--dir PATH] REPOSITORY` | Clone a repository and connect to it, optionally choosing Git's working directory and the destination. |
| `root [--connect]` | Print the current Git repository root, or connect to it. |
| `window [PATH]` | List tabs, or create a tab for a path. |
| `config path` | Print the resolved config path, or the destination when none exists. |
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

Consult [Configuration](config.md) before migrating. Use its lookup order to
check which file is active; do not assume the current working directory selects
the configuration.

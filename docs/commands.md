---
icon: lucide/terminal
---

# Commands

For daily use, open the picker or return to the previous workspace through
Herdr's installed actions:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
herdr plugin action invoke fullerzz.sesh.last
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
| `--version` | Print the build version. |
| `picker` | Open the native picker. |
| `picker --fzf` | Open the experimental picker; requires fzf. |
| `list --json` | List merged session sources as JSON. |
| `list --blacklisted` | Show only blacklisted results. |
| `list --hide-duplicates=false` | Keep duplicate results. |
| `connect TARGET` | Focus or create a workspace by name, path, or workspace ID. |
| `preview TARGET` | Run the configured command preview for a target. |
| `clone REPOSITORY` | Clone a repository and connect to its workspace. |
| `root --connect` | Connect to the current Git repository root. |
| `window [PATH]` | List tabs, or create a tab for a path. |
| `config path` | Print the resolved config path, or the destination when none exists. |
| `config init` | Create a native starter config only when no config exists. |
| `config validate [PATH]` | Validate the active or specified configuration. |
| `config migrate` | Convert a legacy Sesh config without deleting its source. |

For example:

```bash
"$sesh_bin" list --json
"$sesh_bin" connect my-project
"$sesh_bin" config validate
```

Consult [Configuration](config.md) before migrating. Use its lookup order to
check which file is active; do not assume the current working directory selects
the configuration.

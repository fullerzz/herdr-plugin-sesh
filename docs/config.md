# Configuration

`herdr-sesh` intentionally accepts the core Sesh TOML shape so existing Sesh users can migrate gradually.

Lookup order:

1. `--config PATH`
2. `HERDR_SESH_CONFIG`
3. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml`
4. `~/.config/sesh/sesh.toml`

For a linked Herdr plugin, create or edit the plugin-owned config file with:

```bash
just install-plugin
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config init
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config path
```

Herdr creates `HERDR_PLUGIN_CONFIG_DIR` and `HERDR_PLUGIN_STATE_DIR` for the
plugin. Keep user-editable configuration in the config directory and runtime
state in the state directory; do not rely on the linked or installed plugin root
for durable user data.

Supported keys include:

- `cache`
- `strict_mode`
- `import`
- `blacklist`
- `sort_order`
- `dir_length`
- `separator_aware`
- `tmux_command`
- `[tui]`
- `[default_session]`
- `[[session]]`
- `[[window]]`
- `[[wildcard]]`

Example:

```toml
[default_session]
preview_command = "eza --icons=always -la {}"

[[session]]
name = "brain"
path = "~/brain"
startup_command = "git status"
disable_startup_command = false
windows = ["git"]

[[window]]
name = "git"
startup_script = "git status"

[[wildcard]]
pattern = "~/projects/**"
preview_command = "eza --icons=always -la {}"
```

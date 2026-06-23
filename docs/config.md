# Configuration

`herdr-sesh` intentionally accepts the core Sesh TOML shape so existing Sesh users can migrate gradually.

Lookup order:

1. `--config PATH`
2. `HERDR_SESH_CONFIG`
3. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml`
4. `~/.config/sesh/sesh.toml`

Supported keys include:

- `cache`
- `strict_mode`
- `import`
- `blacklist`
- `sort_order`
- `dir_length`
- `separator_aware`
- `[tui]`
- `[default_session]`
- `[[session]]`
- `[[window]]`
- `[[wildcard]]`

Example:

```toml
[default_session]
preview_command = "ls -la {}"

[[session]]
name = "brain"
path = "~/brain"
startup_command = "git status"
windows = ["git"]

[[window]]
name = "git"
startup_script = "git status"

[[wildcard]]
pattern = "~/projects/**"
preview_command = "ls -la {}"
```

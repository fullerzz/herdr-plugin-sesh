# Configuration

`herdr-sesh` accepts a deliberate subset of Sesh TOML. The fields below are the
implemented contract; tmux-specific Sesh settings are not supported by Herdr.

Lookup order:

1. `--config PATH`
2. `HERDR_SESH_CONFIG`
3. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml`
4. `~/.config/sesh/sesh.toml`

For a linked Herdr plugin, create or inspect the plugin-owned config with:

```bash
just install-plugin
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config init
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config path
```

Herdr creates `HERDR_PLUGIN_CONFIG_DIR` and `HERDR_PLUGIN_STATE_DIR` for the
plugin. Keep user configuration in the config directory and runtime state in
the state directory.

## Top-level fields

| Field | Runtime effect |
| --- | --- |
| `cache` | Caches normal deduplicated `list` results for five seconds in `HERDR_PLUGIN_STATE_DIR`, scoped to the resolved config file. It does not cache `list --blacklisted`, `list --hide-duplicates=false`, `picker`, or `connect`. |
| `strict_mode` | Rejects unknown fields in this file and its imported files. Without strict mode, unknown fields are ignored. |
| `import` | Loads additional TOML files before the current file. Relative paths are resolved from the importing file; `~/` is expanded. |
| `blacklist` | Treats each value as a regular expression matched against session names. Normal listings hide matches; `list --blacklisted` shows them. |
| `sort_order` | Orders sources such as `herdr`, `config`, `zoxide`, and `dir`. Sources omitted from the list are appended. |
| `dir_length` | Sets the number of path components used by the directory-name fallback for a newly created direct-path workspace. Git repositories keep their repository-derived name. The default and minimum effective value are `1`. |
| `separator_aware` | Makes native and fzf picker searches treat `-`, `_`, `/`, and `.` as spaces. |

## Picker fields

`[tui]` supports:

| Field | Runtime effect |
| --- | --- |
| `show_icons` | Shows Nerd Font source icons in the native picker. The default is `false`; source names remain visible when icons are hidden. |
| `prompt` | Replaces the picker prompt. An empty value uses `Sesh> `. |
| `placeholder` | Replaces the picker placeholder. An empty value uses `Filter workspaces`. |

## Session behavior

`[default_session]` supports only:

| Field | Runtime effect |
| --- | --- |
| `startup_command` | Fallback command run after a new Herdr workspace is created. `{}` is replaced with the session path. |
| `preview_command` | Fallback command used by `preview` and the native picker. `{}` is replaced with the session path. |

`[[session]]` supports:

| Field | Runtime effect |
| --- | --- |
| `name` | Session label and connect target. |
| `path` | Workspace path; `~/` is expanded before it is sent to Herdr. |
| `startup_command` | Session-specific startup command. |
| `preview_command` | Session-specific preview command. |
| `disable_startup_command` | Suppresses startup execution when `true`. |
| `windows` | Names of `[[window]]` entries to create as Herdr tabs. |

Startup commands are selected in this order: the explicit session command, the
first matching wildcard command, then `[default_session].startup_command`.
Preview commands use the same explicit session, wildcard, then default order.

`[[window]]` supports:

| Field | Runtime effect |
| --- | --- |
| `name` | Name referenced by a session or wildcard `windows` list and used as the Herdr tab label. |
| `path` | Optional tab working directory. Without it, the session path is used; `~/` is expanded. |
| `startup_script` | Command run in the new tab. `{}` is replaced with that tab's working directory. |

## Wildcards

Wildcard startup, preview, and disable settings apply to every matching session
when the corresponding explicit session field is unset. Wildcard windows apply
only to discovered or direct-path sessions. The first matching wildcard wins.

| Field | Runtime effect |
| --- | --- |
| `pattern` | Path glob. `*`, `?`, and character classes use `filepath.Match` semantics; a trailing `/**` matches the base directory and all descendants. |
| `startup_command` | Startup command for a matching path. |
| `preview_command` | Preview command for a matching path. |
| `disable_startup_command` | Suppresses wildcard and default startup behavior for a matching path when `true`. |
| `windows` | `[[window]]` entries created for a matching discovered or direct-path session, not a configured session. |

## Unsupported Sesh fields

`tmux_command`, `tmuxp`, and `tmuxinator` have no Herdr equivalent and are not
supported. `windows` is supported on `[[session]]` and `[[wildcard]]`, but not
under `[default_session]`. These and any other unknown fields are rejected when
`strict_mode = true`; otherwise they are ignored rather than changing runtime
behavior. The generated starter config therefore does not reference Sesh's
broader JSON schema.

## Example

```toml
cache = true
strict_mode = true
sort_order = ["herdr", "config", "zoxide", "dir"]
dir_length = 1
separator_aware = true
blacklist = ["^scratch$"]

[tui]
show_icons = true
prompt = "Sesh> "
placeholder = "Search workspaces"

[default_session]
startup_command = "git status"
preview_command = "eza --icons=always -la {}"

[[window]]
name = "git"
startup_script = "git status"

[[session]]
name = "brain"
path = "~/brain"
disable_startup_command = true
windows = ["git"]

[[wildcard]]
pattern = "~/projects/**"
startup_command = "git status"
preview_command = "eza --icons=always -la {}"
windows = ["git"]
```

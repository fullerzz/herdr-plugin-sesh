# Configuration

`herdr-sesh` reads a versioned native TOML config. Every native file starts
with `version = 1` and unknown keys are always rejected. Legacy
Sesh-compatible files (no `version` key) still load during the migration
period and print a deprecation warning on stderr; see
[Legacy migration](#legacy-migration).

Lookup order:

1. `--config PATH`
2. `HERDR_SESH_CONFIG` (an error if the file does not exist)
3. `${HERDR_PLUGIN_CONFIG_DIR}/config.toml`
4. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml` as a legacy fallback
5. `~/.config/herdr-sesh/config.toml`
6. `~/.config/herdr-sesh/sesh.toml` as a legacy fallback
7. `~/.config/sesh/sesh.toml` as a legacy fallback

Explicit paths (`--config`, `HERDR_SESH_CONFIG`) may hold either schema: a
top-level `version` key selects native decoding, otherwise the file is treated
as legacy. `config path` prints the file that would load, or the native
`config.toml` destination when none exists. `config init` writes a native
starter file only when no config exists anywhere in the lookup order; an
existing config (legacy included) is printed instead so init can never shadow
it. With `HERDR_SESH_CONFIG` set to a missing path, init creates the starter
at that exact path. `config validate [PATH]` strictly validates the active or
specified config and prints its resolved path on success. It returns an error
when no config exists; legacy files remain valid but emit the migration warning.

For a linked Herdr plugin, create or inspect the plugin-owned config with:

```bash
just install-plugin
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config init
HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)" ./bin/herdr-sesh config path
```

Herdr creates `HERDR_PLUGIN_CONFIG_DIR` and `HERDR_PLUGIN_STATE_DIR` for the
plugin. Keep user configuration in the config directory and runtime state in
the state directory.

## `[list]`

| Field | Runtime effect |
| --- | --- |
| `cache` | Caches normal deduplicated `list` results for five seconds in `HERDR_PLUGIN_STATE_DIR`, scoped to the resolved config file. It does not cache `list --blacklisted`, `list --hide-duplicates=false`, `picker`, or `connect`. |
| `source_order` | Orders sources among `herdr`, `config`, `zoxide`, and `dir`. Unknown or duplicated names are rejected; sources omitted from the list are appended. |
| `blacklist` | Treats each value as a regular expression matched against workspace names. Normal listings hide matches; `list --blacklisted` shows them. Invalid regexes are rejected. |

## `[naming]`

| Field | Runtime effect |
| --- | --- |
| `path_components` | Sets the number of path components used by the directory-name fallback for a newly created direct-path workspace. Git repositories keep their repository-derived name. Must be at least `1` (the default). |

## `[picker]`

| Field | Runtime effect |
| --- | --- |
| `show_icons` | Shows Nerd Font source icons in the native picker. The default is `false`; source names remain visible when icons are hidden. |
| `herdr_theme_inherit` | Inherits colors from Herdr's active theme. The default is `true`; set it to `false` to keep the native picker's built-in colors. |
| `replace_worktree_icon` | Replaces the Herdr sheep icon with `↳` for linked worktree rows. The default is `true`. Set it to `false` to keep the sheep icon (or plain `[herdr]` when icons are hidden); the purple type color and tree branches remain. |
| `prompt` | Replaces the picker prompt. An empty value uses `Sesh> `. |
| `placeholder` | Replaces the picker placeholder. An empty value uses `Filter workspaces`. |
| `separator_aware` | Makes native and fzf picker searches treat `-`, `_`, `/`, and `.` as spaces. |
| `workspace_sort` | Sets the native picker's initial Herdr workspace order to `workspace` (Herdr's order, the default) or `recent` (most recently visited first). Press `ctrl+r` to switch modes while the picker is open. |
| `show_last_workspace` | Shows the workspace targeted by `herdr-sesh last` in the picker footer. The default is `true`; set it to `false` to disable the feature. |
| `show_last_workspace_path` | Shows the Herdr workspace working directory beside the last workspace name. The default is `true`; set it to `false` to show only the workspace name. |

Set `HERDR_SESH_SMEAR_PRESET` to choose the cursor animation:

| Preset | Effect |
| --- | --- |
| `crisp` | Fast cyan rail with a short violet line trail. This is the default. |
| `gooey` | Slower block cursor with a longer shaded trail and eased movement. |
| `ghost` | Soft diamond cursor with a dotted, low-contrast trail. |

Set `HERDR_SESH_REDUCE_MOTION=1` (or `true`) to keep cursor movement
instantaneous without drawing any preset's trail.

Open Herdr workspaces show the agent state reported by Herdr: an animated amber
Jump spinner (`⢄⢂⢁⡁⡈⡐⡠`) while working, red `◉` when blocked, green `✓` when idle,
and teal `●` when done. Workspaces with an unknown state have no indicator.

## Picker colors

By default, the native picker inherits colors from Herdr's own theme so it
matches the running Herdr UI. Disable inheritance to keep the picker's built-in
colors:

```toml
[picker]
herdr_theme_inherit = false
```

When enabled, it reads the same config file Herdr uses (`HERDR_CONFIG_PATH`,
then `$XDG_CONFIG_HOME/herdr/config.toml`, then
`~/.config/herdr/config.toml`) and resolves `[theme] name` against Herdr's
built-in themes:

`catppuccin` (default), `catppuccin-latte`, `tokyo-night`, `tokyo-night-day`,
`dracula`, `nord`, `gruvbox`, `gruvbox-light`, `one-dark`, `one-light`,
`solarized`, `solarized-light`, `kanagawa`, `kanagawa-lotus`, `rose-pine`,
`rose-pine-dawn`, and `vesper`. Common aliases (`catppuccin-mocha`,
`tokyonight`, `onedark`, …) are accepted, as are `[theme.custom]` overrides on
top of any base theme.

| Herdr token | Picker role |
| --- | --- |
| `accent` | Prompt, cursor, and selection rail |
| `mauve` | Title, section headers, search matches, smear trail |
| `text` | Row labels |
| `subtext0` | Paths, counts, help text |
| `green` | Idle agents (`✓`) |
| `yellow` | Working agents (spinner) and the empty-state message |
| `red` | Blocked agents (`◉`) |
| `overlay1` | Ghost cursor trail |

Custom tokens that are unknown or not a `#RGB`/`#RRGGBB` hex value leave that
role's built-in color in place, so partial `[theme.custom]` tables only affect
the roles they define. The ANSI-based `terminal` theme has no fixed palette to
inherit; the picker keeps its built-in colors there unless you add explicit
overrides.

The native picker marks a linked Git worktree workspace with a purple
`↳ herdr` type label, replacing the normal Herdr sheep icon, and groups it
immediately beneath its open parent workspace in both workspace and recent sort
modes, matching Herdr's sidebar. With icons disabled, the label is `[↳ herdr]`.
When the parent is visible, `├─` and `└─` branches reinforce the family in the
workspace-name column. Wide layouts add `worktree of <parent>` and the worktree
path when space permits; narrow layouts retain the purple type label. This is
automatic and does not depend on `show_icons`. Set
`picker.replace_worktree_icon = false` to retain the normal sheep icon or plain
`[herdr]` label while keeping the other child-worktree cues. If Herdr reports a
linked worktree but no single open parent can be resolved, the picker shows
`linked worktree` without inventing or grouping under a parent.

## `[workspace_defaults]`

| Field | Runtime effect |
| --- | --- |
| `startup` | Fallback command run after a new Herdr workspace is created. `{}` is replaced with the workspace path. |
| `preview` | Fallback command used by `preview` and the native picker. `{}` is replaced with the workspace path. Absent or empty values use the built-in `eza` preview. |

## `[[workspace]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Workspace label and connect target. Must be non-empty and unique. |
| `path` | Workspace path; `~/` is expanded before it is sent to Herdr. Must be non-empty. |
| `startup` | Workspace-specific startup command. |
| `preview` | Workspace-specific preview command. |
| `disable_startup` | Suppresses startup execution when `true`. |
| `tabs` | Names of `[[tab]]` entries to create as Herdr tabs. Every referenced tab must exist. |

Startup commands are selected in this order: the explicit workspace command,
the first matching rule command, then `workspace_defaults.startup`. Preview
commands use the same explicit workspace, rule, then default order.

## `[[tab]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Name referenced by a workspace or rule `tabs` list and used as the Herdr tab label. Must be non-empty and unique. |
| `path` | Optional tab working directory. Without it, the workspace path is used; `~/` is expanded. |
| `startup` | Command run in the new tab. `{}` is replaced with that tab's working directory. |

## `[[rule]]`

Rule startup, preview, and disable settings apply to every matching workspace
when the corresponding explicit workspace field is unset. Rule tabs apply only
to discovered or direct-path workspaces. The first matching rule wins.

| Field | Runtime effect |
| --- | --- |
| `path_glob` | Path glob. `*`, `?`, and character classes use `filepath.Match` semantics; a trailing `/**` matches the base directory and all descendants. Must be non-empty and compile. |
| `startup` | Startup command for a matching path. |
| `preview` | Preview command for a matching path. |
| `disable_startup` | Suppresses rule and default startup behavior for a matching path when `true`. |
| `tabs` | `[[tab]]` entries created for a matching discovered or direct-path workspace, not a configured workspace. |

## Example

```toml
version = 1

[list]
cache = true
source_order = ["herdr", "config", "zoxide", "dir"]
blacklist = ["^scratch$"]

[naming]
path_components = 1

[picker]
show_icons = true
herdr_theme_inherit = true
replace_worktree_icon = true
prompt = "Sesh> "
placeholder = "Search workspaces"
separator_aware = true
workspace_sort = "recent"
show_last_workspace = true
show_last_workspace_path = false

[workspace_defaults]
startup = "git status"
preview = "eza --icons=always --color=always -la {}"

[[tab]]
name = "git"
startup = "git status"

[[workspace]]
name = "brain"
path = "~/brain"
disable_startup = true
tabs = ["git"]

[[rule]]
path_glob = "~/projects/**"
startup = "git status"
preview = "eza --icons=always --color=always -la {}"
tabs = ["git"]
```

## Legacy migration

Legacy Sesh-compatible files keep loading for at least one released version.
Run `herdr-sesh config migrate` to convert the active legacy file
automatically. Conversion intentionally modernizes two defaults: when
`tui.show_icons` was never set, the native config enables icons; and the former
colorless default preview (`eza --icons=always -la {}`) is replaced by the
color-forced runtime default. Explicit icon settings and custom preview commands
are preserved. The command flattens any `import` files into a native
`config.toml`, leaves the legacy file untouched, and prints the new path.

For an installed plugin, invoke its managed binary directly:

```bash
"$(herdr plugin list --plugin fullerzz.sesh --json | jq -r '.result.plugins[0].plugin_root')/bin/herdr-sesh" config migrate
```

Pass `--config PATH` to convert a specific file. The command refuses to
overwrite an existing native file unless `--force` is passed; even with
`--force`, unrelated or invalid `config.toml` files are never replaced. The
native file is installed atomically with `0600` permissions. A specific file can
also be supplied positionally, for example
`herdr-sesh config migrate ~/.config/sesh/sesh.toml --force`. Values the native
schema rejects (invalid regexes, duplicate names, missing tab references) fail
with an error before anything is written. Comments and key order do not survive
conversion.
Delete the legacy file once the native one looks right. If
`HERDR_SESH_CONFIG` selects the legacy file, point it at the printed native path
before deleting the legacy file. A legacy file already named `config.toml`
cannot be migrated in place, even with `--force`; rename it first so migration
can leave the source untouched.

For manual migration, rename keys as follows; unlisted fields keep their
meaning under the renamed table.

| Legacy key | Native key |
| --- | --- |
| `cache` | `list.cache` |
| `strict_mode` | Removed; native decoding is always strict |
| `import` | Unsupported in native version 1 |
| `blacklist` | `list.blacklist` |
| `sort_order` | `list.source_order` |
| `dir_length` | `naming.path_components` |
| `separator_aware` | `picker.separator_aware` |
| `tui.show_icons` | `picker.show_icons` |
| `tui.herdr_theme_inherit` | `picker.herdr_theme_inherit` |
| `tui.replace_worktree_icon` | `picker.replace_worktree_icon` |
| `tui.show_last_workspace` | `picker.show_last_workspace` |
| `tui.show_last_workspace_path` | `picker.show_last_workspace_path` |
| `tui.prompt` | `picker.prompt` |
| `tui.placeholder` | `picker.placeholder` |
| `tui.default_sort` | `picker.workspace_sort` |
| `default_session.startup_command` | `workspace_defaults.startup` |
| `default_session.preview_command` | `workspace_defaults.preview` |
| `session[]` | `workspace[]` |
| `session[].startup_command` | `workspace[].startup` |
| `session[].preview_command` | `workspace[].preview` |
| `session[].disable_startup_command` | `workspace[].disable_startup` |
| `session[].windows` | `workspace[].tabs` |
| `window[]` | `tab[]` |
| `window[].startup_script` | `tab[].startup` |
| `window[].path` | `tab[].path` |
| `wildcard[]` | `rule[]` |
| `wildcard[].pattern` | `rule[].path_glob` |
| `wildcard[].startup_command` | `rule[].startup` |
| `wildcard[].preview_command` | `rule[].preview` |
| `wildcard[].disable_startup_command` | `rule[].disable_startup` |
| `wildcard[].windows` | `rule[].tabs` |

Note: a legacy file containing a stray top-level `version` key was silently
ignored before and now selects strict native decoding, which fails hard on the
remaining legacy keys. Remove the key or migrate the file.

Legacy `tmux_command`, `tmuxp`, and `tmuxinator` fields have no Herdr
equivalent; native decoding rejects them like any other unknown key.

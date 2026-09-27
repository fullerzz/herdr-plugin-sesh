---
icon: lucide/settings
---

# Configuration

`herdr-sesh` reads a versioned native TOML config. Every native file starts
with `version = 1` and unknown keys are always rejected. Legacy
Sesh-compatible files (no `version` key) still load during the migration
period and print a deprecation warning on stderr; see
[Legacy migration](#legacy-migration).

Configure a workspace once, then select it in the picker to open its named tabs
and split panes, each with its own working directory, environment, and command.
Layouts run when a workspace is **created**; reconnecting preserves your running
terminals.

| To configure… | Start here |
| --- | --- |
| Picker appearance, sorting, and shortcuts | [Picker](config/picker.md) |
| Edit settings in the picker | [Settings editor](#settings-editor) |
| A project with a named tab | [Add a workspace with a tab](#add-a-workspace-with-a-tab) |
| An editor, development server, and logs in split panes | [Pane layout walkthrough](config/layouts.md#pane-layout-walkthrough) |
| The same tabs for discovered projects under a directory | [Path rules](#rule) |

Workspace, tab, and pane definitions are edited in the TOML file. A
`[[workspace]]` selects reusable `[[tab]]` definitions through its `tabs` list;
each tab can contain ordered `[[tab.pane]]` entries describing its splits.

## Configuration file lookup

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

## Settings editor

Use **F2** in the native picker (**Ctrl+,** when preview cycling uses F2) or the
**Sesh Settings** Herdr action:

```bash
herdr plugin action invoke fullerzz.sesh.open-settings
```

For standalone use from a local checkout:

```bash
just build
./bin/herdr-sesh config edit
# Select a particular file instead:
./bin/herdr-sesh config edit --config /absolute/path/to/config.toml
```

The screen edits Picker, Lists (including source order and blacklist patterns),
Naming, Keys, and Workspace Defaults. Workspace, tab, and rule definitions remain
file-only. The editor shares the native picker's colors and honors
`picker.herdr_theme_inherit`; theme changes apply after saving.

Edits remain in memory until **Ctrl+S** opens a review and **Y** confirms it.
Within text and list editors, **Ctrl+S** first applies the edit to the draft.
Saving does not run startup or preview commands. Returning to the picker applies
persisted settings and refreshes its sessions, keeping the search and selection
when possible. Normal picker preview commands resume then.

### Files, defaults, and conflicts

The editor follows the lookup order above. If no config exists, it displays
defaults and the proposed path, creating the file only after confirmation. An
explicit missing path can be created through the editor; other commands retain
their existing missing-path behavior. Untouched defaults are not written out.

Only changed settings are patched. Unrelated definitions, comments, and line
endings are preserved. Edited arrays are reformatted; their comments are retained
but can move above the elements, which the review discloses. Native TOML tables,
inline tables, dotted/quoted keys, and multiline strings are supported.

Saves preserve existing file permission bits, use mode 0600 for new files, and
replace the target atomically. Symlinks are followed without replacing the link;
a changed target or changed file is rejected. A per-target `.settings.lock` file
coordinates settings writers and intentionally remains after exit. Advisory
locking cannot exclude arbitrary external editors.

A conflict keeps the draft. Choose **R** on the error screen to review a reload
confirmation; **Y** discards the draft and loads the current file. There is no
force overwrite or automatic merge. Other save errors also preserve the draft.
The plugin's session-list cache is invalidated when its state directory is
available; a cache cleanup failure is reported separately from a successful save.

### Legacy conversion

Opening a legacy config offers a separate migration review showing source and
destination. Conversion flattens imports and may normalize formatting/defaults.
Only confirmation creates the native file. Legacy files are preserved, existing
destinations are never overwritten, and source/import changes invalidate the
prepared conversion. Invalid configs are reported without automatic repair.

After conversion the editor and returning picker use the new native path. If
`HERDR_SESH_CONFIG` or an explicit argument still selects the legacy file on future
launches, follow the displayed path instruction; the editor does not change your
environment. Declining later draft edits does not undo a confirmed migration or save.

The form supports 80×24 and a compact 60×18 layout. Smaller terminals show a resize
message and safe exit controls. See [Settings controls](keybindings.md#settings-controls).

## Create your configuration

For a fresh installation, ask Herdr where this plugin keeps its configuration:

```bash
herdr plugin config-dir fullerzz.sesh
```

Use your editor to create `config.toml` in the printed directory. Herdr creates
the plugin directory during installation; you do not need `jq` or the plugin
binary to write your first config. If you already have a herdr-sesh or Sesh
config, check the [lookup order](#configuration-file-lookup) and
[legacy migration](#legacy-migration) before creating a file that could take
precedence over it.

Herdr keeps runtime state in a separate `HERDR_PLUGIN_STATE_DIR` directory.

### Add a workspace with a tab

Put this complete example in `config.toml`, replacing the path with an existing
Git checkout:

```toml
version = 1

[[tab]]
name = "git"
startup = "git status"

[[workspace]]
name = "my-project"
path = "/absolute/path/to/my-project"
tabs = ["git"]
```

If a native config already exists, add just the `[[tab]]` and `[[workspace]]`
entries, using unique names; keep its single `version = 1` line. Migrate a
legacy config before adding native entries.

Open the picker, search for `my-project`, and press ++enter++. A newly created
workspace receives the named `git` tab and runs `git status` there. Selecting
an existing workspace focuses it; it does not recreate its tabs or rerun startup
commands. For a new workspace, Herdr initially shows a separate shell tab;
select `git` to see the command. See [Initial tab behavior](config/layouts.md#initial-tab-behavior)
for when a configured tab can reuse that first tab.

To check the file from a shell before opening the picker, use `config validate`.
The installed plugin keeps its binary in Herdr's managed checkout, so that
option requires `jq` to locate it:

=== "Installed plugin"

    ```bash
    sesh_root="$(herdr plugin list --plugin fullerzz.sesh --json | jq -r '.result.plugins[0].plugin_root')"
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    "$sesh_root/bin/herdr-sesh" config validate
    ```

=== "Local checkout"

    From the repository root:

    ```bash
    just build
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    ./bin/herdr-sesh config validate
    ```

To open several panes inside a tab, follow the [pane layout walkthrough](config/layouts.md#pane-layout-walkthrough).

## Example

This is a customization example, not a dump of the defaults. Omitted settings
use the defaults described in [Settings](#settings) and [Picker](config/picker.md).

```toml
version = 1 # (1)!

[list]
cache = true
source_order = ["herdr", "config", "zoxide", "dir"] # (2)!
blacklist = ["^scratch$"]

[naming]
path_components = 1

[picker]
show_icons = true
show_path = true
show_preview = true
preview_mode = "command"
prioritize_home = false
herdr_theme_inherit = true
replace_worktree_icon = true
prompt = "Sesh> "
placeholder = "Search workspaces"
separator_aware = true
workspace_sort = "agent"
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
tabs = ["git"] # (3)!

[[rule]]
path_glob = "~/projects/**"
startup = "git status"
preview = "eza --icons=always --color=always -la {}"
tabs = ["git"]
```

1. Native configuration requires this schema version; unknown keys are rejected.
2. Source order controls how results are combined; picker sorting affects Herdr rows.
3. Tab names refer to `[[tab]]` definitions. They are created when the workspace is new.

## Legacy migration

Legacy Sesh-compatible files keep loading for at least one released version.
Run `config migrate` to convert the active legacy file automatically:

=== "Local checkout"

    ```bash
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    ./bin/herdr-sesh config migrate
    ```

=== "Installed plugin"

    ```bash
    sesh_root="$(herdr plugin list --plugin fullerzz.sesh --json | jq -r '.result.plugins[0].plugin_root')"
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    "$sesh_root/bin/herdr-sesh" config migrate
    ```

Conversion intentionally modernizes two defaults: when
`tui.show_icons` was never set, the native config enables icons; and the former
colorless default preview (`eza --icons=always -la {}`) is replaced by the
color-forced runtime default. Explicit icon settings and custom preview commands
are preserved. The command flattens any `import` files into a native
`config.toml`, leaves the legacy file untouched, and prints the new path.

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

??? info "Manual migration: legacy → native key reference"

    Rename keys as follows; unlisted fields keep their meaning under the
    renamed table.

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
    | `tui.default_sort` | `picker.workspace_sort` (`workspace`, `recent`, or `agent`) |
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

!!! warning "Stray `version` keys in legacy files"

    A legacy file containing a stray top-level `version` key was silently
    ignored before and now selects strict native decoding, which fails hard on
    the remaining legacy keys. Remove the key or migrate the file.

Legacy `tmux_command`, `tmuxp`, and `tmuxinator` fields have no Herdr
equivalent; native decoding rejects them like any other unknown key. Describe
tab splits with native [pane layouts](config/layouts.md#tabpane) instead.


## Settings

Picker shortcuts, appearance, sorting, and history are documented in
[Picker](config/picker.md).

### `[list]`

| Field | Runtime effect |
| --- | --- |
| `cache` | Caches normal deduplicated `list` results for five seconds in `HERDR_PLUGIN_STATE_DIR`, scoped to the resolved config file. It does not cache `list --blacklisted`, `list --hide-duplicates=false`, `picker`, or `connect`. |
| `source_order` | Orders sources among `herdr`, `config`, `zoxide`, and `dir`. Unknown or duplicated names are rejected; sources omitted from the list are appended. |
| `blacklist` | Treats each value as a regular expression matched against workspace names. Normal listings hide matches; `list --blacklisted` shows them. Invalid regexes are rejected. |

### `[naming]`

| Field | Runtime effect |
| --- | --- |
| `path_components` | Sets the number of path components used by the directory-name fallback for a newly created direct-path workspace. Git repositories keep their repository-derived name. Must be at least `1` (the default). |

### `[workspace_defaults]`

| Field | Runtime effect |
| --- | --- |
| `startup` | Fallback command run after a new Herdr workspace is created. `{}` is replaced with the workspace path. |
| `preview` | Fallback command used by `preview` and the native picker. `{}` is replaced with the workspace path. Absent or empty values use the built-in `eza` preview. |

### `[[workspace]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Workspace label and connect target. Must be non-empty and unique. |
| `path` | Workspace path; `~/` is expanded before it is sent to Herdr. Must be non-empty. |
| `startup` | Workspace-specific startup command. |
| `preview` | Workspace-specific preview command. |
| `disable_startup` | Suppresses the workspace startup command, including rule/default fallbacks, when `true`. Tab and pane startup commands still run. |
| `tabs` | Names of `[[tab]]` entries to create as Herdr tabs. Every referenced tab must exist. |

Startup commands are selected in this order: the explicit workspace command,
the first matching rule command, then `workspace_defaults.startup`. Preview
commands use the same explicit workspace, rule, then default order.

### Layouts

For named tabs, pane splits, startup behavior, and recovery, see
[Layouts](config/layouts.md).

### `[[rule]]`

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

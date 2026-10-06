---
icon: lucide/settings
---

# Configuration

`herdr-sesh` uses TOML configuration with `version = 1` and rejects unknown
keys. Older Sesh files still load with a deprecation warning; see
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

## Create your configuration

For a fresh installation, ask Herdr where this plugin keeps its configuration:

```bash
herdr plugin config-dir fullerzz.sesh
```

Create `config.toml` in the printed directory. If you already have a config,
check the [lookup order](#configuration-file-lookup) and
[migration instructions](#legacy-migration) before creating a higher-priority file.

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

Open the picker from Herdr:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

Search for `my-project` and press ++enter++. A newly created
workspace receives the named `git` tab and runs `git status` there. Selecting
an existing workspace focuses it; it does not recreate its tabs or rerun startup
commands. For a new workspace, Herdr initially shows a separate shell tab;
select `git` to see the command. See [Initial tab behavior](config/layouts.md#initial-tab-behavior)
for when a configured tab can reuse that first tab.

For optional shell validation, follow the [CLI setup](commands.md#run-the-binary-directly)
and run `"$sesh_bin" config validate`. For splits, continue with the
[pane layout walkthrough](config/layouts.md#pane-layout-walkthrough).

## Settings editor

Press **F2** in the native picker (**Ctrl+,** if preview cycling uses F2), or run:

```bash
herdr plugin action invoke fullerzz.sesh.open-settings
```

Edit Picker, Lists (source order and blacklist), Naming, Keys, and Workspace
Defaults here. Workspace, tab, pane, and rule definitions remain file-only.
For standalone use, see [`config edit`](commands.md#command-reference).

Press **Ctrl+S** to review changes, then **Y** to save. In text/list editors,
**Ctrl+S** first applies the edit to the draft. Saving runs no startup or preview
commands. Returning to the picker applies saved settings, refreshes sessions,
resumes previews, and preserves search/selection when possible. The editor uses
the picker's colors; theme changes apply after saving.

Save errors keep your draft. On a conflict, **R** opens a reload confirmation;
**Y** discards the draft and loads the current file. There is no force overwrite
or automatic merge.

The form supports 80×24 and compact 60×18 terminals; smaller sizes show resize
instructions and safe exit controls. See [Settings controls](keybindings.md#settings-controls).

### Files, defaults, and conflicts

??? info "File handling details"

    The editor follows the [lookup order](#configuration-file-lookup). Missing
    files, including explicit paths, are created only after confirmation;
    untouched defaults are omitted.

    Only changed settings are patched. Unrelated definitions, comments, and line
    endings survive. Edited arrays are reformatted and their comments may move
    above elements, as disclosed in review. Native tables, inline tables,
    dotted/quoted keys, and multiline strings are supported.

    Saves are atomic, preserve existing permissions, and use `0600` for new files.
    Symlinks are followed without replacing the link; changed files or targets
    cause a conflict. The per-target `.settings.lock` remains after exit and
    coordinates settings writers, but cannot exclude arbitrary external editors.
    When a state directory is available, saving invalidates the session-list
    cache; cleanup failures are reported separately from a successful save.

### Legacy conversion

Opening a legacy file offers a separate migration review. Confirming preserves
the source and creates a native file, then switches the editor and returning
picker to it. Existing destinations are never overwritten.

??? info "Conversion details"

    Conversion flattens imports and may normalize formatting/defaults; see
    [Legacy migration](#legacy-migration). Source/import changes invalidate the
    prepared conversion. Invalid configs are reported without automatic repair.
    Update any explicit argument or `HERDR_SESH_CONFIG` that still selects the
    old file for future launches; the editor does not change your environment.
    Discarding later edits does not undo a confirmed conversion or save.

## Configuration file lookup

Lookup order:

1. `--config PATH`
2. `HERDR_SESH_CONFIG` (an error if the file does not exist)
3. `${HERDR_PLUGIN_CONFIG_DIR}/config.toml`
4. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml` as a legacy fallback
5. `~/.config/herdr-sesh/config.toml`
6. `~/.config/herdr-sesh/sesh.toml` as a legacy fallback
7. `~/.config/sesh/sesh.toml` as a legacy fallback

Explicit paths may use either schema: a top-level `version` selects native
decoding; otherwise they are legacy. See [config commands](commands.md#configuration-commands)
for `path`, `init`, `validate`, and missing-file exceptions.

## Settings

Picker shortcuts, appearance, sorting, and history are documented in
[Picker](config/picker.md).

### `[list]`

| Field | Runtime effect |
| --- | --- |
| `cache` | Default `false`. Caches normal deduplicated `list` results for five seconds in `HERDR_PLUGIN_STATE_DIR`, scoped to the resolved config file. It does not cache `list --blacklisted`, `list --hide-duplicates=false`, `picker`, or `connect`. |
| `source_order` | Default order: `herdr`, `config`, `zoxide`, `dir`. Reorder these sources; unknown or duplicate names are rejected and omitted sources are appended. |
| `blacklist` | Default `[]`. Treats each value as a regular expression matched against workspace names. Normal listings hide matches; `list --blacklisted` shows them. Invalid regexes are rejected. |

### `[naming]`

| Field | Runtime effect |
| --- | --- |
| `path_components` | Sets the number of path components used by the directory-name fallback for a newly created direct-path workspace. Git repositories keep their repository-derived name. Must be at least `1` (the default). |

### `[history]`

| Field | Runtime effect |
| --- | --- |
| `last_workspace_label` | Default `"last"`. Text for the sidebar's `$sesh_last` marker. An empty string hides the marker; control characters are rejected. Applies on the next workspace focus or close event. See [Sidebar marker](config/picker.md#sidebar-marker). |

### `[workspace_defaults]`

| Field | Runtime effect |
| --- | --- |
| `startup` | Default: none. Fallback command run after a new Herdr workspace is created. `{}` is replaced with the workspace path. |
| `preview` | Fallback command used by `preview` and the native picker. `{}` is replaced with the workspace path. Absent or empty values use `eza --icons=always --color=always -la {}`. |

### `[[workspace]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Required workspace label and connect target. Must be non-empty and unique. |
| `path` | Required workspace path; `~/` is expanded before it is sent to Herdr. Must be non-empty. |
| `startup` | Startup override; absent or empty uses the rule/default fallback. |
| `preview` | Preview override; absent or empty uses the rule/default fallback. |
| `disable_startup` | Suppresses the workspace startup command, including rule/default fallbacks, when `true`; unset inherits the rule, while explicit `false` overrides it. Tab and pane startup commands still run. |
| `tabs` | Default `[]`. Names of `[[tab]]` entries to create as Herdr tabs. Every referenced tab must exist. |

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

For example, append this rule to a native config containing the `git` tab above:

```toml
[[rule]]
path_glob = "~/projects/**"
tabs = ["git"]
```

Discovered projects under `~/projects` then receive that tab when created.
Omitted rule commands fall back to workspace defaults; `disable_startup` defaults
to `false` and `tabs` to `[]`.

## Legacy migration

Legacy files remain supported for at least one released version. To convert,
use the [settings editor](#legacy-conversion), or follow the
[CLI setup](commands.md#run-the-binary-directly) and run
`"$sesh_bin" config migrate`. Add `--config PATH` (or positional `PATH`) to select
a particular file.

Migration flattens imports into native `config.toml`, preserves the source, and
prints the new path. Check the result before optionally deleting the legacy
file. First update `HERDR_SESH_CONFIG` or explicit arguments that select the old
path. A legacy file already named `config.toml` must be renamed before migration;
in-place conversion is not supported, even with `--force`.

??? info "Conversion and overwrite rules"

    Unspecified legacy `tui.show_icons` becomes enabled. The old colorless default
    preview becomes `eza --icons=always --color=always -la {}`. Explicit icon
    choices and custom preview commands survive; comments and key order do not.

    Invalid regexes, duplicate names, missing tab references, and other native
    validation errors stop conversion before writing. Output is installed
    atomically with `0600` permissions. CLI `--force` permits replacing an existing
    native destination, but never an unrelated or invalid `config.toml`.

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

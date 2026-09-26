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
| Picker appearance, sorting, and defaults | [Settings editor](#settings-editor) |
| A project with a named tab | [Add a workspace with a tab](#add-a-workspace-with-a-tab) |
| An editor, development server, and logs in split panes | [Pane layout walkthrough](#pane-layout-walkthrough) |
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
commands.

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

To open several panes inside a tab, follow the [pane layout walkthrough](#pane-layout-walkthrough).

## Example

This is a customization example, not a dump of the defaults. Omitted settings
use the defaults described in [Settings](#settings).

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
tab splits with native [pane layouts](#tabpane) instead.


## Settings

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

### `[keys]`

```toml
[keys]
cycle_preview_mode = "ctrl+o"
```

`cycle_preview_mode` changes the native picker's preview-mode shortcut. Omit it
for `"ctrl+o"`, choose a key such as `"alt+p"` or `"f2"`, or set it to `""` to
disable cycling and hide the shortcut hint. Key names use Bubble Tea's exact,
case-sensitive spelling: a single printable character or a named key, with
modifiers joined by `+` in `ctrl`, `alt`, `shift`, `meta`, `hyper`, `super` order.
Unsupported names, duplicate modifiers, and incorrect modifier order are rejected
as configuration errors. For shifted printable keys, configure the resulting
character (`"P"` rather than `"shift+p"`, or `"?"` rather than `"shift+/"`).
Shift-only printable spellings are rejected because key events report the text;
combinations such as `"ctrl+shift+p"` and `"shift+f2"` remain valid. When Shift is
combined with other modifiers, use the unshifted base key: `"ctrl+shift+p"`, not
`"ctrl+shift+P"`, and `"alt+shift+/"`, not `"alt+shift+?"`.
The configured shortcut
takes precedence over other native picker bindings, so choose an unused key.
Disabling cycling leaves the initial `[picker].preview_mode` in effect and
returns keys to their normal picker/input handling. This does not affect fzf or
configure shortcuts in Herdr itself.

### `[picker]`

| Field | Runtime effect |
| --- | --- |
| `show_icons` | Shows Nerd Font source icons in the native picker. The default is `false`; source names remain visible when icons are hidden. |
| `show_path` | Shows the path column in the native picker when space permits (default `true`). Set it to `false` to hide the column and give the side-by-side preview 50% of the available width initially. The divider remains draggable; narrow terminals keep stacked previews. This does not affect path matching, the last-workspace footer, or fzf. |
| `show_preview` | Shows the preview panel in the native picker. The default is `true`; set it to `false` to give the workspace list the full available width and height without running preview commands. This does not change fzf preview behavior. |
| `preview_mode` | Sets the native picker's initial preview to `command` (the configured preview command or built-in fallback, the default) or `pane` (the active pane of the selected Herdr workspace, refreshed once per second). Press ++ctrl+o++ (or `keys.cycle_preview_mode`) to switch modes while the picker is open. `show_preview = false` still disables previews. This does not affect fzf or `herdr-sesh preview`. |
| `prioritize_home` | Controls exact case-insensitive `home` searches in the native picker. The default is `true`, which promotes the actual home-directory session ahead of real-name and ordinary path matches. Set it to `false` to keep real-name matches first, then path matches in their existing order; the actual home-directory session remains searchable through the exact `home` alias. |
| `herdr_theme_inherit` | Inherits colors from Herdr's active theme. The default is `true`; set it to `false` to keep the native picker's built-in colors. |
| `replace_worktree_icon` | Replaces the Herdr sheep icon with `↳` for linked worktree rows. The default is `true`. Set it to `false` to keep the sheep icon (or plain `[herdr]` when icons are hidden); the purple type color and tree branches remain. |
| `prompt` | Replaces the picker prompt. An empty value uses `Sesh> `. |
| `placeholder` | Replaces the picker placeholder. An empty value uses `Filter workspaces`. |
| `separator_aware` | Makes native and fzf picker searches treat `-`, `_`, `/`, and `.` as spaces. |
| `workspace_sort` | Sets the native picker's initial Herdr workspace order to `workspace` (Herdr's order, the default), `recent` (most recently visited first), or `agent` (agent-status priority). Press ++ctrl+r++ to cycle `workspace` → `recent` → `agent` while the picker is open. This setting does not affect fzf or JSON output. |
| `show_last_workspace` | Shows the workspace targeted by `herdr-sesh last` in the picker footer. The default is `true`; set it to `false` to hide the footer without disabling history tracking or the `last` command. |
| `show_last_workspace_path` | Shows the Herdr workspace working directory beside the last workspace name. The default is `true`; set it to `false` to show only the workspace name. |

#### Search ranking

The native picker matches names and paths case-insensitively and places name
matches before path-only matches, retaining the existing order within each
group. For example, searching `api` puts a workspace named `api` ahead of one
named `web` whose path contains `/api/`. `workspace_sort` determines the Herdr
workspace order within these match groups.

An exact `home` query also matches the actual home-directory session, even if
its name does not contain `home`. With `prioritize_home = true` (the default),
that session comes first. With `false`, it stays in the path-match group.
These ranking rules apply only to the native picker; fzf uses its own ranking.

#### Preview controls

When the native picker shows the preview beside the workspace list, click and
drag the vertical divider with the left mouse button to resize it. Both panels
keep a minimum width. The chosen width lasts until the picker closes; narrow
terminals continue to show the preview below the list.

See [Keybindings](keybindings.md) for switching between command and active-pane
previews. Pane mode reads visible terminal contents without focusing the selected
workspace or running its configured preview command.

#### Workspace history

`herdr-sesh last`, the previous-workspace footer, and recent sorting use history
that also tracks workspace switches made through Herdr's own controls or CLI.
The installed plugin starts tracking automatically through startup, focus, and
close hooks; no additional keybinding or configuration is required. Closed
workspaces are removed from history.

!!! warning "Herdr 0.9.0 sidebar navigation"

    Herdr 0.9.0 suppresses lifecycle focus events for client navigation, so
    sidebar switches can leave history stale and make `last` select the wrong
    workspace. Use a running Herdr server on 0.9.1 or newer for the upstream
    fix; rebuilding herdr-sesh alone does not fix the missing events.
    See [issue #125](https://github.com/fullerzz/herdr-plugin-sesh/issues/125).

History is separate for each Herdr session, using `HERDR_SOCKET_PATH` to select
`${HERDR_PLUGIN_STATE_DIR}/history/<socket-hash>/history.json`. Existing unscoped
history is copied on first use for the default session only; named sessions
start with their own history. Hiding the footer with `show_last_workspace = false`
does not disable history tracking or the `last` command.

See [Workspace history tracking](development/workspace-history.md) for lifecycle,
persistence, and reconnect limitations.

#### Cursor and status indicators

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
Herdr calls an actively running agent `working`.

The native picker's `agent` sort mode orders recognized states as blocked →
done → working → idle, followed by workspaces with no agent or an unknown
state. Ties retain Herdr's original workspace order, including unrecognized
future states. Live status refreshes update this order without changing the
selected workspace. Sorting only rearranges Herdr rows within their configured
source-order slots; it does not move them ahead of `config`, `zoxide`, or `dir`
rows when `list.source_order` puts those sources first.

### Picker colors

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
immediately beneath its open parent workspace in workspace, recent, and agent
sort modes, matching Herdr's sidebar. In agent mode, the highest-priority status
on any family member ranks the whole family; the parent remains first and its
children follow in agent-priority order. With icons disabled, the label is `[↳ herdr]`.
When the parent is visible, `├─` and `└─` branches reinforce the family in the
workspace-name column. Wide layouts show the worktree path in the secondary
column when space permits; narrow layouts retain the purple type label. This is
automatic and does not depend on `show_icons`. Set
`picker.replace_worktree_icon = false` to retain the normal sheep icon or plain
`[herdr]` label while keeping the other child-worktree cues. If Herdr reports a
linked worktree but no single open parent can be resolved, the row remains
ungrouped rather than inventing a parent.

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

Workspace startup runs in Herdr's initial workspace pane before configured tabs
are created. Tab and pane startup commands run in their own terminals, so an
interactive workspace command such as `lazygit` does not receive a tab's `nvim`
command as input. Workspace exports and directory changes do not carry into
configured tabs; use their `path` and pane `env` settings instead.

### `[[tab]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Name referenced by a workspace or rule `tabs` list and used as the Herdr tab label. Must be non-empty and unique. |
| `path` | Optional tab working directory. Without it, the workspace path is used; relative paths resolve against the workspace path and `~/` is expanded. |
| `startup` | Command run in the new tab. `{}` is replaced with that tab's working directory. Cannot be combined with `[[tab.pane]]` entries; set `startup` on each pane instead. |
| `pane` | Optional `[[tab.pane]]` layout. See below. |

### `[[tab.pane]]`

Pane entries split a new tab into a layout. They apply only when herdr-sesh
creates the workspace; selecting an existing workspace never changes its panes
or reruns commands. Pane layouts use `herdr pane split` and `herdr tab create
--env`, available since Herdr 0.8.2.

The plugin manifest requires Herdr 0.8.2 or newer. Direct CLI invocation does
not perform a version preflight; check `herdr --version` before using layouts.
An older binary may fail after creating the workspace. Upgrade Herdr, then
save any work before closing and recreating that partial workspace; reconnecting
does not retry its layout.

#### Pane layout walkthrough

The following complete native configuration creates an editor on the left,
with a server above logs on the right. Replace the workspace path with your
project directory. This example assumes `nvim` is installed, `web/` contains
an npm project with a `dev` script, and `development.log` exists in the project
root; replace the commands to suit your project.

Edit the file printed by `config path`; pane definitions are file-only and
cannot be edited in the settings TUI. When adding this example to an existing
native config, keep its single top-level `version = 1` line and use unique tab
and workspace names. [Migrate legacy configs](#legacy-migration) first.

```toml
version = 1

[[tab]]
name = "development"

[[tab.pane]]
name = "editor"
startup = "nvim"

[[tab.pane]]
name = "server"
split_from = "editor"
split = "right"
ratio = 0.35
path = "./web"
env = { NODE_ENV = "development" }
startup = "npm run dev"

[[tab.pane]]
name = "logs"
split_from = "server"
split = "down"
ratio = 0.3
startup = "tail -f development.log"

[[workspace]]
name = "my-project"
path = "~/projects/my-project"
tabs = ["development"]
```

The workspace's `tabs` list activates the layout; defining a tab alone does
not create it. Validate and connect from a shell in the intended Herdr session:

=== "Installed plugin"

    ```bash
    sesh_root="$(herdr plugin list --plugin fullerzz.sesh --json | jq -r '.result.plugins[0].plugin_root')"
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    "$sesh_root/bin/herdr-sesh" config validate && "$sesh_root/bin/herdr-sesh" connect my-project
    ```

=== "Local checkout"

    ```bash
    just build
    export HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir fullerzz.sesh)"
    ./bin/herdr-sesh config validate && ./bin/herdr-sesh connect my-project
    ```

Alternatively, open the picker and select `my-project` after validation. Use a
workspace that is not already open: reconnecting never reapplies a layout.
The new workspace opens on the `development` tab with the editor focused.
The server initially gets 35% of the tab's width; splitting it downward gives
logs 30% of that right-hand column's height.

```text
my-project workspace
Tabs: [Herdr initial tab] [development (selected)]

development tab — approximate proportions
┌──────────────────────────────────────┬────────────────────┐
│ editor (focused)                     │ server             │
│ nvim                                 │ npm run dev        │
│                                      │                    │
│                                      │                    │
│                                      │                    │
│                                      │                    │
│                                      ├────────────────────┤
│                                      │ logs               │
│                                      │ tail -f            │
│                                      │ development.log    │
└──────────────────────────────────────┴────────────────────┘
              65% width                       35% width
```

Read the pane entries in order:

1. **`editor`** uses the new tab's root pane. It has no split settings.
2. **`server`** splits `editor` to the right, taking `0.35` (35%) of its width.
3. **`logs`** splits `server` downward, taking `0.3` (30%) of that column's
   height. The server keeps the upper 70%; the editor is unaffected.

`ratio` always describes the **new pane's share of the pane being split**,
not its share of the entire tab. Omitting it gives an even split.

| Pane | Working directory | Pane-specific environment | Startup command |
| --- | --- | --- | --- |
| `editor` | `~/projects/my-project` | None added | `nvim` |
| `server` | `~/projects/my-project/web` | `NODE_ENV=development` | `npm run dev` |
| `logs` | `~/projects/my-project` | None added | `tail -f development.log` |

The tab has no `path`, so it uses the workspace directory. A pane without
`path` uses that **tab directory**, even when it splits a pane with a different
directory. Relative pane paths resolve against the tab directory. For example,
adding `path = "frontend"` to `[[tab]]` would make the server's `./web` resolve
to `~/projects/my-project/frontend/web`. `~/` expands to the home directory;
absolute paths are used directly.

Herdr's initial tab remains alongside `development`, even when no workspace
startup command is configured. If you set workspace `startup = "lazygit"`,
it runs in that initial tab, independently of these three panes.

#### Pane fields

| Field | Runtime effect |
| --- | --- |
| `name` | Pane name referenced by later `split_from` values. Must be non-empty and unique within the tab. |
| `split_from` | Earlier pane to split. Required for every pane after the first; forward and self references are rejected. |
| `split` | `"right"` or `"down"`. Required for every pane after the first. |
| `ratio` | Optional share of the split given to the new pane, from `0.1` to `0.9`. Without it, Herdr splits evenly. |
| `path` | Optional working directory. Without it, the tab path is used; relative paths resolve against the tab path and `~/` is expanded. |
| `env` | Optional environment variables for the pane's shell. Names must match `[A-Za-z_][A-Za-z0-9_]*`. Values are passed to Herdr as arguments, not through a shell. Shell startup files run afterward and can override them. |
| `startup` | Command run in the pane. `{}` is replaced with the pane's shell-quoted working directory. |

Pane `env` values are passed as `--env KEY=VALUE` command-line arguments and may
be visible to other local users through process inspection such as `ps`, subject
to OS permissions. Error-message redaction does not hide process arguments. Do
not put secrets in these values; load them inside the pane through your shell's
credential tooling instead. `split_from` selects where to split; it does not
copy the source pane's `path` or `env`. Set those on each pane that needs them.

The first pane is the tab's root pane and cannot set `split_from`, `split`, or
`ratio`; its `path` and `env` are applied when the tab is created. Later panes
are created in declaration order, each split without taking focus, so the root
pane stays focused. Herdr also creates the workspace's own initial tab; pane
layouts apply only to configured tabs.

#### Startup commands and focus

Workspace startup runs in the initial workspace pane for both plain tabs and
pane layouts. Each command is sent separately without an `eval` wrapper or shell
composition. Layout creation does not wait for the workspace command to finish;
it is not a dependency or readiness check. Commands must use the pane shell's
syntax. The existing `{}` path substitution uses POSIX shell quoting; commands
for other shells should avoid that placeholder when its quoting is incompatible.

`disable_startup = true` on a workspace suppresses its workspace startup
command, including rule/default fallbacks. It does not suppress pane startup
commands or layout creation; remove a pane's `startup` to leave it at a shell.

With `connect --no-focus`, the workspace is created in the background and
opens on Herdr's initial tab: Herdr cannot select a tab without also focusing
its workspace.

#### Reconnecting and recovering a partial layout

Editing the configuration does not rearrange an open workspace. To try a changed
layout, save your work, close that workspace, and select it again to create a new
one. Reconnecting alone does not create missing panes or restart commands.

Layouts are validated when the configuration loads, before any workspace is
created. If a Herdr call fails partway through a layout, herdr-sesh stops and
reports the workspace, tab, pane, and failed operation. The partially created
workspace is kept so no running process is terminated. Reconnecting focuses it
without retrying the layout; close the workspace and connect again to rebuild
it.

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

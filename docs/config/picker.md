---
icon: lucide/search
---

# Picker configuration

Edit picker settings through the [Settings editor](../config.md#settings-editor)
or in the native TOML file. This page covers shortcuts, appearance, search,
previews, workspace history, and status indicators. See
[Configuration](../config.md) for file lookup and a complete starter example.

## `[keys]`

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

## `[picker]`

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

### Search ranking

The native picker matches names and paths case-insensitively and places name
matches before path-only matches, retaining the existing order within each
group. For example, searching `api` puts a workspace named `api` ahead of one
named `web` whose path contains `/api/`. `workspace_sort` determines the Herdr
workspace order within these match groups.

An exact `home` query also matches the actual home-directory session, even if
its name does not contain `home`. With `prioritize_home = true` (the default),
that session comes first. With `false`, it stays in the path-match group.
These ranking rules apply only to the native picker; fzf uses its own ranking.

### Preview controls

When the native picker shows the preview beside the workspace list, click and
drag the vertical divider with the left mouse button to resize it. Both panels
keep a minimum width. The chosen width lasts until the picker closes; narrow
terminals continue to show the preview below the list.

See [Keybindings](../keybindings.md) for switching between command and active-pane
previews. Pane mode reads visible terminal contents without focusing the selected
workspace or running its configured preview command.

### Workspace history

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

See [Workspace history tracking](../development/workspace-history.md) for lifecycle,
persistence, and reconnect limitations.

### Cursor and status indicators

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

## Linked worktrees

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

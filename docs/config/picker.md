---
icon: lucide/search
---

# Picker configuration

Use the [Settings editor](../config.md#settings-editor) or edit `config.toml`.
For example, add these settings to a native file with `version = 1`; if `[picker]`
already exists, edit its values instead of adding another table:

```toml
[picker]
show_icons = true
workspace_sort = "agent"
preview_mode = "pane"
```

## `[picker]`

These settings affect the native picker, except `separator_aware`, which also
applies to fzf. Picker sorting does not change JSON output.

| Setting | Default | Effect |
| --- | --- | --- |
| `show_icons` | `false` | Show Nerd Font source icons; source labels remain when hidden. |
| `show_path` | `true` | Show the path column when space permits. Does not affect path matching or the history footer. |
| `show_preview` | `true` | Show previews. `false` gives the list all available space and stops preview commands. |
| `preview_mode` | `"command"` | Initial preview: `command` or `pane`. See [Preview controls](#preview-controls). |
| `prioritize_home` | `true` | Put the actual home directory first for an exact `home` search; see [Search ranking](#search-ranking). |
| `herdr_theme_inherit` | `true` | Use [Herdr colors](#picker-colors); `false` keeps built-in picker colors. |
| `replace_worktree_icon` | `true` | Use `↳` for linked worktrees; `false` keeps the sheep icon or plain `[herdr]` label. Other worktree cues remain. |
| `prompt` | `"Sesh> "` | Prompt text; empty uses the default. |
| `placeholder` | `"Filter workspaces"` | Placeholder text; empty uses the default. |
| `separator_aware` | `false` | Treat `-`, `_`, `/`, and `.` as spaces in native and fzf searches. |
| `workspace_sort` | `"workspace"` | Herdr order (`workspace`), most recently visited first (`recent`), or status priority (`agent`). Press ++ctrl+r++ to cycle `workspace` → `recent` → `agent`. |
| `show_last_workspace` | `true` | Show the previous-workspace footer. Hiding it does not disable history or `last`. |
| `show_last_workspace_path` | `true` | Include the previous workspace's working directory in the footer. |

### Search ranking

Names and paths match case-insensitively; name matches precede path-only matches.
Each group retains its existing order, including `workspace_sort` for Herdr rows.
An exact `home` query also finds the actual home directory regardless of its
name: `prioritize_home = true` puts it first; `false` keeps it among path matches.
The fzf picker uses its own ranking.

Agent sorting uses blocked → done → working → idle → unknown/no agent, retaining
Herdr order for ties and unrecognized states. Live updates preserve the selected
workspace. Sorting rearranges only Herdr rows within their `list.source_order`
slots; it cannot move them ahead of sources ordered first. Linked worktrees
[sort as families](#linked-worktrees).

### Preview controls

`command` uses the configured preview command or built-in fallback. `pane` shows
the selected running workspace's active pane, refreshing once per second without
changing focus or running preview commands. Configured sessions and directories
without running panes show an unavailable message.

`show_preview = false` disables both modes and cycling. `preview_mode` does not
change the `herdr-sesh preview` command. With `show_path = false`, side-by-side
previews initially take 50% of the width; narrow terminals keep stacked previews.
See [Keybindings](../keybindings.md#native-picker-previews) for toggling modes and
resizing the divider; these adjustments last only until the picker closes.

## `[keys]`

```toml
[keys]
cycle_preview_mode = "alt+p" # default: "ctrl+o"; "" disables cycling
```

Add this table to your native config, or edit the existing `[keys]` table.
The shortcut overrides other native picker bindings, so choose an unused key.
An empty value hides the shortcut hint, leaves `preview_mode` in effect, and
returns keys to normal handling. This does not configure fzf or Herdr shortcuts.

??? info "Key syntax"

    Names use Bubble Tea's exact, case-sensitive spelling: a printable character
    or named key such as `f2`. Join modifiers in `ctrl`, `alt`, `shift`, `meta`,
    `hyper`, `super` order. Unsupported names, repeated modifiers, and incorrect
    order are rejected.

    For shifted printable keys alone, use `P` or `?`, not `shift+p` or `shift+/`.
    With other modifiers, use the unshifted base: `ctrl+shift+p` and `alt+shift+/`,
    not `ctrl+shift+P` or `alt+shift+?`. Named keys such as `shift+f2` are valid.

## Picker colors

With `herdr_theme_inherit = true`, the picker reads Herdr's config from
`HERDR_CONFIG_PATH`, then `$XDG_CONFIG_HOME/herdr/config.toml`, then
`~/.config/herdr/config.toml`. Set `[theme] name` and optional `[theme.custom]`
colors in **Herdr's config**, not the plugin config.

??? info "Supported themes and custom colors"

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

## Cursor and status indicators

Set these environment variables in the environment inherited by Herdr.
`HERDR_SESH_SMEAR_PRESET` selects the cursor animation:

| Preset | Effect |
| --- | --- |
| `crisp` (default) | Fast cyan rail with a short violet line trail. |
| `gooey` | Slower block cursor with a long shaded trail and eased movement. |
| `ghost` | Soft diamond cursor with a dotted, low-contrast trail. |

Set `HERDR_SESH_REDUCE_MOTION=1` (or `true`) for instantaneous movement without trails.

Herdr agent indicators are amber animated spinner (working), red `◉` (blocked),
green `✓` (idle), and teal `●` (done). Unknown states have no indicator.

## Workspace history

The installed plugin automatically tracks switches through the picker, Herdr
controls, and CLI for `last`, recent sorting, and the footer. History is separate
per Herdr session; closed workspaces are removed. No extra setup is required.
See [Workspace history tracking](../development/workspace-history.md) for storage,
migration, lifecycle hooks, and reconnect limitations.

!!! warning "Herdr 0.9.0 sidebar navigation"

    Sidebar switches can leave history stale. Upgrade the running Herdr server
    to 0.9.1 or newer; see [Previous workspace is unexpected](../troubleshooting.md#previous-workspace-is-unexpected).

### Sidebar marker

The plugin reports a `sesh_last` workspace metadata token with the default value `last`
on the workspace that `last` would switch to. Add `$sesh_last` to Herdr's
[Space rows](https://herdr.dev/docs/configuration/#sidebar-row-layouts) in
`~/.config/herdr/config.toml` to show it:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace", { token = "$sesh_last", fg = "#f9e2af", bold = true }],
  ["branch", "git_status"],
]
```

The marker moves with workspace history, including switches made outside the
picker.

To change its text, add this to the **herdr-sesh** `config.toml`:

```toml
[history]
last_workspace_label = "previous"
```

The default is `"last"`. An empty string hides the marker. Text may include
Unicode but must not contain control characters. Changes apply on the next
workspace focus or close event without restarting the plugin.

## Linked worktrees

Linked worktrees appear beneath their open parent in every sort mode, with a
purple type label and `├─`/`└─` branches. The default label is `↳ herdr`
(`[↳ herdr]` without icons). Paths appear when space permits; narrow layouts keep
the type label. Grouping is automatic, independent of `show_icons` and
`replace_worktree_icon`. Without a single resolvable open parent, a row stays ungrouped.

In agent mode, the family's highest-priority status ranks the whole family.
The parent remains first; children follow in agent-priority order.

---
icon: lucide/panels-top-left
---

# Layouts

Named tabs and split panes are created when a workspace is new. Reconnecting to
an open workspace keeps its running terminals. Select tab names through a
`[[workspace]].tabs` list, or through a matching `[[rule]].tabs` list for a
discovered project. See [Configuration](../config.md) for file setup and lookup.
For one named tab without pane splits, start with the
[small workspace example](../config.md#add-a-workspace-with-a-tab).

## Pane layout walkthrough

The following complete native configuration creates an editor on the left,
with a server above logs on the right. Replace the workspace path with your
project directory. This example assumes `nvim` is installed, `web/` contains
an npm project with a `dev` script, and `development.log` exists in the project
root; replace the commands to suit your project.

Edit the file printed by `config path`; pane definitions are file-only and
cannot be edited in the settings TUI. When adding this example to an existing
native config, keep its single top-level `version = 1` line and use unique tab
and workspace names. [Migrate legacy configs](../config.md#legacy-migration) first.

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
The new workspace opens on a separate initial tab because the editor root has
a startup command. Select `development` to see the layout with the editor focused.
The server initially gets 35% of the tab's width; splitting it downward gives
logs 30% of that right-hand column's height.

```text
my-project workspace
Tabs: [Herdr initial tab (selected)] [development]

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

`development` has its own tab because its root starts `nvim`. Remove the
editor's `startup` to allow reuse, provided no workspace startup applies.
If you set workspace `startup = "lazygit"` (or a rule or default startup applies),
it runs in Herdr's initial tab independently of the layout commands.

## Initial tab behavior

Herdr creates each workspace with an initial tab. For a workspace with a
configured `tabs` list, these conditions determine whether the first tab
reuses it and which tab is active after creation:

| Workspace startup | First tab's root pane | Reuse initial tab? | Active tab: normal / `--no-focus` |
| --- | --- | --- | --- |
| None | Workspace directory, no startup | Yes | First configured / first configured |
| None | Workspace directory, with startup | No | Initial / initial |
| Runs | Workspace directory, with or without startup | No | First configured / initial |
| Either | Another directory | No | Initial / initial |

“Workspace startup” includes an explicit command or a rule/default fallback,
unless `disable_startup = true` suppresses it. The root pane uses the tab's
`path`, then the first pane's `path` when set. Startup commands on later panes
or tabs do not affect initial-tab reuse. Workspaces without configured tabs
simply keep Herdr's initial tab.
Selecting a workspace in the picker follows the normal `connect` column.

Herdr reports the active pane's directory as the workspace path. A root in
another directory leaves the initial tab at the workspace path so
`connect <path>` can find it unless a command moves that pane. A root startup
can change directories,
so even `git status` or `nvim` keeps a separate initial tab. To reuse the
initial tab, leave the first root at a shell and put commands in other panes.
Changing directories or selecting another pane later can still change the
workspace path reported by Herdr.

When reused, Herdr's initial tab receives the configured tab's label and its
root pane's `env`. A workspace startup command runs in Herdr's initial pane; tab
and pane commands run in their own terminals, so an interactive command such
as `lazygit` does not receive a tab's `nvim` command as input. Workspace
exports and directory changes do not carry into configured tabs; use their
`path` and pane `env` settings instead. Startup commands can change either
pane's directory; reconnect by name or ID if the active process moves away
from the workspace path.

## `[[tab]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Name referenced by a workspace or rule `tabs` list and used as the Herdr tab label. Must be non-empty and unique. |
| `path` | Optional tab working directory. Without it, the workspace path is used; relative paths resolve against the workspace path and `~/` is expanded. |
| `startup` | Command run in the new tab. `{}` is replaced with that tab's working directory. Cannot be combined with `[[tab.pane]]` entries; set `startup` on each pane instead. |
| `pane` | Optional `[[tab.pane]]` layout. See below. |

## `[[tab.pane]]`

Pane entries split a new tab into a layout. They apply only when herdr-sesh
creates the workspace; selecting an existing workspace never changes its panes
or reruns commands. Pane layouts use `herdr pane split` and `herdr tab create
--env`, available since Herdr 0.8.2.

The plugin manifest requires Herdr 0.8.2 or newer. Direct CLI invocation does
not perform a version preflight; check `herdr --version` before using layouts.
An older binary may fail after creating the workspace. Upgrade Herdr, then
save any work before closing and recreating that partial workspace; reconnecting
does not retry its layout.

### Pane fields

| Field | Runtime effect |
| --- | --- |
| `name` | Pane name referenced by later `split_from` values. Must be non-empty and unique within the tab. |
| `split_from` | Earlier pane to split. Required for every pane after the first; forward and self references are rejected. |
| `split` | `"right"` or `"down"`. Required for every pane after the first. |
| `ratio` | Optional share of the split given to the new pane, from `0.1` to `0.9`. Without it, Herdr splits evenly. |
| `path` | Optional working directory. Without it, the tab path is used; relative paths resolve against the tab path and `~/` is expanded. |
| `env` | Optional environment variables for the pane's shell. Names must match `[A-Za-z_][A-Za-z0-9_]*`. Values are passed to Herdr as arguments, not through a shell. Shell startup files run afterward and can override them. |
| `startup` | Command run in the pane. `{}` is replaced with the pane's shell-quoted working directory. |
| `wait_for` | Optional readiness check: `{ match = "...", timeout_ms = 15000 }`. Requires `startup`. See [Waiting for a pane to become ready](#waiting-for-a-pane-to-become-ready). |

Pane `env` values are passed as `--env KEY=VALUE` command-line arguments and may
be visible to other local users through process inspection such as `ps`, subject
to OS permissions. Error-message redaction does not hide process arguments. Do
not put secrets in these values; load them inside the pane through your shell's
credential tooling instead. `split_from` selects where to split; it does not
copy the source pane's `path` or `env`. Set those on each pane that needs them.

The first pane is the tab's root pane and cannot set `split_from`, `split`, or
`ratio`; its `path` and `env` are applied when the tab is created. Later panes
are created in declaration order, each split without taking focus, so the root
pane stays focused. When the first configured tab reuses Herdr's initial tab,
its root pane's `env` is applied when the workspace is created.

### Startup commands and focus

Workspace startup runs in the initial workspace pane, which then stays a
separate tab, for both plain tabs and pane layouts. Each command is sent separately without an `eval` wrapper or shell
composition. Layout creation does not wait for the workspace command to finish;
it is not a dependency or readiness check. To hold later panes until a pane is
ready, use [`wait_for`](#waiting-for-a-pane-to-become-ready). Commands must use the pane shell's
syntax. The existing `{}` path substitution uses POSIX shell quoting; commands
for other shells should avoid that placeholder when its quoting is incompatible.

`disable_startup = true` on a workspace suppresses its workspace startup
command, including rule/default fallbacks. It does not suppress pane startup
commands or layout creation; remove a pane's `startup` to leave it at a shell.

`connect --no-focus` creates the workspace in the background. Herdr cannot
select a different tab without focusing its workspace, so it keeps the active
tab shown in the [table above](#initial-tab-behavior).

### Waiting for a pane to become ready

Pane startup commands are sent in declaration order without waiting for them to
finish. When a later pane depends on an earlier one, such as integration tests
that need a development server, add `wait_for` to the earlier pane:

```toml
[[tab]]
name = "development"

[[tab.pane]]
name = "server"
startup = "npm run dev"
wait_for = { match = "Local: http://localhost:3000", timeout_ms = 15000 }

[[tab.pane]]
name = "tests"
split_from = "server"
split = "right"
startup = "npm run test:integration"
```

After sending the server's startup command, herdr-sesh waits until a line of
that pane's recent output contains the literal `match` text. Only then does it
create the `tests` pane, run its command, and continue with later panes and
tabs. Herdr also searches output printed before the wait starts, so a fast
command cannot slip past it.

| Field | Meaning |
| --- | --- |
| `match` | Literal text to find on a single output line. A line that wraps at the pane's edge still counts as one line. Required and non-empty. It must not appear in `startup`, because the pane echoes the typed command. ANSI styling is ignored. |
| `timeout_ms` | Optional wait limit in milliseconds, from `1` to `600000` (10 minutes). Defaults to `30000`. |

The `startup` check runs when the configuration loads. If `startup` uses `{}`,
herdr-sesh checks again after replacing it with the pane's final path, just
before sending the command. If the path contains `match`, the layout stops at
that pane, like a failed wait.

Readiness checks use `herdr pane wait-output`, available in every Herdr version
the plugin supports (0.8.2 or newer).

If the text does not appear within the timeout or the wait fails, herdr-sesh
stops the layout and reports the workspace, tab, pane, and `wait_for` check
that failed. Interrupting herdr-sesh during the wait also stops the layout. In
either case, no later panes, commands, or tabs are created. The workspace and its running processes are kept, as with other
[partial layouts](#reconnecting-and-recovering-a-partial-layout); reconnecting
neither reruns commands nor retries the check.

`wait_for` is a one-time startup barrier, not a health check. A match only means
the text was printed once; the service can still fail afterward, and nothing is
restarted or monitored. Panes without `wait_for` keep the default behavior.

### Reconnecting and recovering a partial layout

Editing the configuration does not rearrange an open workspace. To try a changed
layout, save your work, close that workspace, and select it again to create a new
one. Reconnecting alone does not create missing panes or restart commands.

Layouts are validated when the configuration loads, before any workspace is
created. If a Herdr call fails partway through a layout, herdr-sesh stops and
reports the workspace, tab, pane, and failed operation. The partially created
workspace is kept so no running process is terminated. Reconnecting focuses it
without retrying the layout; close the workspace and connect again to rebuild
it.

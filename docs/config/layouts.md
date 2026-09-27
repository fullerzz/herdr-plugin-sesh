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

Edit your [plugin config](../config.md#create-your-configuration); pane definitions
are file-only. When adding this example to an existing
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

The `tabs` list activates the layout; defining a tab alone does not create it.
Open the picker from the intended Herdr session:

```bash
herdr plugin action invoke fullerzz.sesh.open-picker
```

Select `my-project`, which must not already be open. The new workspace opens on
Herdr's initial tab; select `development` to see the layout with the editor
focused. See [Initial tab behavior](#initial-tab-behavior) for why it stays separate.
For optional validation before opening, follow the [CLI setup](../commands.md#run-the-binary-directly)
and run `"$sesh_bin" config validate`.

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

`ratio` is the **new pane's share of the pane being split**: the right column
gets 35% of the tab width, then logs take 30% of that column's height.

| Pane | Working directory | Pane-specific environment | Startup command |
| --- | --- | --- | --- |
| `editor` | `~/projects/my-project` | None added | `nvim` |
| `server` | `~/projects/my-project/web` | `NODE_ENV=development` | `npm run dev` |
| `logs` | `~/projects/my-project` | None added | `tail -f development.log` |

A pane inherits the **tab directory**, not the pane it splits. Here the tab
uses the workspace directory, so logs stay at the project root. Relative pane
paths resolve against the tab directory: a tab `path = "frontend"` would make
`./web` resolve to `~/projects/my-project/frontend/web`.

## `[[tab]]`

| Field | Runtime effect |
| --- | --- |
| `name` | Name referenced by a workspace or rule `tabs` list and used as the Herdr tab label. Must be non-empty and unique. |
| `path` | Optional tab working directory. Without it, the workspace path is used; relative paths resolve against it, `~/` expands, and absolute paths are used directly. |
| `startup` | Command run in the new tab. `{}` is replaced with that tab's working directory. Cannot be combined with `[[tab.pane]]` entries; set `startup` on each pane instead. |
| `pane` | Optional `[[tab.pane]]` layout. See below. |

## `[[tab.pane]]`

Layouts require Herdr 0.8.2 or newer, as enforced by the plugin manifest.
Direct CLI use has no version preflight: check `herdr --version`. An older binary
may leave a partial workspace; upgrade, then follow [recovery](#reconnecting-and-recovering-a-partial-layout).

### Pane fields

| Field | Runtime effect |
| --- | --- |
| `name` | Pane name referenced by later `split_from` values. Must be non-empty and unique within the tab. |
| `split_from` | Earlier pane to split. Required for every pane after the first; forward and self references are rejected. |
| `split` | `"right"` or `"down"`. Required for every pane after the first. |
| `ratio` | Optional share of the split given to the new pane, from `0.1` to `0.9`. Without it, Herdr splits evenly. |
| `path` | Optional working directory. Without it, the tab path is used; relative paths resolve against the tab path and `~/` is expanded. |
| `env` | Optional environment variables for the pane's shell. Names must match `[A-Za-z_][A-Za-z0-9_]*`; values cannot contain NUL bytes. Values pass as arguments, not through a shell. Shell startup files can override them. |
| `startup` | Command run in the pane. `{}` is replaced with the pane's shell-quoted working directory. |
| `wait_for` | Optional readiness check: `{ match = "...", timeout_ms = 15000 }`. Requires `startup`. See [Waiting for a pane to become ready](#waiting-for-a-pane-to-become-ready). |

!!! warning "Do not put secrets in pane environment values"

    Values are passed as `--env KEY=VALUE` arguments and may be visible to local
    users through process inspection, subject to OS permissions. Error redaction
    does not hide them. Load secrets through shell credential tooling instead.

`split_from` copies neither `path` nor `env`; configure each pane separately.
The first pane is the root and cannot set `split_from`, `split`, or `ratio`.
Its `path` and `env` apply at tab creation (or workspace creation when reusing
the initial tab). Later panes split in declaration order without taking focus.

## Initial tab behavior

Herdr creates an initial tab. With configured tabs, reuse and focus follow this
table; picker selection uses the normal column, while `connect --no-focus`
creates the workspace in the background:

| Workspace startup | First tab's root pane | Reuse initial tab? | Active tab: normal / `--no-focus` |
| --- | --- | --- | --- |
| None | Workspace directory, no startup | Yes | First configured / first configured |
| None | Workspace directory, with startup | No | Initial / initial |
| Runs | Workspace directory, with or without startup | No | First configured / initial |
| Either | Another directory | No | Initial / initial |

Workspace startup includes rule/default fallbacks unless `disable_startup = true`.
The root uses the tab's `path`, overridden by the first pane's `path`. Later
panes/tabs do not affect reuse. Without configured tabs, the initial tab remains.
A reused tab receives the configured label and root `env`.

To reuse the initial tab, leave the first root at a shell in the workspace
directory, with no workspace startup; put commands in other panes. Any root
startup, even `git status`, keeps a separate tab because commands can change
directories. Herdr reports the active pane's directory as the workspace path;
reconnect by name or ID if a command or pane selection changes it.

Workspace startup runs in the initial pane, separately from tab/pane commands.
Its exports and directory changes do not carry into configured tabs; use their
`path` and pane `env` settings.

### Startup commands and focus

Commands are sent separately, without `eval` or shell composition. Layouts do
not wait for workspace startup to finish. Use pane [`wait_for`](#waiting-for-a-pane-to-become-ready)
for dependencies. Commands use the pane shell's syntax; `{}` uses POSIX shell
quoting, so avoid it with incompatible shells.

`disable_startup = true` suppresses only workspace startup, including rule/default
fallbacks. Tabs and panes still run their commands; omit a pane's `startup` to
leave it at a shell.

## Waiting for a pane to become ready

To wait for a server before starting tests, add `wait_for` to the server pane.
Without it, commands run in declaration order without waiting for completion.
Add this tab to a native config and reference `"development"` in a workspace's
`tabs` list; replace any existing tab with that name:

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

After sending startup, herdr-sesh waits for matching output before creating
later panes or tabs. Output printed before the wait starts is also searched.

| Field | Meaning |
| --- | --- |
| `match` | Literal text to find on a single output line. A line that wraps at the pane's edge still counts as one line. Required and non-empty. It must not appear in `startup`, because the pane echoes the typed command. ANSI styling is ignored. |
| `timeout_ms` | Optional wait limit in milliseconds, from `1` to `600000` (10 minutes). Defaults to `30000`. |

Matches are checked against `startup` when loading and again after `{}` expands
to the final path. A match in the expanded command stops the layout before that
command runs, to avoid accepting echoed input as readiness.

Timeout, wait failure, or interruption stops later panes, commands, and tabs;
errors identify the workspace, tab, pane, and failed check. Running processes
are kept; use [recovery](#reconnecting-and-recovering-a-partial-layout).
`wait_for` is a one-time startup barrier, not health monitoring or automatic
restart. It uses `herdr pane wait-output`, supported since Herdr 0.8.2.

## Reconnecting and recovering a partial layout

Configuration is validated before workspace creation. If a Herdr operation
fails during layout creation, the error identifies the workspace, tab, pane,
and operation; the workspace and running processes are kept.

To apply edits or rebuild a partial layout, save your work, stop processes as
needed, close the workspace, and select it again. Reconnecting to an open
workspace never rearranges panes, reruns commands, or retries readiness checks.

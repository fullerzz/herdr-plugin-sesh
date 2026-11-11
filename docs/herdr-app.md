# Herdr app

This page covers the **herdr-app mods** for the desktop app. The
[Herdr TUI plugin](index.md#choose-your-herdr-integration) has separate
installation, keybindings, and history tracking.

The [Herdr app](https://herdr.dev) runs mods, not plugins. The `sesh` mod in
this repository opens the herdr-sesh picker over the app window, and the
`sesh-last` mod switches to the space you were in before. The picker
lists the app's local and remote spaces alongside configured sessions and
zoxide directories. Selecting an existing space switches to it. Configured
sessions and directories create new local spaces, and a configured workspace
that names a `machine` creates its space on that machine.

Remote workspace support is available only with the app integration, including
direct CLI commands targeting the app backend. The Herdr TUI plugin cannot
list, select, or create remote spaces.

## Install

1. Put `herdr-sesh` on your `PATH`, for example by building this checkout and
   linking the binary. The mod looks it up with your login shell's `PATH`, the
   one herdr-app gives programs it runs:

    ```bash
    just build
    ln -s "$PWD/bin/herdr-sesh" ~/.local/bin/herdr-sesh
    ```

2. Link the mods into the app's home, `~/.config/herdr-app` unless
   `HERDR_APP_HOME` names another. `just install-app-mod` builds the binary and
   creates these links:

    ```bash
    mkdir -p ~/.config/herdr-app/mods
    ln -sfn "$PWD/herdr-app/mods/sesh" ~/.config/herdr-app/mods/sesh
    ln -sfn "$PWD/herdr-app/mods/sesh-last" ~/.config/herdr-app/mods/sesh-last
    herdr-app check ~/.config/herdr-app/mods/sesh
    herdr-app check ~/.config/herdr-app/mods/sesh-last
    ```

    The app writes `gpui-kit.d.ts` and `jsconfig.json` into the linked folder;
    Git ignores them.

3. Bind a key in `~/.config/herdr-app/keys.json`:

    ```json
    { "global": { "cmd-k": "mod.open:sesh" } }
    ```

    You can also open the picker from a shell with
    `herdr-app do mod.open '{"name":"sesh"}'`.

The mods use the shared [Sesh configuration](config.md#configuration-file-lookup).
Use `~/.config/herdr-sesh/config.toml` or set `HERDR_SESH_CONFIG` to an explicit
path. The app does not provide the TUI plugin's managed config directory.
Use the picker's [settings shortcut](keybindings.md#native-picker-controls)
or run `herdr-sesh config edit` to edit settings.

## Switch to the last space

Bind a key to the `sesh-last` mod in `~/.config/herdr-app/keys.json`, as the
Herdr plugin's `fullerzz.sesh.last` action is bound in Herdr:

```json
{ "global": { "cmd-;": "mod.open:sesh-last" } }
```

The mod runs `herdr-sesh last`, which shows the most recent other space in
herdr-sesh's history that is still open, and then closes. Pressing the key
again returns to where you started. If there is nowhere to go back to, the
app shows a notification.

On its own, that history holds only switches made through herdr-sesh: the
picker, `connect`, and `last`. To also record spaces you choose in the app
itself, such as from the sidebar or with `space.select`, put a `sesh-last`
block in the layout you use. The app's own layouts are read-only, so clone
one first:

```bash
herdr-app do layout.clone '{"name":"main","as":"my-layout"}'
```

Then add a zero-width block to the end of the root's `children` in
`~/.config/herdr-app/layouts/my-layout.json`:

```json
{ "id": "sesh-last", "type": "block", "size": { "px": 0 }, "content": { "mod": "sesh-last" } }
```

The block draws nothing. While the layout shows, it records each space switch
with `herdr-sesh app record-switch`, so `last` returns to the previous
space however you left it. A space shown for less than half a second is not
recorded, so passing through one, or `connect --no-focus` briefly showing the
space it creates, does not count as being there. Switches made while another
layout shows, or while the app is closed, are not recorded.

Run `just test-app-last` to exercise the installed mod in the running app. It
requires this layout block and at least two open spaces. The test navigates
between existing spaces (including remote ones), checks repeated toggles and
transient-space suppression, then restores the original space, tab, and focus.
Its navigation remains in history.

## How it works

The mod starts `herdr-sesh picker` in a herdr-core terminal and shows that
terminal in a dialog. The terminal is labelled `sesh=picker`, and the app
does not add it to any space. When you pick an entry or press ++esc++, the
picker exits and the dialog closes. If the picker fails, the dialog stays
open with its error until you press ++enter++. Opening the picker again ends
any earlier picker terminal that is still running.

herdr-sesh drives the app instead of the Herdr server when it runs in a
herdr-core terminal outside a Herdr pane. The mod forces this mode with
`HERDR_SESH_BACKEND=app`. Set `HERDR_SESH_BACKEND=herdr` to force the Herdr
server. `connect`, `last`, `list`, and `window` follow the same rule. Run
them from a terminal in the app to act on its spaces.

## Select an existing remote space

Register the remote machine with herdr-core using its SSH target:

```bash
hc machine add buntu26
```

`hc` owns machine registration and terminal processes. `herdr-app` owns the
spaces shown in its window. Sesh reads those spaces from `herdr-app ls`.
Registering a machine does not create a space; a configured workspace can
[create one](#create-a-remote-space-from-a-configured-workspace).

Open the picker and search for the machine name or space name. Remote entries
show the machine before the name, for example `[buntu26] api`. Spaces with
the same name on different machines remain separate entries. Select the
entry to switch to that existing remote space.

Remote IDs include the machine prefix, for example `buntu26/sp1`. Sesh keeps
the full ID when selecting a space and recording switch history, so `last`
can return to the remote space. For an explicit command, use the qualified ID
reported by `herdr-app ls`:

```bash
HERDR_SESH_BACKEND=app herdr-sesh connect buntu26/sp1
```

Remote files stay on the remote machine. Sesh never inspects a remote space's
folder through the local filesystem or local Git. Its directory preview runs on
that machine through `hc exec`: a portable `ls -A -p -1` listing of the space's
folder, followed by `git status --short --branch`. Configured preview commands
name local tools, so they run only for local spaces; in the native picker and
`herdr-sesh preview`, remote spaces always get this built-in preview. The
experimental fzf picker shows a remote space's name, machine, and path only.
When the folder is not a Git repository or the machine has no Git, the
preview shows only the listing. When the Git status cannot be
read for another reason, such as the machine disconnecting, the folder
becoming unreachable, or the preview deadline expiring, the listing ends with
one line saying why. The preview has 3 seconds in total. If the listing itself
fails or does not finish in time, the preview shows the space's metadata and
one line explaining why. A remote space without a recorded folder shows its
metadata only. Pane preview reads the remote terminal through `hc read` with
its qualified terminal address. Selecting an existing remote space does not
apply local session configuration or run startup commands there.

For CLI previews, select the app backend and use the qualified workspace ID:

```bash
HERDR_SESH_BACKEND=app herdr-sesh preview buntu26/sp1
```

Bare names and paths resolve locally, including the fallback that previews a
local directory; they do not select a remote namesake. The one exception is a
[configured remote workspace](#create-a-remote-space-from-a-configured-workspace),
which its name selects when no local session has that name and no local
directory is at that target. Its path never selects it.

## Create a remote space from a configured workspace

Add `machine` to a native `[[workspace]]` entry, using a name from
`hc machine ls`. Its `path` is then a path on that machine:

```toml
[[workspace]]
name = "api"
machine = "buntu26"
path = "~/src/api"
tabs = ["dev"]
```

The picker lists it as `[buntu26] api`. Selecting it, or running
`HERDR_SESH_BACKEND=app herdr-sesh connect api`, creates the space on
`buntu26` and builds its configured tabs, panes, `env` values, startup
commands, and `wait_for` checks there. Startup commands run in the remote
terminals, so they name the remote machine's tools.

- A leading `~` in the workspace, tab, or pane `path` is the remote home, and
  a relative workspace `path` starts there too. Sesh asks the machine for
  `$HOME` with one `hc exec` call before it creates the space; a workspace
  whose paths are all absolute skips the call. Relative tab and pane paths
  resolve against the workspace path as usual.
- Sesh does not check the path locally, inspect it with Git, or add it to
  zoxide. `[[rule]]` entries and `workspace_defaults.startup` match local
  paths, so they do not apply to remote workspaces.
- `connect` and `preview` take the workspace's name, not its path: a path
  target is always a local path. A target is first checked as a local
  directory, and one that exists is named with local Git, so a local
  directory called like the workspace wins.
- Once the space is open, it is listed once, as the open space. Selecting it,
  or connecting by its name, switches to that space instead of creating
  another. Sesh matches the open space by machine and name, as it matches
  local spaces by name.
- A local session with the same name wins a bare `connect api`. Use the
  picker, or the open space's qualified ID, to reach the remote one.
- An unknown or unreachable machine fails with the error from `hc` or
  herdr-app, and nothing is created.
- The Herdr TUI backend has no machines. Connecting to a configured remote
  workspace there fails with an error that names the Herdr app.

## Differences from the Herdr plugin

These differences apply when using the app mods or CLI commands with
`HERDR_SESH_BACKEND=app`. See the
[integration comparison](index.md#choose-your-herdr-integration) for an overview.

- Workspaces are spaces. Remote spaces show their machine and the folder
  they were made in; local spaces can show a terminal's live directory.
- The app runs no plugin hooks, so workspace history records only switches
  made through herdr-sesh, unless your layout has a
  [`sesh-last` block](#switch-to-the-last-space). History lives in
  `$XDG_STATE_HOME/herdr-sesh/herdr-app/` (default `~/.local/state`).
  Closing a space outside the picker leaves its ID in history.
- Layout splits ignore `ratio`; the app splits evenly.
- herdr-sesh starts a pane with `env` by passing the app a `/bin/sh` command
  that exports the values and starts your `$SHELL` as a login shell. Environment
  values are not typed into the terminal. When the root
  pane of a new space or tab sets `env`, the app runs that command inside its
  own login shell, so shell startup files run twice, and exiting the shell
  leaves you in one without the `env` values. Values are visible in the
  terminal's command line, for example through `hc get` or the process list.
  For a split with a `path`, Sesh sends a quoted `cd` command after shell
  startup and before the configured startup command, so a login profile that
  changes directory cannot override that path. The `cd` line may appear in
  terminal history; `wait_for` rejects a match contained in that line to avoid
  counting its echo as readiness. If the directory cannot be entered, the
  shell shows the `cd` error and stays in its current directory. Root panes
  of spaces and tabs still start in the app's `folder` before shell startup,
  so their startup files can change that directory. When the app refuses
  to create a pane that sets `env`, the error names the command and its exit
  status only: the app's message could repeat the values, so Sesh withholds
  it. Remove `env` to see the message.
- In the shown tab, a split gives focus back to the pane that had it. In a tab
  that isn't shown, the last split pane keeps focus.
- The app focuses everything it creates. For `connect --no-focus`,
  herdr-sesh selects the previously shown tab again afterwards. The new space
  keeps its last configured tab active, not its initial tab.

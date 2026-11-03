# Herdr app

The [Herdr app](https://herdr.dev) runs mods, not plugins. The `sesh` mod in
this repository opens the herdr-sesh picker over the app window. The picker
lists the app's local and remote spaces alongside configured sessions and
zoxide directories. Selecting an existing space switches to it. Configured
sessions and directories can create new local spaces.

## Install

1. Put `herdr-sesh` on your `PATH`, for example by building this checkout and
   linking the binary. The mod looks it up with your login shell's `PATH`, the
   one herdr-app gives programs it runs:

    ```bash
    just build
    ln -s "$PWD/bin/herdr-sesh" ~/.local/bin/herdr-sesh
    ```

2. Link the mod into the app's home, `~/.config/herdr-app` unless
   `HERDR_APP_HOME` names another. `just install-app-mod` builds the binary and
   creates this link:

    ```bash
    mkdir -p ~/.config/herdr-app/mods
    ln -sfn "$PWD/herdr-app/mods/sesh" ~/.config/herdr-app/mods/sesh
    herdr-app check ~/.config/herdr-app/mods/sesh
    ```

    The app writes `gpui-kit.d.ts` and `jsconfig.json` into the linked folder;
    Git ignores them.

3. Bind a key in `~/.config/herdr-app/keys.json`:

    ```json
    { "global": { "cmd-k": "mod.open:sesh" } }
    ```

    You can also open the picker from a shell with
    `herdr-app do mod.open '{"name":"sesh"}'`.

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
spaces shown in its window. Sesh reads those spaces from `herdr-app ls`;
registering a machine does not create a remote space through Sesh.

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

Remote files stay on the remote machine. Sesh displays the space's recorded
folder and does not inspect it through the local filesystem or Git. Remote
command and directory previews show space metadata without running local
commands or reading local files. Pane preview reads the remote terminal
through `hc read` with its qualified terminal address. Selecting a remote
space does not apply local session configuration or run startup commands there.

For CLI previews, use `herdr-sesh preview buntu26/sp1` with the qualified
workspace ID. Bare names and paths resolve locally, including the fallback
that previews a local directory; they do not select a remote namesake.

Configured sessions, zoxide directories, and directory entries still create
local spaces. Creating remote spaces from configured sessions is not supported.

## Differences from the Herdr plugin

- Workspaces are spaces. Remote spaces show their machine and the folder
  they were made in; local spaces can show a terminal's live directory.
- The app runs no plugin hooks, so workspace history records only switches
  made through herdr-sesh. History lives in
  `$XDG_STATE_HOME/herdr-sesh/herdr-app/` (default `~/.local/state`).
  Closing a space outside the picker leaves its ID in history.
- Layout splits ignore `ratio`; the app splits evenly.
- The app can't set a new terminal's directory or environment, so herdr-sesh
  types the pane's `cd` and `export` commands into it, octal-encoded behind
  `eval` so the echo shows no paths or values. This needs a shell that runs
  `eval "$(printf '\ooo')"`: sh, bash, zsh, or fish 3.4 and newer. A
  `wait_for` match that appears in that echoed line is rejected, as one in the
  startup command is.
- In the shown tab, a split gives focus back to the pane that had it. In a tab
  that isn't shown, the last split pane keeps focus.
- The app focuses everything it creates. For `connect --no-focus`,
  herdr-sesh selects the previously shown tab again afterwards. The new space
  keeps its last configured tab active, not its initial tab.

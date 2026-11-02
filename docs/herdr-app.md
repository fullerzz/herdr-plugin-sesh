# Herdr app

The [Herdr app](https://herdr.dev) runs mods, not plugins. The `sesh` mod in
this repository opens the herdr-sesh picker over the app window. The picker
lists the app's spaces alongside configured sessions and zoxide directories.
Selecting an entry switches to its space or creates a new one.

## Install

1. Put `herdr-sesh` on your `PATH`, for example by building this checkout and
   linking the binary:

    ```bash
    just build
    ln -s "$PWD/bin/herdr-sesh" ~/.local/bin/herdr-sesh
    ```

2. Link the mod into the app's home. `just install-app-mod` builds the
   binary and creates this link:

    ```bash
    ln -sfn "$PWD/herdr-app/mods/sesh" ~/.config/herdr-app/mods/sesh
    herdr-app check ~/.config/herdr-app/mods/sesh
    ```

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
open with its error until you press ++enter++.

herdr-sesh drives the app instead of the Herdr server when it runs in a
herdr-core terminal outside a Herdr pane. The mod forces this mode with
`HERDR_SESH_BACKEND=app`. Set `HERDR_SESH_BACKEND=herdr` to force the Herdr
server. `connect`, `last`, `list`, and `window` follow the same rule. Run
them from a terminal in the app to act on its spaces.

## Differences from the Herdr plugin

- Workspaces are spaces. Spaces on other machines are listed under the folder
  they were made in.
- The app runs no plugin hooks, so workspace history records only switches
  made through herdr-sesh. History lives in
  `$XDG_STATE_HOME/herdr-sesh/herdr-app/` (default `~/.local/state`).
  Closing a space outside the picker leaves its ID in history.
- Layout splits ignore `ratio`; the app splits evenly. Pane `path` and `env`
  are typed into the new terminal as `cd` and `export` before its startup
  command.
- The app focuses everything it creates. For `connect --no-focus`,
  herdr-sesh selects the previously shown tab again afterwards. The new space
  keeps its last configured tab active, not its initial tab.

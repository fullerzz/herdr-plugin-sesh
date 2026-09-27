---
icon: lucide/wrench
---

# Troubleshooting

## Check installation and logs

```bash
herdr plugin action list --plugin fullerzz.sesh
herdr plugin log list --plugin fullerzz.sesh
herdr plugin config-dir fullerzz.sesh
```

Check that the picker action is listed and inspect recent errors before changing
configuration. Use the documentation version selector for your installed
release. For direct binary commands below, first follow the
[shell setup](commands.md#run-the-binary-directly).

## A workspace is missing

- Clear the filter with ++ctrl+u++. Name and path matching can place results
  in different groups.
- Confirm you are in the intended Herdr session. Its socket determines which
  running workspaces are visible.
- Run `"$sesh_bin" list --json` and check the active config with
  `"$sesh_bin" config path`.
- Check `list.blacklist` and deduplication with `list --blacklisted` and
  `list --hide-duplicates=false`. These options bypass the normal list cache.
- Directory-history results require `zoxide` to be available and populated.
  Configured workspaces require a valid `[[workspace]]` entry.

The list cache lasts five seconds and does not cache the picker. Repeatedly
clearing state files is not necessary to refresh picker results.

## Configuration changes have no effect

Run `"$sesh_bin" config path` and `"$sesh_bin" config validate` in the same
shell context used to invoke the plugin. An explicit `HERDR_SESH_CONFIG` takes
precedence over the plugin config directory. See the full
[lookup order](config.md).

Herdr action bindings belong in Herdr's config; picker appearance and
`keys.cycle_preview_mode` belong in the plugin config. Native plugin files need
`version = 1` and reject unknown keys. Reopen the picker after changing its
configuration. Startup commands and tabs apply when creating a workspace, not
when focusing an existing one.

## Pane layout changes are not appearing

- Confirm the workspace's `tabs` list references the layout's `[[tab]]` name;
  an unreferenced tab is not created.
- Run `"$sesh_bin" config validate` after editing `[[tab.pane]]` entries.
  Pane layouts require the native config format (`version = 1`).
- Layouts apply only when creating a workspace. Reopening the picker or
  reconnecting to an existing workspace does not rebuild its panes or rerun
  commands.
- An extra initial tab or unexpected active tab depends on workspace startup
  and the first root pane's path/command. Check the
  [initial-tab decision table](config/layouts.md#initial-tab-behavior).

If creation failed, correct the reported error first. To apply changes or
rebuild the layout, save your work, stop processes
as needed, close the workspace, and select it again. Running processes are
preserved after failures; reconnecting does not retry the layout. See
[layout recovery](config/layouts.md#reconnecting-and-recovering-a-partial-layout).

## Startup command text appears before the shell banner

This is the shell-readiness limitation tracked in
[issue #137](https://github.com/fullerzz/herdr-plugin-sesh/issues/137).
Startup input can reach the terminal while the interactive shell is still
initializing. The terminal can echo that input before the banner, and the shell
can display it again at its prompt. Two appearances do not prove two executions.

An isolated zsh PTY reproduction with a one-second delay in `.zshrc` and a visible
banner confirmed this sequence: one command submission was echoed before the
banner, then executed once after initialization. A file append counted executions
independently of terminal output. This verifies the early-echo mechanism; the
full slow-shell scenario inside Herdr has not yet been verified.

### Upstream dependency

The installed Herdr 0.9.1 CLI and upstream source at
[`04682e57`](https://github.com/fullerzz/herdr/tree/04682e57e60ae4276761e95271a7ef1845127180)
provide no supported shell-readiness wait.
[`pane run`](https://github.com/fullerzz/herdr/blob/04682e57e60ae4276761e95271a7ef1845127180/src/cli/pane.rs#L929)
submits text and Enter through `pane.send_input`; its
[handler](https://github.com/fullerzz/herdr/blob/04682e57e60ae4276761e95271a7ef1845127180/src/app/api/panes.rs#L1424)
queues input without waiting for shell initialization.
[Pane and process metadata](https://github.com/fullerzz/herdr/blob/04682e57e60ae4276761e95271a7ef1845127180/src/api/schema/panes.rs#L342)
do not expose a shell-ready state. A foreground shell process alone does not
prove its startup files have finished.

The preferred fix is a Herdr-owned, bounded, cancellable startup submission that
waits for an explicit shell-integration readiness signal and submits input once.
Herdr owns the PTY and shell lifecycle, so it can synchronize these operations
for every client. This is a required upstream capability, **not an existing API**;
no minimum supporting Herdr version or required shell-integration installation
can be specified until it ships. Fixed sleeps, matching prompt text, and
application-output `wait_for` rules cannot establish shell readiness reliably.

### Current behavior and completion requirements

For now, workspace, plain-tab, and pane-layout startup commands continue to use
immediate submission, including newly created tabs/splits and any reused initial
pane. There is no readiness guarantee for any shell. If this affects your setup,
remove the affected `startup` command and run it manually after the prompt appears.
Reconnecting to an existing workspace does not rerun startup commands.

Once Herdr provides the capability, the plugin should use it for all three
startup paths. Unsupported servers or shells must produce an actionable error
before startup input is sent, rather than silently falling back to a delay or
immediate submission. Timeout or cancellation must identify the affected
workspace, tab, and pane, stop subsequent startup work, and retain the partial
workspace. Separate workspace/tab terminals, `--no-focus`, and creation-only
behavior must remain intact. Completion requires focused ordering, timeout,
cancellation, and single-submission tests, plus a live Herdr check with a slow
shell and visible banner.

## Preview is unavailable or reports an error

Pane mode requires a running Herdr workspace. A configured session or directory
without a running pane cannot provide a pane preview. Switch back to command
mode with ++ctrl+o++ or your configured shortcut.

Command previews use `eza` by default. If it is unavailable, install it or set
`workspace_defaults.preview` to an available command, for example `ls -la {}`.
Test the command with `"$sesh_bin" preview TARGET`. Commands time out after
five seconds; superseded previews are canceled when the selection changes.
The loading message appears only after a preview has been pending for 500 ms.

If no preview panel appears, check `picker.show_preview`. Narrow terminals put
the preview below the list, and hiding paths does not force a side-by-side
layout.

## Icons or cursor animation look wrong

Source icons need a Nerd Font in your terminal. Set `picker.show_icons = false`
to use source labels. The default `eza` preview has its own icon flag; customize
the preview command separately if its glyphs are missing.

Set `HERDR_SESH_REDUCE_MOTION=1` in the environment inherited by Herdr to disable
the cursor trail. See [cursor settings](config/picker.md#cursor-and-status-indicators).

## Previous workspace is unexpected

History is scoped to the Herdr session and automatically removes closed
workspaces. Use the installed `fullerzz.sesh.last` action to retain Herdr's
plugin environment. Check logs if lifecycle tracking stops; a subsequent
lifecycle hook can restart the watcher. Hiding the footer does not disable
history. If sidebar switches are missing from history on Herdr 0.9.0, upgrade
the running Herdr server to 0.9.1 or newer; that release restores the focus
events the plugin needs. Rebuilding the plugin alone cannot restore those
events. See [history behavior](config/picker.md#workspace-history) and the
[reconnect limitations](development/workspace-history.md#failure-model).

## Report a reproducible problem

Include the Herdr and plugin versions, OS, native/fzf picker choice, the exact
action or command, expected and actual behavior, and relevant log errors.
Share a minimal config that reproduces the issue, removing private paths and
commands as needed. A screenshot is useful for layout problems; include terminal
dimensions and whether a Nerd Font is configured.

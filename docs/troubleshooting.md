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
the cursor trail. See [cursor settings](config.md#cursor-and-status-indicators).

## Previous workspace is unexpected

History is scoped to the Herdr session and automatically removes closed
workspaces. Use the installed `fullerzz.sesh.last` action to retain Herdr's
plugin environment. Check logs if lifecycle tracking stops; a subsequent
lifecycle hook can restart the watcher. Hiding the footer does not disable
history. See [history behavior](config.md#workspace-history) and the
[reconnect limitations](development/workspace-history.md#failure-model).

## Report a reproducible problem

Include the Herdr and plugin versions, OS, native/fzf picker choice, the exact
action or command, expected and actual behavior, and relevant log errors.
Share a minimal config that reproduces the issue, removing private paths and
commands as needed. A screenshot is useful for layout problems; include terminal
dimensions and whether a Nerd Font is configured.

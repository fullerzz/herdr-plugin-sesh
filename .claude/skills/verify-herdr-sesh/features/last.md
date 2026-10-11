# Previous workspace

The `last` action switches to the previously focused workspace, including switches made outside the plugin, using per-Herdr-session history recorded by the plugin's background watcher.

## Sub-features

- `last-toggle` invoking twice returns focus to where it started.
- `last-external` focus changes made through Herdr itself (not the picker) are recorded.
- `last-closed` closed workspaces are pruned and skipped.
- `last-empty` with no history the action fails with `no previous workspace recorded`.

## How to get to it (user POV)

- Herdr action `fullerzz.sesh.last` (usually bound to `prefix+shift+b`).
- Herdr CLI: `"$HSV" x "$RUN" herdr plugin action invoke fullerzz.sesh.last` (users run it without the wrapper in their own Herdr).

## Driving it with hsv + herdr pane API

Preconditions:

- Fresh run at baseline from [README](./README.md): only workspace `base`, focused. `BASE=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="base").workspace_id')`.
- Doctor reports plugin state under `$RUN/state` (the watcher records there).
- "Invoke" = `"$HSV" x "$RUN" herdr plugin action invoke fullerzz.sesh.last >/dev/null`. The action returns before the switch; never read focus right after it.
- "Focus-poll L" = the README focus poll with `_ L`; exit `0` means `L` is focused within 5s.
- "Log-poll N" waits until the plugin log holds more than N *finished* `last` entries (a fresh entry is `running` with null stderr):

  ```sh
  "$HSV" x "$RUN" sh -c 'for i in $(seq 25); do herdr plugin log list --plugin fullerzz.sesh | jq -e --argjson n "$1" "[..|objects|select(.command? == [\"./bin/herdr-sesh\",\"last\"] and .status != \"running\")]|length > \$n" >/dev/null && exit 0; sleep 0.2; done; exit 1' _ 0
  ```

- "Save log LABEL" keeps the full log and prints the `last` entries in log order:

  ```sh
  "$HSV" x "$RUN" sh -c 'herdr plugin log list --plugin fullerzz.sesh | tee "$HSV_EVIDENCE/$1.json" | jq -c "[..|objects|select(.command? == [\"./bin/herdr-sesh\",\"last\"])|{status,stderr}]"' _ LABEL
  ```

- **Empty (`last-empty`).** Invoke. Log-poll 0. Save log `last-empty-log`: exactly one `last` entry; its stderr contains `no previous workspace recorded`. Focus-poll `base` succeeds (focus unchanged); `"$HSV" cli "$RUN" last-empty-ws herdr workspace list`.
- **Create history.** `"$HSV" cli "$RUN" last-connect-sesh herdr-sesh connect sesh`. Focus-poll `sesh`. `S=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="sesh").workspace_id')`. History is only `sesh` now (see Gotchas: `base` was never recorded), so refocus it: `"$HSV" x "$RUN" herdr workspace focus "$BASE"`; focus-poll `base`. History: `base`, `sesh`.
- **Toggle (`last-toggle`).** Invoke. Focus-poll `sesh`. Save `"$HSV" cli "$RUN" last-toggle-1-ws herdr workspace list`. Invoke. Focus-poll `base`. Save `last-toggle-2-ws`.
- **External switch (`last-external`).** `"$HSV" x "$RUN" herdr workspace focus "$S"` (plain Herdr, not the plugin); focus-poll `sesh`. Invoke; focus-poll `base`. Save `last-external-ws`.
- **Closed workspace (`last-closed`).** Third workspace first: `"$HSV" cli "$RUN" last-connect-zoxed herdr-sesh connect "$RUN/home/projects/zoxed"`; focus-poll `zoxed`. History is now `zoxed`, `base`, `sesh`. Close the previous one: `"$HSV" x "$RUN" herdr workspace close "$BASE"` (focus-poll `zoxed` still succeeds). Wait for the watcher to prune it:

  ```sh
  for i in $(seq 25); do jq -e --arg id "$BASE" '.workspaces|index($id)|not' "$RUN"/state/herdr/plugins/fullerzz.sesh/history/*/history.json >/dev/null && break; sleep 0.2; done
  ```

  (still listed after 5s = pruning broken; report it). Invoke. Log-poll 4. Focus-poll `sesh` succeeds: `last` skipped the closed `base` and landed on the remaining live workspace. Save `last-closed-ws` (`base` absent, `sesh` focused) and log `last-closed-log`: the fifth (newest) `last` entry has no `workspace … not found` and no `no previous workspace recorded` in stderr.
- **Proof.** History file: `$RUN/state/herdr/plugins/fullerzz.sesh/history/<sha256 of socket path>/history.json`. Copy it at the end: `"$HSV" x "$RUN" sh -c 'cp "$HSV_RUN"/state/herdr/plugins/fullerzz.sesh/history/*/history.json "$HSV_EVIDENCE/last-history.json"'`.

## Gotchas

- The action returns before the switch completes; poll `workspace list`, not the invoke output.
- `plugin log list` also contains `watch-history` event entries; pick the entry whose `command` is `["./bin/herdr-sesh","last"]`.
- The focus event that starts the history watcher is not recorded (`applyHistoryHook` ignores `workspace.focused`; the watcher only sees later events). On a fresh run `base` is missing from history until it is focused again, so `last` right after one `connect` reports `no previous workspace recorded`. Observed 2026-10-08; treat as known product behavior, not a harness fault.
- Running `herdr-sesh last` directly (`"$HSV" cli`) lacks `HERDR_PLUGIN_STATE_DIR` and reports `no previous workspace recorded`; that is documented behavior, not proof of a bug.
- Never run this against a shared `XDG_STATE_HOME`: a new session's history can be seeded from the user's legacy `history.json`, so `last` targets foreign workspace IDs (`workspace wT not found`).

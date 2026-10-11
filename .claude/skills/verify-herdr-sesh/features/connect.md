# Connect

`herdr-sesh connect TARGET` focuses an existing workspace or creates one for a configured session name, a directory path, or a workspace ID, applying configured tabs and startup commands, and prints the session name.

## Sub-features

- `connect-name` connects a configured session by name (`sesh`).
- `connect-path` connects an arbitrary directory, naming the workspace from the path.
- `connect-id` focuses a running workspace by ID (read from `workspace list`).
- `connect-existing` reconnecting focuses the existing workspace instead of creating another.
- `connect-no-focus` `--no-focus` creates or finds the workspace without moving focus.

## How to get to it (user POV)

- From a shell inside Herdr: `herdr-sesh connect [--config PATH] [--no-focus] TARGET`.
- Indirectly via `herdr-sesh clone` and `herdr-sesh root --connect` (not mapped here).

## Driving it with hsv + herdr pane API

Preconditions:

- Baseline from [README](./README.md); no workspace named `sesh` or `zoxed`.
- `BASE=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="base").workspace_id')`.

- **By name.** `"$HSV" cli "$RUN" connect-name herdr-sesh connect sesh`. Exit `0`, stdout `sesh`. `"$HSV" cli "$RUN" connect-name-ws herdr workspace list` shows `sesh` focused. `S=$("$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="sesh").workspace_id')`; `"$HSV" cli "$RUN" connect-name-tabs herdr tab list --workspace "$S"` shows `git`. Git pane: `T=$("$HSV" x "$RUN" herdr tab list --workspace "$S" | jq -r '.result.tabs[]|select(.label=="git").tab_id')`, `G=$("$HSV" x "$RUN" herdr pane list --workspace "$S" | jq -r --arg t "$T" '.result.panes[]|select(.tab_id==$t).pane_id')`, `"$HSV" x "$RUN" herdr pane wait-output "$G" --match 'No commits yet' --timeout 5000`, then `"$HSV" cli "$RUN" connect-name-git herdr pane read "$G" --source recent` shows `On branch …` and `No commits yet`.
- **Existing.** `"$HSV" cli "$RUN" connect-existing herdr-sesh connect sesh`. Exit `0`; `"$HSV" cli "$RUN" connect-existing-ws herdr workspace list` has the same workspace count and `sesh` still has ID `$S`.
- **By path, no focus.** `"$HSV" cli "$RUN" connect-path herdr-sesh connect --no-focus "$RUN/home/projects/zoxed"`. Exit `0`, stdout `zoxed`. `"$HSV" cli "$RUN" connect-path-ws herdr workspace list` has `zoxed` with `focused:false`; `sesh` is still focused.
- **By ID.** `"$HSV" cli "$RUN" connect-id herdr-sesh connect "$BASE"`. Exit `0`; `"$HSV" cli "$RUN" connect-id-ws herdr workspace list` shows `base` (`$BASE`) focused.

## Gotchas

- Without `HERDR_SOCKET_PATH` from `env.sh` this command acts on the user's real Herdr. Always go through `"$HSV" cli` or `"$HSV" x "$RUN"`.
- `HERDR_PLUGIN_CONFIG_DIR` in `env.sh` selects the fixture config; `--config` overrides it.
- A path argument is shown by Herdr under `/private/tmp/...` on macOS.
- Direct CLI runs do not set `HERDR_PLUGIN_STATE_DIR`, so they do not feed the `last` history the plugin uses; prove history through the action instead.

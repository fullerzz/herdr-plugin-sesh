# List sessions

`herdr-sesh list` prints the merged, deduplicated sessions the picker searches: running Herdr workspaces, configured sessions, and zoxide directories, in configured source order with blacklist applied.

## Sub-features

- `list-sources` includes `herdr`, `config`, and `zoxide` sources in `source_order`.
- `list-json` `--json` prints objects with `source`, `name`, `path`, and `workspace_id` for running workspaces.
- `list-dedupe` a configured session that is already running appears once (as `herdr`); `--hide-duplicates=false` shows both.
- `list-blacklist` names matching `blacklist` (`^scratch$`) are hidden; `--blacklisted` shows only those.
- `list-picker-json` `herdr-sesh picker --json` prints the picker's selectable sessions without opening a TUI.

## How to get to it (user POV)

- `herdr-sesh list [--config PATH] [--json] [--blacklisted] [--hide-duplicates=false]`.
- `herdr-sesh picker --json`.

## Driving it with hsv + herdr pane API

Preconditions:

- Baseline from [README](./README.md); workspace `base` running; `sesh` not running.

- **All sources.** `"$HSV" cli "$RUN" list-json herdr-sesh list --json`. Exit `0`; entries with `source` `herdr` (`base`), `config` (`sesh`, `startup_command: "git status"`, `window_names: ["git"]`), `zoxide` (`zoxed`).
- **Dedupe.** `"$HSV" cli "$RUN" list-connect-sesh herdr-sesh connect --no-focus sesh`, then `"$HSV" cli "$RUN" list-dedupe herdr-sesh list --json`. `sesh` appears once with `source: "herdr"` and a `workspace_id` equal to the one in `"$HSV" cli "$RUN" list-dedupe-ws herdr workspace list`. `"$HSV" cli "$RUN" list-dupes herdr-sesh list --json --hide-duplicates=false` shows it under both `herdr` and `config`.
- **Blacklist.** `mkdir "$RUN/home/scratch"` and `"$HSV" x "$RUN" zoxide add "$RUN/home/scratch"`. `"$HSV" cli "$RUN" list-blacklist herdr-sesh list --json` has no `scratch`; `"$HSV" cli "$RUN" list-blacklisted herdr-sesh list --json --blacklisted` contains it.
- **Picker JSON.** `"$HSV" cli "$RUN" list-picker-json herdr-sesh picker --json`. Exit `0`; same session names as `list --json`.

## Gotchas

- zoxide's database lives under the isolated `HOME` (`$RUN/home/Library/Application Support/zoxide` on macOS); the user's zoxide history never appears.
- `list` ignores an unavailable Herdr; if Herdr rows are missing, run `doctor` before concluding the source is broken.
- The fixture has `cache = false`. Testing the cache needs a config edit and a `HERDR_PLUGIN_STATE_DIR` (only set for plugin-launched commands).

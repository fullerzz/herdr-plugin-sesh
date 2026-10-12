---
name: verify-herdr-sesh
description: Drive the real herdr-sesh Herdr plugin (Sesh picker overlay, settings editor, previous-workspace action, and the herdr-sesh CLI) inside a throwaway, isolated Herdr server and capture proof. Use when asked to verify, demo, smoke-test, or confirm a herdr-sesh change works in the real app rather than only in Go tests.
---

# Verify herdr-sesh

herdr-sesh is a Go plugin for Herdr (a terminal multiplexer). Users reach it through Herdr plugin actions that open TUI overlay panes (`Sesh Picker`, `Sesh Settings`) or switch workspaces, plus a `herdr-sesh` CLI. Everything talks to Herdr through the `herdr` CLI and `HERDR_SOCKET_PATH`.

**Never drive the user's Herdr.** Agents often run *inside* Herdr (`HERDR_ENV=1`, socket `~/.config/herdr/herdr.sock`); a bare `herdr …` or `herdr-sesh connect …` creates, focuses, or closes the user's real workspaces and writes their plugin history. Every `herdr` / `herdr-sesh` command in a verification runs as `"$HSV" x "$RUN" …`, `"$HSV" cli`, or `"$HSV" tui`. Nothing else.

## Setup

Resolve paths once per command (shell state does not persist between tool calls; reuse the literal values):

```sh
VERIFY_DIR="$(dirname "$(realpath "<path of this SKILL.md>")")"   # e.g. <repo>/.claude/skills/verify-herdr-sesh
TARGET_REPO="$(git -C <checkout> rev-parse --show-toplevel)"
HSV="$VERIFY_DIR/scripts/hsv.sh"
```

Requires `herdr` (≥ 0.8.2), `tmux`, `jq`, Go via `mise install`. Optional: `zoxide`, `eza` (default preview).

## Launch

```sh
RUN=$("$HSV" up "$TARGET_REPO")      # stdout: only the RUN path, e.g. /tmp/hsv.gQs5Jj
```

`up` creates `RUN=/tmp/hsv.XXXXXX` (short on purpose: macOS caps unix socket paths at 104 bytes) and immediately prints `hsv: RUN=<dir>` on stderr. It then:

- builds the plugin into the run, never into the checkout: `$RUN/plugin/herdr-plugin.toml` (copy of the checkout manifest) and `$RUN/plugin/bin/herdr-sesh` (`go build` with `Version` from `git describe`). `$TARGET_REPO/bin/` is never written; the user's real Herdr may run its plugin from there.
- writes `$RUN/env.sh` (unsets every inherited `HERDR_*`, `TMUX`, `_ZO_DATA_DIR`; points `HOME`, all `XDG_*`, `HERDR_SOCKET_PATH`, `HERDR_PLUGIN_CONFIG_DIR` into `$RUN`; `PATH` starts with `$RUN/plugin/bin`),
- starts `herdr server` there, waits for the socket, links `$RUN/plugin` with `herdr plugin link`,
- seeds fixtures: plugin config = copy of `testdata/herdr-sesh.toml` (session `sesh`, tab `git`, blacklist `^scratch$`, sort `recent`); `~/projects/sesh` (git repo + README) and `~/projects/zoxed` (added to zoxide), both under `$RUN/home`.

Any failure after `RUN` exists runs `down` on it and exits non-zero (stderr says so); the `hsv: RUN=` line tells you which dir it was. Ready = `up` printed `RUN` and exited 0. The plugin's startup history watcher runs as a child of that server.

After editing Go code mid-run, `"$HSV" rebuild "$RUN"` (same build into `$RUN/plugin/bin`), then `doctor`.

Most user paths need a focused workspace first (`open-picker` fails with `no active workspace` otherwise):

```sh
"$HSV" x "$RUN" herdr workspace create --cwd "$RUN/home" --label base --focus
```

### Running commands (`x`)

```sh
"$HSV" x "$RUN" herdr workspace list                                  # one command
"$HSV" x "$RUN" sh -c 'herdr pane list | jq ... > "$HSV_EVIDENCE/x.json"'   # pipelines / several commands
"$HSV" x "$RUN" herdr workspace list | jq -r '.result.workspaces[]|select(.label=="sesh").workspace_id'   # jq outside is fine
```

`x` validates `RUN` first and refuses (dies) if it is missing, invalid, or torn down, then execs the command with the isolated env. Inside `sh -c '…'`, `$HOME`, `$HSV_EVIDENCE`, `$HERDR_PLUGIN_CONFIG_DIR`, `$XDG_STATE_HOME` are the isolated values; pass outer values as arguments (`sh -c '… "$1" …' _ "$P"`).

**Forbidden: `( . "$RUN/env.sh"; … )`.** It fails open: with an empty, mistyped, or torn-down `RUN` the source fails, the subshell continues, and the next `herdr` talks to the user's real Herdr through the inherited `HERDR_SOCKET_PATH`.

## Doctor

Run first, and whenever anything looks off. Read-only.

```sh
"$HSV" doctor "$RUN"
```

All lines must be `ok`:

- server owned: the server PID still has the start time recorded at `up`.
- socket answers: `herdr workspace list` succeeds on `$RUN/cfg/herdr/herdr.sock`.
- plugin `fullerzz.sesh` is enabled and its `plugin_root` is `$RUN/plugin`.
- binary fresh: no `*.go`, `go.mod`, or `go.sum` in `$TARGET_REPO` is newer than `$RUN/plugin/bin/herdr-sesh` (fix: `"$HSV" rebuild "$RUN"`).
- plugin state lives under `$RUN/state`.
- fixture config is present.

## Drive

Two harnesses, pick by entry point:

**Herdr panes (primary, the real user path).** Actions open overlay panes inside the isolated server; drive them with Herdr's own pane API. Keep one flow in one Bash call (or reuse the literal `$P`):

```sh
"$HSV" x "$RUN" herdr plugin action invoke fullerzz.sesh.open-picker >/dev/null
P=$("$HSV" overlay "$RUN" "Sesh Picker")      # polls until the focused pane's label matches; prints its ID; dies after 10s
"$HSV" x "$RUN" herdr pane wait-output "$P" --source visible --match 'Sesh>' --timeout 5000
"$HSV" x "$RUN" herdr pane send-text "$P" ses     # literal text into the filter
"$HSV" x "$RUN" herdr pane send-keys "$P" enter   # key names: enter, esc, up, down, ctrl+x, ctrl+o, ctrl+r, ctrl+s, f2, y
"$HSV" x "$RUN" herdr pane read "$P" --source visible
```

Use `"Sesh Settings"` for `open-settings`. Screen rows are column-padded: match rows with `wait-output --regex 'config +sesh'`, not literal row text.

**Isolated tmux (direct binary).** For `herdr-sesh picker`, `picker --fzf`, and standalone `config edit`, which users can also run from a shell:

```sh
"$HSV" tui  "$RUN" pick herdr-sesh picker      # 120x32 tmux session "pick" on socket hsv-<id>, cwd $HOME
"$HSV" wait "$RUN" pick 'Sesh>' 10
"$HSV" keys "$RUN" pick -l ses                 # -l = literal text; otherwise tmux key names (Enter, Escape, C-x, F2)
"$HSV" keys "$RUN" pick Enter
"$HSV" snap "$RUN" pick picker-tmux-after      # screen -> evidence/picker-tmux-after.txt
```

When the command exits, the pane shows `[hsv exit N]` and stays readable.

**CLI.** `"$HSV" cli "$RUN" <label> <cmd…>` runs any command (`herdr-sesh …` or `herdr …`) under the isolated env (run binary first on `PATH`, `HERDR_PLUGIN_CONFIG_DIR` set to the fixture config) and saves `<label>.cmd/.out/.err/.exit`.

Per-feature recipes: [features/README.md](features/README.md). Each feature lists every entry point; a proof that drives only one of several is partial and must say so.

## Evidence

Location: `$TARGET_REPO/scratch/verify-herdr-sesh/<run-id>/` (`scratch/` is gitignored; `env.sh` exports it as `HSV_EVIDENCE`). It survives `down`.

Proof standards:

- Exercise the real user path: plugin action → overlay pane → keys. `herdr-sesh connect` is a separate CLI entry point, not proof of the picker.
- Capture the action *and* the resulting state: screen before the key (`pane read` / `snap`), screen after, then a second read-only view of the side effect.
- Verify side effects, not just screens: `workspace list` / `tab list` / `pane read` of the new workspace's pane (startup command output), `diff` of the plugin `config.toml` after a settings save, focus changes for `last`.
- CLI proof = command, stdout, stderr, exit code (`hsv.sh cli` writes all four).
- Every artifact goes in `$HSV_EVIDENCE`, named `<feature-id>-<step>`: `"$HSV" cli "$RUN" picker-open herdr pane read "$P" --source visible` (→ `picker-open.out`), or `"$HSV" x "$RUN" sh -c 'herdr workspace list > "$HSV_EVIDENCE/picker-ws-before.json"'`.
- No mocks: the isolated server is a real Herdr; `internal/herdr/fake_client.go` is for unit tests only.

## Cleanup

```sh
"$HSV" down "$RUN"
```

Kills the run's tmux server (`-L hsv-<id>`), sends `herdr server stop` to the run's socket, then signals the recorded server PID and its children (history watcher, pane shells) only if each still has the start time recorded for it (`$RUN/server.start`, `$RUN/children`, `ps -o lstart`). Cleanup waits up to five seconds after signaling for confirmed exit before deleting `$RUN` (including its plugin build). If a process remains alive or cannot be inspected, `down` exits non-zero and keeps the run and ownership records for a retry, including orphaned children. A reused server PID is never signaled. Evidence stays. After changing `down`, run `"$VERIFY_DIR/scripts/hsv_test.sh"` (decoy processes and a stub `herdr`; never touches a real server). Never `pkill herdr` or `pkill herdr-sesh`: the user's real server and watchers share those names. Run `down` after every failed attempt too (a failed `up` already attempted it); `ls -d /tmp/hsv.*` lists runs to check (only tear down ones you started).

## Isolation notes

- Runs are side-by-side safe: own dir, socket, tmux socket, and plugin build (`$RUN/plugin`). Editing the checkout does not change a running run's binary until `rebuild`; doctor's freshness check flags it.
- `x` is the only sanctioned way into the env: it refuses a bad `RUN` instead of falling back to the caller's Herdr. See Launch → Running commands.
- Without `XDG_STATE_HOME` isolation a second Herdr writes into the user's `~/.local/state/herdr/plugins/fullerzz.sesh` and its fresh session history can be seeded from the user's legacy `history.json` (observed: `last` tried to focus a workspace ID from the user's real session). `env.sh` prevents this; doctor checks it.
- Herdr reports `/tmp/...` paths as `/private/tmp/...` on macOS. Compare with `realpath`.
- Pane shells start with the isolated empty `HOME` (no user rc files), so prompts look like `user@host sesh %`.

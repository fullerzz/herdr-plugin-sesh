---
name: verify-herdr-sesh
description: Drive herdr-sesh's real CLI and native picker with isolated fixtures, capture evidence, and verify workspace or remote behavior against an explicitly selected Herdr host. Use after CLI, picker, configuration, startup, or preview changes.
disable-model-invocation: true
---

# Verify herdr-sesh

The primary surface is the `herdr-sesh` CLI and native terminal picker.
The desktop app mods and legacy Herdr TUI plugin are additional entry points.
Read [the feature map](features/README.md) before choosing a journey.

## Launch

Run from the checkout root. Prerequisites are the tools pinned by `mise.toml`
(`mise install` when needed). Use the repository build; do not install/link mods
or replace the user's installed binary just to verify a CLI change.

```sh
just build
VERIFY="$PWD/.agents/skills/verify-herdr-sesh/scripts/verify.py"
EVIDENCE="$PWD/scratch/verify-herdr-sesh/$(date +%Y%m%d-%H%M%S)-$$"
uv run --frozen python "$VERIFY" launch "$EVIDENCE"
uv run --frozen python "$VERIFY" doctor "$EVIDENCE" > "$EVIDENCE/doctor.txt"
```

The launcher requires a new evidence directory. It copies the just-built binary,
records its hash, HEAD and working diff, and creates an empty private HOME,
XDG config/state/cache, two workspace directories, and a native TOML fixture.
It starts no server and needs no authentication, port, migration, or database.
Ready means doctor prints the build version and `private fixture ready`.

The helper passes a fresh environment to the binary: no inherited Herdr/hc
sockets, plugin state, zoxide database, shell profiles or theme settings. The
production `HERDR_BIN_PATH` boundary points to `/usr/bin/false`: Herdr is
**unavailable**, not simulated as successful. PATH is `/usr/bin:/bin`.
These runs can coexist because each has its own runtime directory and binary.

Do not use these fixtures to claim successful workspace creation or mod behavior.
For those, follow [workspace lifecycle](features/workspaces.md). The running
desktop app is shared even when Sesh's state directory is private; never have two
agents drive it concurrently. Do not launch another app/core to bypass that limit.

## Doctor

```sh
uv run --frozen python "$VERIFY" doctor "$EVIDENCE"
```

Doctor is read-only: it verifies runtime ownership, the copied binary hash,
fixture presence, and a successful `--version`. It does not claim a live host
is healthy. A copied binary remains a snapshot: rebuild and launch a new run
after source changes. For live app work also capture `herdr-app status`,
`herdr-app ls`, `hc --version`, and `herdr-app --version` before driving it.
Unexpected overlays, errors, missing tools, or a different host mean stop and
resolve that precondition; do not silently change backends.

## Drive

One fully scripted real CLI journey:

```sh
uv run --frozen python "$VERIFY" catalog "$EVIDENCE"
```

It asserts two configured workspaces through both `list --json` and
`picker --json`, checks the human-readable list, renders the alpha directory
preview (`README.md`), validates configuration, and checks that configuration
was not modified. It records `catalog-proof.json` only after all checks pass.
`picker --json` proves data collection, **not interactive rendering or keys**.

For individual commands use a unique evidence label:

```sh
uv run --frozen python "$VERIFY" cli "$EVIDENCE" config-path config path
uv run --frozen python "$VERIFY" cli "$EVIDENCE" preview-beta preview verify-beta
```

For a TUI, use your host's owned PTY (or this macOS `script` command from an interactive terminal):

```sh
script -q "$EVIDENCE/picker.terminal.txt" uv run --frozen python "$VERIFY" tui "$EVIDENCE" picker-interactive picker
```

Wait for `Sesh>` and the fixture names; type `verify-beta`, capture the filtered
screen, then press Escape. The helper records argv and exit; `script` captures
the PTY. See [picker](features/picker.md) for checks and other entry points.
Use actual keyboard input, not a pipe, for interactive claims. The helper caps
CLI calls at 30 seconds and TUI calls at 180 seconds. A timeout is a failure.

## Evidence

All proof stays in `$EVIDENCE` (`scratch/` is gitignored). Each CLI action writes
`LABEL.command.json`, `.stdout`, `.stderr`, and `.exit`, including failure output.
Labels cannot be reused. Keep `manifest.json`, `environment.json`, `fixture.toml`,
the copied binary, revision and diff alongside results. Do not share artifacts
without checking for project paths, private filenames and configuration values.

Exercise real user commands or keys, and capture both the action and the result.
For mutations, also inspect config files, terminal output, workspace IDs and
focus through a second read-only interface. A successful exit or final screen
alone is insufficient. Backend-unavailable checks and unit fakes do not prove
live integration; screenshots do not prove persistence. There is no generic
dry-run switch. Never assume a command avoids network or navigation by its name.
Report each mapped entry point as passed, failed, blocked, or untested.

## Cleanup

First exit any interactive command and wait for its owned PTY/process to end.
For live journeys, close only the recorded spaces created by that journey and
restore the original selected tab. Do not stop the user's app/core, kill by
process name, or delete their history to erase test navigation.

```sh
uv run --frozen python "$VERIFY" cleanup "$EVIDENCE"
test ! -e "$EVIDENCE/runtime"
test -f "$EVIDENCE/catalog-proof.json"
test -f "$EVIDENCE/list-json.stdout"
test -f "$EVIDENCE/cleanup.json"
```

Cleanup checks the ownership marker and removes only `runtime/`, retaining the
binary and every evidence artifact. It is repeatable after successful cleanup.
Run it after failed drives too; missing proof files then remain failures. If
launch failed before writing a manifest, inspect that partial directory before
removing anything. Refuse cleanup while a TUI still uses the run.

## Helpers

`scripts/verify.py` is executable; invoke it with `uv run --frozen python` as
above, from the checkout root. Commands: `launch`, `doctor`, `catalog`,
`cli EVIDENCE LABEL COMMAND...`, `tui EVIDENCE LABEL COMMAND...`, and `cleanup`.
It uses only Python's standard library. The existing `just test-app-last` is a
separate **live navigation test**, documented in the lifecycle feature map.

Use `/maintain-verification-skill` when commands, keys, host contracts or fixtures
change. Update the map and rerun affected journeys rather than only updating prose.

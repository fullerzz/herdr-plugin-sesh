# Workspace lifecycle

Users open configured sessions, reconnect, apply layouts, and switch back with last.

## Sub-features

- `connect-create`: create once and reconnect to the same ID.
- `layout-startup`: tab/pane paths, environment and startup readiness are correct.
- `connect-no-focus`: preserve the previously selected tab.
- `history-last`: return to the preceding open workspace.

## How to get to it (user POV)

Run `connect TARGET`, `connect --no-focus TARGET`, or `last`; select with Enter
from the picker. App bindings `mod.open:sesh`/`mod.open:sesh-last` and TUI plugin
actions are separate user entry points.

## Driving it with verify.py

Preconditions: build/launch/doctor; an explicitly selected live host, no concurrent
driver, and no preexisting space named `verify-alpha` or `verify-beta`.
The private helper cannot drive this journey: its backend is unavailable.

Save `herdr-app status` and `herdr-app ls` in evidence before acting. Refuse if
fixture names already exist or an overlay is open. In a shell with the intended
app's normal environment (not the helper's private environment), run:

Before changing `XDG_STATE_HOME`, set `HC_SOCKET` to the verified live core socket
and export it. Otherwise hc derives a different socket from the private state
directory (and long evidence paths can exceed the Unix socket path limit).
Use the socket reported by the session preflight; confirm `hc get "$HC_TERMINAL_ID"`
still resolves the caller with that explicit socket. Do not start a new core.

```sh
HERDR_SESH_BACKEND=app XDG_STATE_HOME="$EVIDENCE/live-state" ./bin/herdr-sesh connect --no-focus --config "$EVIDENCE/runtime/config.toml" verify-alpha
herdr-app ls > "$EVIDENCE/live-after-create.json"
herdr-app status > "$EVIDENCE/live-after-status.json"
HERDR_SESH_BACKEND=app XDG_STATE_HOME="$EVIDENCE/live-state" ./bin/herdr-sesh connect --no-focus --config "$EVIDENCE/runtime/config.toml" verify-alpha
herdr-app ls > "$EVIDENCE/live-after-reconnect.json"
```

Require exactly one new alpha space at the fixture path, the same ID after
reconnect, and the original selected tab still shown. Capture command exit codes
and stderr, not just these JSON snapshots. Record every newly created space ID.
For layout coverage, seed a tab with root/server and down/logs panes in the
disposable config, each printing its path and a harmless env marker; server must
print a `wait_for` readiness marker. Inspect via `hc read` using returned terminal
IDs. Missing layout fixtures means this sub-feature is untested.

To exercise `last`, connect alpha then beta without `--no-focus` using the same
private `XDG_STATE_HOME`, run `herdr-sesh last` with that environment, and require
alpha selected plus matching private history. For installed mod navigation,
the existing `just test-app-last` requires at least two spaces and a loaded
`sesh-last` recorder. Read `.github/scripts/test-app-last.py` first: it changes
navigation and the user's app history despite restoring focus.

Cleanup: restore the original tab with `herdr-app do tab.select --id "$ORIGINAL_TAB"`;
close each recorded test space only with
`herdr-app do space.close --id "$CREATED_SPACE" --confirmed true`.
Confirm those IDs disappeared before helper cleanup removes their directories.

## Gotchas

App/core are shared; Sesh's isolated history does not isolate host navigation or
the recorder mod. Recent-space entries may remain. Remote creation additionally
needs a registered machine and a disposable directory on that machine; add
`machine` only to that run's config and retain the returned qualified IDs.
The TUI backend should reject machine workspaces; it is not an alternate way to
prove remote creation. Root env launches may run profiles twice. Splits send
their directory command after shell startup; test a profile that changes cwd
when verifying that behavior. Never clean another user's spaces or stop their core.

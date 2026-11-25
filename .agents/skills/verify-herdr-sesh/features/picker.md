# Interactive picker

Users search sessions and select one, or cancel without opening anything.

## Sub-features

- `picker-filter`: filter by name and clear the query.
- `picker-cancel`: return to the terminal without selecting a workspace.
- `picker-open`: select a result and observe the target workspace on a live host.
- `picker-scroll`: scroll a long result list and resize a side-by-side preview.
- `picker-sort`: Ctrl+R cycles workspace, recent and agent ordering.
- `picker-close`: Ctrl+X closes the selected open Herdr workspace.

## How to get to it (user POV)

- `herdr-sesh picker` or `herdr-sesh picker --fzf`.
- Herdr app key binding `mod.open:sesh`.
- Herdr TUI action `herdr plugin action invoke fullerzz.sesh.open-picker`.

## Driving it with verify.py

Preconditions: private launch/doctor; a real interactive PTY sized at least 100x30.

```sh
script -q "$EVIDENCE/picker.terminal.txt" uv run --frozen python "$VERIFY" tui "$EVIDENCE" picker-interactive picker
```

Capture the opening `Sesh>` screen with both fixtures. The query field starts
with focus. The first Down or Ctrl+J moves focus into the list but keeps the
highlight in place. The next press moves it, and the heading counter (`1/2`)
shows the highlighted row. Type `verify-beta` and
require only beta in the result list. Press Ctrl+U and require both names again.
Press Ctrl+R and check that the footer changes from workspace to recent. Press
Ctrl+X on an unopened fixture and confirm both rows remain. Press Escape;
require exit 0, restored terminal input, and no history/workspace
creation. Preserve the terminal transcript plus screenshots before/after filtering.

For mouse coverage, add enough uniquely named workspace directories to the disposable
config to overflow the list. Drag its scrollbar down and check that the selected
row stays visible and the position in the heading advances. With a side-by-side
preview, drag the dotted divider and check that both panes resize while the
selected workspace stays the same.

Enter and a single click on a result both choose it. Verify successful selection
on the live host with the preconditions, `HC_SOCKET` and private zoxide from
[workspace lifecycle](workspaces.md), in a private tmux pane:

```sh
HERDR_SESH_BACKEND=app XDG_STATE_HOME="$EVIDENCE/live-state" _ZO_DATA_DIR="$EVIDENCE/runtime/zoxide" "$EVIDENCE/herdr-sesh" picker --config "$EVIDENCE/runtime/config.toml"
```

Require exit 0 and exactly one new space at the fixture path, focused in
`herdr-app status`. Record its ID. For a click, highlight one row and click
another; the clicked one must be created.

To verify Ctrl+X, type the name of an open workspace created by this drive so
it is the only row, then press Down. Ctrl+X closes whatever is highlighted, so
require heading `1/1` and a `[herdr]` row first. Its recorded ID must disappear
from `herdr-app ls`, and the picker then shows the name as a `[config]` row.

For app/plugin entry points, capture the host before invoking the binding or
action (`herdr-app do mod.open --name sesh` toggles the desktop dialog). In the
desktop app, check the modal picker against the host window (80.5% of its
width and 86.25% of its height). `herdr-app status` lists overlays by name only,
so measure the dialog edges in a `herdr-app shot` image. Close it by sending
Esc to the `sesh=picker` terminal (`hc keys ADDRESS esc`); closing the overlay
any other way leaves the picker running. Use the same filter, clear and select
keys and inspect the resulting workspace independently.

## Gotchas

The private run intentionally cannot create a workspace, and it may display a
Herdr-unavailable warning. Enter is not a successful selection test here.
The fzf entry needs an fzf executable; the private PATH excludes it. Its local
preview looks for bat and displays a message if it is missing. Record fzf
coverage separately. Exit the owned PTY before helper cleanup.

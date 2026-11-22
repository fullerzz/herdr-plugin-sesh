# Interactive picker

Users search sessions and select one, or cancel without opening anything.

## Sub-features

- `picker-filter`: filter by name and clear the query.
- `picker-cancel`: return to the terminal without selecting a workspace.
- `picker-open`: select a result and observe the target workspace on a live host.
- `picker-scroll`: scroll a long result list and resize a side-by-side preview.

## How to get to it (user POV)

- `herdr-sesh picker` or `herdr-sesh picker --fzf`.
- Herdr app key binding `mod.open:sesh`.
- Herdr TUI action `herdr plugin action invoke fullerzz.sesh.open-picker`.

## Driving it with verify.py

Preconditions: private launch/doctor; a real interactive PTY sized at least 100x30.

```sh
script -q "$EVIDENCE/picker.terminal.txt" uv run --frozen python "$VERIFY" tui "$EVIDENCE" picker-interactive picker
```

Capture the opening `Sesh>` screen with both fixtures. Type `verify-beta` and
require only beta in the result list. Press Ctrl+U and require both names again.
Press Escape; require exit 0, restored terminal input, and no history/workspace
creation. Preserve the terminal transcript plus screenshots before/after filtering.

For mouse coverage, add enough uniquely named workspace directories to the disposable
config to overflow the list. Drag its scrollbar down and check that the selected
row stays visible and the position in the heading advances. With a side-by-side
preview, drag the dotted divider and check that both panes resize while the
selected workspace stays the same.

Selection with Enter requires the live host journey. For app/plugin entry points,
capture the host before invoking the exact binding/action above, then use the same
filter/clear/select keys and inspect the resulting workspace independently.

## Gotchas

The private run intentionally cannot create a workspace, and it may display a
Herdr-unavailable warning. Enter is not a successful selection test here.
The fzf entry needs an fzf executable; the private PATH excludes it. Its local
preview looks for bat and displays a message if it is missing. Record fzf
coverage separately. Exit the owned PTY before helper cleanup.

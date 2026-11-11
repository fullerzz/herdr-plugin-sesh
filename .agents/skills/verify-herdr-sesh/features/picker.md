# Interactive picker

Users search sessions and select one, or cancel without opening anything.

## Sub-features

- `picker-filter`: filter by name and clear the query.
- `picker-cancel`: return to the terminal without selecting a workspace.
- `picker-open`: select a result and observe the target workspace on a live host.

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

Selection with Enter requires the live host journey. For app/plugin entry points,
capture the host before invoking the exact binding/action above, then use the same
filter/clear/select keys and inspect the resulting workspace independently.

## Gotchas

The private run intentionally cannot create a workspace, and it may display a
Herdr-unavailable warning. Enter is not a successful selection test here.
The fzf entry needs installed fzf/bat and its own live environment; the private
PATH excludes them. Report blocked rather than treating native-picker proof as
fzf proof. Keep foreground PTY work owned; exit it before helper cleanup.

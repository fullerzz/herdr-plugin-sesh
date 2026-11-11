# Configuration

Users locate, validate, edit or migrate their Sesh configuration.

## Sub-features

- `config-path`: identify the file actually selected.
- `config-validate`: accept valid native TOML; reject invalid values.
- `config-save`: review and persist settings through the editor.
- `config-migrate`: preserve legacy input while creating native configuration.

## How to get to it (user POV)

Run `config path`, `config validate`, `config edit`, or `config migrate PATH`.
The native picker also opens settings with F2; Herdr TUI provides
`herdr plugin action invoke fullerzz.sesh.open-settings`.

## Driving it with verify.py

Preconditions: a fresh private run. Use unique labels per action.

```sh
uv run --frozen python "$VERIFY" cli "$EVIDENCE" path config path
uv run --frozen python "$VERIFY" cli "$EVIDENCE" valid config validate
script -q "$EVIDENCE/settings.terminal.txt" uv run --frozen python "$VERIFY" tui "$EVIDENCE" settings config edit
```

Require `path.stdout` to name `runtime/config.toml`. For editor proof, capture
the screen, change one visible setting, use the displayed review/save controls,
confirm, and reopen the editor. Capture the config diff and run validation again;
the persisted value and reopened control must agree. Cancel a second edit and
require an unchanged file. Use the visible footer because editor keys differ
from picker keys.

For migration, copy the repository's `testdata/sesh.toml` to
`$EVIDENCE/runtime/legacy/sesh.toml` (create `legacy/` first), preserve its hash,
then run:

```sh
uv run --frozen python "$VERIFY" cli "$EVIDENCE" migrate config migrate "$EVIDENCE/runtime/legacy/sesh.toml"
uv run --frozen python "$VERIFY" cli "$EVIDENCE" migrated-valid config validate "$EVIDENCE/runtime/legacy/config.toml"
```

Require the source hash unchanged, native destination present, and validation 0.

## Gotchas

The fixture already exists, so `config init` here proves only reuse, not first-run
creation. Legacy migration has its own precondition; do not run it on personal
files. F2/plugin entry coverage requires driving those entry points, not just
the standalone editor. Preserve edited configs in evidence before cleanup.

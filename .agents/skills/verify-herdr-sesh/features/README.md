# herdr-sesh verification map

Launch and doctor via [SKILL.md](../SKILL.md). The maintained journeys are:

| Feature | Baseline | What a pass proves |
| --- | --- | --- |
| [Catalog](catalog.md) | Private fixture | CLI list, JSON picker data, source isolation |
| [Picker](picker.md) | Private fixture + PTY | Actual filtering, cancellation, terminal recovery |
| [Configuration](configuration.md) | Private config | Lookup, validation, editing and migration persistence |
| [Previews](previews.md) | Private fixture; remote host for remote cases | Content from the correct filesystem and useful failure output |
| [Workspace lifecycle](workspaces.md) | Explicit live host | Create/select, layout startup, history and cleanup |

The seed has `verify-alpha` (README.md) and `verify-beta` (empty), no existing
workspaces, no remote machine, and no legacy file. Each absent precondition must
be prepared or reported as blocked. Do not substitute JSON output for TUI proof,
or a CLI connect for opening the picker through its host mod/plugin.

Feature entries use the same four sections. Record the entry point, command or
keys, before/after evidence, side-effect check, and result. A proof of one entry
point is partial when the feature lists others. Other commands (`clone`, `root`,
`window`) are outside this initial map; add them when verifying those features.

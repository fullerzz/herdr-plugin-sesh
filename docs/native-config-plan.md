# Herdr-native configuration implementation plan

## Goal

Introduce a versioned configuration format owned by `herdr-sesh`, using Herdr
terminology and semantics instead of mirroring Sesh. Preserve existing users by
continuing to read the current Sesh-compatible subset during a migration period.

The first implementation should change configuration decoding and discovery,
not the downstream session, picker, preview, or startup behavior.

## Decisions for version 1

- The native filename is `config.toml` inside the plugin-owned config directory.
- Every native file starts with `version = 1`.
- Native files always reject unknown keys. There is no `strict_mode` switch.
- Native files use `workspace`, `tab`, and `rule` instead of Sesh's `session`,
  `window`, and `wildcard` terms.
- Native version 1 does not support imports. The legacy loader retains existing
  import behavior.
- Existing environment variable names such as `HERDR_SESH_CONFIG` remain valid;
  they refer to the product, not the legacy schema.
- Native configuration is translated into the existing runtime `config.Config`
  model so consumers do not need to change in the first release.
- Legacy files remain readable and emit one migration warning on stderr. Do not
  add an automatic migration command until manual migration proves burdensome.

## Proposed native schema

```toml
version = 1

[list]
cache = true
source_order = ["herdr", "config", "zoxide", "dir"]
blacklist = ["^scratch$"]

[naming]
path_components = 1

[picker]
show_icons = true
prompt = "Sesh> "
placeholder = "Search workspaces"
separator_aware = true
workspace_sort = "recent"

[workspace_defaults]
startup = "git status"
# Absent or "" both fall back to the built-in default preview command.
preview = "eza --icons=always -la {}"

[[tab]]
name = "git"
startup = "git status"
path = "~/projects"

[[workspace]]
name = "brain"
path = "~/brain"
disable_startup = true
tabs = ["git"]

[[rule]]
path_glob = "~/projects/**"
startup = "git status"
preview = "eza --icons=always -la {}"
tabs = ["git"]
```

Keep the current source identifiers in `source_order` for version 1. Renaming
them would also change CLI JSON/list behavior and should be handled separately.

## Legacy-to-native mapping

| Legacy key | Native key |
| --- | --- |
| `cache` | `list.cache` |
| `strict_mode` | Removed; native decoding is always strict |
| `import` | Unsupported in native version 1 |
| `blacklist` | `list.blacklist` |
| `sort_order` | `list.source_order` |
| `dir_length` | `naming.path_components` |
| `separator_aware` | `picker.separator_aware` |
| `tui.show_icons` | `picker.show_icons` |
| `tui.prompt` | `picker.prompt` |
| `tui.placeholder` | `picker.placeholder` |
| `tui.default_sort` | `picker.workspace_sort` |
| `default_session.startup_command` | `workspace_defaults.startup` |
| `default_session.preview_command` | `workspace_defaults.preview` |
| `session[]` | `workspace[]` |
| `session[].startup_command` | `workspace[].startup` |
| `session[].preview_command` | `workspace[].preview` |
| `session[].disable_startup_command` | `workspace[].disable_startup` |
| `session[].windows` | `workspace[].tabs` |
| `window[]` | `tab[]` |
| `window[].startup_script` | `tab[].startup` |
| `window[].path` | `tab[].path` |
| `wildcard[]` | `rule[]` |
| `wildcard[].pattern` | `rule[].path_glob` |
| `wildcard[].startup_command` | `rule[].startup` |
| `wildcard[].preview_command` | `rule[].preview` |
| `wildcard[].disable_startup_command` | `rule[].disable_startup` |
| `wildcard[].windows` | `rule[].tabs` |

Fields not listed above, including `name`, `path`, and tab references, keep the
same meaning under their renamed table.

### Set-versus-unset semantics

The legacy loader probes the raw document so that an explicitly empty
`default_session.preview_command = ""` overrides an imported value, after which
`attachDefaults` restores the built-in default preview
(`TestLoadExplicitEmptyPreviewCommandRestoresDefault`). Native has no imports,
so the probe machinery collapses: absent and `""` both resolve to the built-in
default preview, and plain string fields suffice for every native key. No
`*string` is needed for parity.

## Implementation phases

### 1. Lock the native contract

- Add native TOML-only structs in `internal/config/native.go`.
- Keep the existing `Config` type as the normalized runtime representation.
- Add a `decodeNative` function that strictly decodes `version = 1`, validates
  the document, and converts it to `Config`.
- Reject a missing, zero, or unsupported version for files discovered at native
  `config.toml` locations.
- Add a complete native fixture at `testdata/herdr-sesh.toml` (the name the
  verification gate below expects) and table tests for every key.

### 2. Isolate the legacy decoder

- Move the current TOML tags, import traversal, probes, and merge behavior behind
  `decodeLegacy` without changing their behavior.
- Treat an unversioned file selected through an explicit path or
  `HERDR_SESH_CONFIG` as legacy during the migration period. A present `version`
  always selects native decoding.
- Add one parity test that decodes equivalent native and legacy fixtures and
  compares their normalized runtime `Config` values.
- Preserve current legacy edge cases until they are separately deprecated; do
  not fix import precedence as part of the native schema change.

### 3. Unify discovery and generated paths

Use one resolver for loading, `config path`, and `config init`.
The filename decides whether a default candidate is native or legacy; the
`version` key decides for explicit `--config` and `HERDR_SESH_CONFIG` paths,
which can point at either schema.

Discovery order:

1. Explicit `--config PATH`.
2. `HERDR_SESH_CONFIG`.
3. `${HERDR_PLUGIN_CONFIG_DIR}/config.toml`.
4. `${HERDR_PLUGIN_CONFIG_DIR}/sesh.toml` as a legacy fallback.
5. `~/.config/herdr-sesh/config.toml`.
6. `~/.config/herdr-sesh/sesh.toml` as a legacy fallback. Today `config init`
   without `HERDR_PLUGIN_CONFIG_DIR` writes this file but the loader never
   reads it; adding this candidate finally honors those files.
7. `~/.config/sesh/sesh.toml` as a legacy fallback.

Additional behavior:

- An explicit missing path remains an error. A `HERDR_SESH_CONFIG` path that
  does not exist becomes an error too — it is explicit intent, and today's
  silent skip to the next candidate hides typos.
- Native config wins when both native and legacy default files exist.
- `config path` reports the file the resolver would actually load; when no
  candidate exists it reports the native `config.toml` destination that
  `config init` would create.
- `config init` writes the native starter file with mode `0600` and never
  overwrites an existing file.
- Loading a fallback legacy file writes a concise deprecation warning to
  stderr, never stdout, so JSON output remains valid. Emit it before the
  picker TUI starts drawing.

### 4. Add semantic validation

Validate native files before conversion:

- `picker.workspace_sort` is `workspace` or `recent`.
- `naming.path_components` is at least `1`.
- Source names are known and not duplicated.
- Blacklist regexes and rule globs compile successfully.
- Workspace names and paths are non-empty and workspace names are unique.
- Tab names are non-empty and unique. Tab `path` stays optional and is not
  validated beyond decoding, matching legacy behavior.
- Every referenced tab exists.
- Rule path globs are non-empty.

Return errors containing the config path and failing key. Do not silently ignore
invalid native values.

### 5. Preserve runtime behavior

- Convert native workspaces, tabs, rules, defaults, picker settings, and list
  settings into the existing `Config` fields.
- Keep command precedence as explicit workspace, first matching rule, then
  workspace defaults.
- Preserve the tri-state behavior of `disable_startup` by representing the
  workspace value as `*bool` during conversion.
- Keep `workspace_defaults.preview` falling back to the built-in default when
  absent or empty, matching legacy `attachDefaults` behavior.
- Keep `{}` shell-quoted path substitution unchanged.
- Keep the five-second list cache and current cache identity unchanged.
- Verify native configuration through `list`, `picker --json`, `connect`, and
  `preview`, not only through decoder tests.

### 6. Documentation and migration

- Replace `docs/config.md` with the native schema reference and full example.
- Add a legacy migration table based on the mapping above. Note that a
  non-strict legacy file containing a stray `version` key was silently
  ignored before and now selects native decoding, which fails hard.
- Update README lookup order and stop describing the primary format as a Sesh
  subset.
- Update `testdata/sesh.toml` only as a legacy fixture; add a clearly named native
  fixture for normal smoke checks.
- Update smoke commands and CI to use the native fixture, including the
  hardcoded `testdata/sesh.toml` in the `justfile` `release` recipe.
- Keep legacy support for at least one released version. Remove it only through a
  separately announced breaking release.

## Test and verification matrix

- Native defaults with no config file.
- Native happy path containing every supported key.
- Unknown native field and unsupported version failures.
- Invalid enum, regex, glob, duplicate name, and missing tab failures.
- Every discovery candidate and native-over-legacy precedence.
- Missing explicit `--config` and missing `HERDR_SESH_CONFIG` both error.
- Empty-string and absent `workspace_defaults.preview` both resolve to the
  built-in default preview.
- `config path` and `config init` matching loader discovery, including the
  no-candidate case reporting the init destination.
- Legacy path, explicit legacy file, imports, and strict-mode compatibility.
- Equivalent native/legacy runtime configuration.
- Native behavior through list caching, picker settings, preview selection,
  startup disablement, rule precedence, and tab creation.
- Warning output remains on stderr and JSON output remains parseable.

Run the repository gate after each implementation phase:

```bash
just fmt-check
just lint
just test
just build
./bin/herdr-sesh --version
./bin/herdr-sesh list --json --config testdata/herdr-sesh.toml
git diff --check
```

## Acceptance criteria

- A generated native config is automatically discovered in both managed-plugin
  and standalone use.
- Native configuration is versioned, strict, semantically validated, and fully
  documented.
- Every current runtime capability has a native representation except imports.
- Existing unversioned Sesh-compatible files still load without behavior drift.
- Native and legacy decoders converge on one runtime model.
- No command or JSON-output consumer needs to know which schema was loaded.

## Non-goals for version 1

- Full compatibility with upstream Sesh.
- Native imports or cross-file merge rules.
- Automatic config rewriting.
- Renaming source identifiers in CLI output.
- Changing cache duration, preview execution, startup execution, or picker
  rendering beyond what the new schema exposes.

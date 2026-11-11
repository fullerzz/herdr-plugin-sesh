# Session catalog

Users list configured projects and open workspaces before choosing where to go.

## Sub-features

- `catalog-json`: machine-readable names, sources and paths.
- `catalog-text`: readable names and paths.
- `picker-data`: the picker collects the same seeded sessions.

## How to get to it (user POV)

Run `herdr-sesh list`, `herdr-sesh list --json`, or `herdr-sesh picker --json`.

## Driving it with verify.py

Preconditions: launch and doctor passed; fixtures have not been edited.

```sh
uv run --frozen python "$VERIFY" catalog "$EVIDENCE"
```

Require exactly `verify-alpha` and `verify-beta`, source `config`, no machine,
and paths under this run's `runtime/`. `catalog-proof.json` records passed checks;
read the individual command/output files as the evidence, not just that summary.
The same journey also checks alpha's preview and unchanged valid configuration.

## Gotchas

The private environment excludes zoxide and deliberately makes Herdr unavailable.
Thus this does not prove real-source deduplication, cached catalogs, blacklist
options, or remote entries. No live source should silently enter the seeded list.

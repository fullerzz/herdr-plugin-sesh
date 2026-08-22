# Plan 001: Show linked-worktree parentage in the native picker

> **Executor instructions**: Follow this plan step by step. Write the focused
> regression test before each implementation change, run every verification
> command, and confirm the expected result before moving on. If any condition
> in **STOP conditions** occurs, stop and report it instead of improvising.
> When done, update this plan's status row in `plans/README.md` unless a
> reviewer says they maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat d84f49b..HEAD -- internal/herdr/client.go internal/herdr/client_test.go internal/model/session.go internal/model/session_test.go internal/sources/herdr_workspaces.go internal/sources/herdr_workspaces_test.go internal/picker/tea.go internal/picker/tea_test.go README.md docs/config.md`
>
> If an in-scope file changed since this plan was written, compare the excerpts
> in **Current state** with live code. Treat a material mismatch as a STOP
> condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: direction
- **Planned at**: commit `d84f49b`, 2026-08-22

## Why this matters

The native picker currently renders a linked Git worktree workspace exactly
like an unrelated Herdr workspace. Users cannot see that two rows belong to the
same repository or that one workspace is a worktree child of another open
workspace. The change should preserve the picker's compact, one-line rows while
adding an always-visible child marker, the parent workspace label when space
permits, and the same parent-first grouping Herdr uses in its sidebar.

This is a metadata, presentation, and Herdr-workspace ordering change. It must
keep each resolved parent immediately above its linked child rows in both
workspace and recent sort modes, without running Git commands, adding
per-workspace Herdr calls, altering selection semantics, or changing public
`list --json` output.

## Visual contract

### Before and after

The source badge and agent-status column stay where they are. `↳` is a normal
Unicode glyph, not a Nerd Font icon, so the relationship remains visible when
`show_icons = false`.

```text
CURRENT — linked child is indistinguishable

  ✓  [herdr]  herdr-plugin-sesh        ~/Code/Go/herdr-plugin-sesh
     [herdr]  picker-worktree          ~/.herdr/worktrees/.../picker-worktree

PROPOSED — wide list panel (parent-first group)

  ✓  [herdr]  herdr-plugin-sesh        ~/Code/Go/herdr-plugin-sesh
     [herdr]  ↳ picker-worktree        worktree of herdr-plugin-sesh · ~/.herdr/…
     [herdr]  ↳ docs-worktree          worktree of herdr-plugin-sesh · ~/.herdr/…
               └ child markers          └ parent relationship (secondary text)

PROPOSED — narrow list panel

  ✓  [herdr]  herdr-plugin-sesh
     [herdr]  ↳ picker-worktree
               └ the marker survives after secondary text/path are dropped

SELECTED CHILD PREVIEW TITLE

  PREVIEW · picker-worktree · worktree of herdr-plugin-sesh · working
```

If Herdr identifies a linked worktree but no single open parent can be resolved,
render `↳` and use `linked worktree` as the wide/preview description. Never name
or group under an ambiguous parent.

When a parent is resolved, keep the parent and all of its linked children
contiguous, with the parent first. Workspace sort uses Herdr's order for groups
and siblings. Recent sort uses the best (lowest) recent rank of any group member
to position the whole group, then keeps the parent first and ranks child
siblings by recency. Filtering remains literal: if a query hides the parent, do
not inject it merely to provide context; the child marker and relationship text
stand alone.

### Metadata flow

```text
┌──────────────────────┐    ┌────────────────────┐    ┌───────────────────┐
│ herdr workspace list │───▶│ Workspace.Worktree │───▶│ HerdrWorkspaces   │
└──────────────────────┘    └────────────────────┘    │ source resolution │
                                                      └─────────┬─────────┘
                                                                │
                                  ┌─────────────────────────────┴──────────┐
                                  │ linked: match one root by repo_key     │
                                  │ normal: preserve current Session       │
                                  └─────────────────────┬──────────────────┘
                                                        ▼
                                          ┌─────────────────────────┐
                                          │ Session.WorktreeRelation│
                                          └────────────┬────────────┘
                                                       │
                                  ┌────────────────────┼──────────────────┐
                                  ▼                    ▼                  ▼
                         Parent-first group     Native row: ↳      Preview: parent label
```

### Parent-resolution decision

```text
Workspace has worktree metadata
               │
               ▼
       is_linked_worktree?
          ╱             ╲
       no                 yes
       │                   │
       ▼                   ▼
No child marker      Set Linked = true
                           │
                           ▼
              Exactly one non-linked workspace
              has the same non-empty repo_key?
                       ╱          ╲
                    yes            no
                    │               │
                    ▼               ▼
          Attach parent ID/name   Keep parent empty;
                                  show “linked worktree”
```

## Current state

- `internal/herdr/client.go:14-21` models workspace identity, paths, active tab,
  and agent status, but it discards Herdr's nested `worktree` object.

  ```go
  type Workspace struct {
      ID            string `json:"id"`
      Label         string `json:"label"`
      CWD           string `json:"cwd"`
      ForegroundCWD string `json:"foreground_cwd"`
      ActiveTabID   string `json:"active_tab_id"`
      AgentStatus   string `json:"agent_status"`
  }
  ```

- Herdr 0.8.2 was observed at planning time to return this shape from the
  existing `herdr workspace list` call for Git-backed workspaces:

  ```json
  {
    "workspace_id": "w-child",
    "label": "picker-worktree",
    "worktree": {
      "checkout_path": "/example/worktrees/picker-worktree",
      "is_linked_worktree": true,
      "repo_key": "/example/project/.git",
      "repo_name": "project",
      "repo_root": "/example/project"
    }
  }
  ```

  The root checkout's workspace has the same `repo_key` and
  `is_linked_worktree: false`. The existing workspace-list call therefore has
  enough information; do not add `herdr worktree list` calls.

- `internal/sources/herdr_workspaces.go:18-30` currently converts every Herdr
  workspace directly into a flat `model.Session`. It already owns the boundary
  where Herdr-specific metadata should become picker metadata.

  ```go
  for _, w := range ws {
      path := workspacePath(w, panes)
      out.Add(model.Session{
          Source: "herdr", Name: w.Label, Path: path,
          WorkspaceID: w.ID, AgentStatus: w.AgentStatus,
      })
  }
  ```

- `internal/model/session.go:9-24` has internal picker-only metadata such as
  `AgentStatus` with `json:"-"`. Worktree relationship metadata belongs beside
  it and must also remain absent from public JSON.

- `internal/picker/tea.go:1131-1173` renders each row as an ANSI-aware,
  fixed-width line: selection rail, two-cell agent status, fixed-width source
  badge, name, then an optional path. `fitLine`, `fitPlain`, `ansi.Truncate`, and
  `lipgloss.Width` are the established width-safety tools; continue using them.

- `internal/picker/tea.go:684-711` sorts Herdr rows as independent items by
  workspace/recent rank, then writes them back into the existing Herdr slots.
  It does not yet understand parent-child groups; this is the boundary to make
  resolved families contiguous while preserving non-Herdr item positions.

- `internal/picker/tea.go:872-888` builds the selected row's one-line preview
  title from its label and agent status. It is the secondary place to expose
  the parent relationship.

- Structural test exemplars:
  - `internal/herdr/client_test.go:60-78` — envelope and legacy-array JSON decode
    tests.
  - `internal/sources/herdr_workspaces_test.go:10-26` — fake-client source
    conversion.
  - `internal/model/session_test.go:9-25` — internal picker fields stay out of
    JSON.
  - `internal/picker/tea_test.go:807-837` — complete native-picker view.
  - `internal/picker/tea_test.go:1067-1088` — exact-width, no-wrap row behavior.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused baseline | `go test ./internal/herdr ./internal/sources ./internal/picker ./internal/model` | exit 0; all four packages report `ok` |
| Format | `just fmt` | exit 0; Go files formatted |
| Full checks | `just check` | exit 0; lint, format check, race-enabled tests, and release-ref test pass |
| Build | `just build` | exit 0; `bin/herdr-sesh` created |
| Version smoke | `./bin/herdr-sesh --version` | exit 0; prints `herdr-sesh <version>` |
| Listing smoke | `./bin/herdr-sesh list --json --config testdata/herdr-sesh.toml` | exit 0; valid JSON array |

The focused baseline passed at commit `d84f49b` during planning.

## Scope

**In scope — only these files may change:**

- `internal/herdr/client.go`
- `internal/herdr/client_test.go`
- `internal/model/session.go`
- `internal/model/session_test.go`
- `internal/sources/herdr_workspaces.go`
- `internal/sources/herdr_workspaces_test.go`
- `internal/picker/tea.go`
- `internal/picker/tea_test.go`
- `README.md`
- `docs/config.md`
- `plans/README.md` for status only

**Out of scope — do not touch:**

- `internal/picker/fzf.go` and `internal/picker/fzf_test.go`; the request is for
  the native picker only.
- Configuration schema/defaults/migration; this indicator is automatic when
  metadata exists and does not need a setting.
- Filtering, selection, closing, history, or connection behavior. Ordering may
  change only among Herdr rows to keep a resolved parent immediately above its
  linked children; non-Herdr slots and sort-mode controls stay unchanged.
- Herdr workspace creation or worktree creation/removal commands.
- Git subprocesses, filesystem-based `.git` discovery, or extra
  `herdr worktree list` calls.
- Public `list`/`picker --json` schema and `model.Key`; relationship metadata is
  transient presentation state.
- A minimum-Herdr-version bump unless the maintainer separately decides older
  supported versions cannot safely ignore the absent JSON field.

## Git workflow

- Suggested branch: `advisor/001-show-worktree-parentage`
- Keep commits focused; the repository uses imperative Conventional Commit
  subjects such as `feat: cache session list when enabled`.
- Suggested subject: `feat: show worktree parentage in native picker`
- Do not push or open a PR unless instructed by the operator.

## Steps

### Step 1: Decode Herdr's workspace worktree metadata

1. In `internal/herdr/client_test.go`, first extend both workspace-list decode
   coverage paths:
   - envelope response with a non-linked root and linked child sharing a
     `repo_key`;
   - legacy top-level array response with no `worktree` field.
2. In `internal/herdr/client.go`, add a narrow Herdr DTO for the nested object.
   Recommended shape:

   ```go
   type Worktree struct {
       CheckoutPath     string `json:"checkout_path"`
       IsLinkedWorktree bool   `json:"is_linked_worktree"`
       RepoKey          string `json:"repo_key"`
       RepoName         string `json:"repo_name"`
       RepoRoot         string `json:"repo_root"`
   }

   type Workspace struct {
       // existing fields unchanged
       Worktree *Worktree `json:"worktree,omitempty"`
   }
   ```

   A pointer distinguishes "metadata absent on older/non-Git workspaces" from
   a decoded non-linked root checkout.
3. Assert all nested fields needed for relation matching decode, and that an
   omitted object produces `nil` without breaking the older array shape.

**Verify**: `go test ./internal/herdr` → exit 0 and package reports `ok`.

### Step 2: Derive a conservative parent relationship in the source layer

1. In `internal/model/session.go`, add picker-only relationship metadata. Use a
   typed zero-value-safe shape rather than unrelated booleans:

   ```go
   type WorktreeRelation struct {
       Linked              bool
       ParentWorkspaceID   string
       ParentWorkspaceName string
   }

   type Session struct {
       // existing fields unchanged
       Worktree WorktreeRelation `json:"-"`
   }
   ```

   Naming may be adjusted to match nearby conventions, but retain all three
   semantics: linked-child marker, parent ID, and display name.
2. Extend `TestSessionJSONOmitsInternalPickerFields` in
   `internal/model/session_test.go` to populate the relation and assert neither
   field names nor values leak into JSON. Do not add relationship data to
   `model.Key`.
3. In `internal/sources/herdr_workspaces_test.go`, add tests before production
   code for:
   - a linked child and one non-linked root with the same non-empty `repo_key`,
     with the child listed before the parent to prove order independence;
   - a normal Git root, which must not be marked as a child;
   - a linked child whose parent workspace is absent, which remains
     `Linked=true` but has empty parent fields;
   - a linked child with two possible non-linked workspaces sharing `repo_key`,
     which remains linked but has no named parent rather than choosing one;
   - unrelated repositories, which never cross-link;
   - missing worktree metadata, preserving current output.
4. In `internal/sources/herdr_workspaces.go`, make one pre-pass over the
   already-fetched workspace slice. Index non-linked candidates by non-empty
   `RepoKey`. During the existing conversion loop, mark linked worktrees and
   attach a parent only when exactly one candidate exists and its workspace ID
   differs from the child ID.
5. For a uniquely resolved parent, use `parent.Label` as the display name and
   fall back to `parent.ID` if the label is empty. Do not match by `repo_name`,
   basename, or path because those can collide or drift.

**Verify**:
`go test ./internal/model ./internal/sources` → exit 0 and both packages report
`ok`.

### Step 3: Keep resolved worktree families together in both sort modes

1. Add focused tests to `internal/picker/tea_test.go` for the existing
   `sortHerdrWorkspaces` path before changing it. Cover:
   - workspace order containing child-before-parent is normalized to parent,
     then children;
   - multiple children remain contiguous beneath their parent and preserve
     Herdr sibling order in workspace mode;
   - recent order led by a child moves the whole family to that rank while the
     parent still renders first;
   - child siblings use recent rank when available and stable Herdr order for
     ties/unranked siblings;
   - unrelated and unresolved worktree rows remain independent;
   - config/zoxide rows retain their existing slots while only Herdr rows move;
   - filtering may hide a parent without injecting it into results;
   - close/reload and `ctrl+r` toggles rebuild the same invariant.
2. Refactor `sortHerdrWorkspaces` into an explicit family-aware stable sort:
   - build the requested rank map exactly as today;
   - group only rows whose `ParentWorkspaceID` resolves to a Herdr row present
     in the current item set;
   - assign each family the best (lowest) rank among its parent and children so
     a recent child can lift the family;
   - emit the parent first, then children by requested rank with stable input
     order as fallback;
   - treat unresolved, ambiguous, missing-parent, and non-worktree rows as
     singleton groups;
   - write sorted Herdr rows back into the existing Herdr slots so mixed-source
     placement is unchanged.
3. In `newTeaModel`, copy the input slice and run the family-aware sorter for
   both initial modes. Use `workspaceOrder` for default workspace mode and
   `recentWorkspaceIDs` for recent mode. Do not mutate the caller's slice.
4. Keep `toggleWorkspaceSort` and close/reload paths on the same helper rather
   than duplicating grouping logic.

**Verify**: `go test ./internal/picker -run 'Sort|Workspace|Worktree'` → exit 0
and all matching tests pass.

### Step 4: Render the relationship without breaking row geometry

1. Add focused rendering tests to `internal/picker/tea_test.go`. Cover:
   - a non-worktree Herdr row is byte-for-byte/visually unchanged after ANSI is
     stripped;
   - a linked child always contains `↳`, with `show_icons` both true and false;
   - a wide child row contains `worktree of <parent>` and its path when space
     permits;
   - an unresolved child contains generic `linked worktree` text and never a
     made-up parent;
   - a narrow child row retains `↳`, omits secondary text/path, remains exactly
     the requested width, and never wraps;
   - selected/unselected child rows preserve the rail, source color, and agent
     indicator;
   - query highlighting still applies to the child name and path;
   - the preview title contains `worktree of <parent>` or `linked worktree`,
     followed by any agent status;
   - the complete `teaModel.View` remains exactly the terminal width and height.
2. In `internal/picker/tea.go`, add a dedicated relation style using the
   existing palette. Use violet for `↳` (relationship accent) and muted text for
   `worktree of …`; do not add a background fill that competes with selection or
   agent status.
3. Refactor `rowWithRail` only enough to support a linked-child prefix and
   responsive secondary context:
   - render `↳ ` immediately before the child label and account for its display
     width before calling `highlightMatches`;
   - keep `↳` visible at every width where a row itself is visible;
   - at wide widths, make the secondary field begin with
     `worktree of <parent>` or `linked worktree`; append ` · <path>` only when it
     fits;
   - drop/truncate the path before dropping the relationship description;
   - at narrow widths, drop the secondary field but retain the marker;
   - continue returning exactly one `fitLine(..., width)` line.
4. Put width composition into small helpers (for example,
   `worktreeDescription` and an ANSI-aware secondary renderer) rather than
   adding more nested branches to `rowWithRail`. Continue using
   `lipgloss.Width`, `fitPlain`, `fitLine`, and `ansi.Truncate`; never use byte
   length for display width.
5. Extend `previewTitle` after the selected label and before agent status. The
   title may naturally truncate through `previewView` at small preview widths;
   the list marker remains the guaranteed compact indicator.

**Verify**: `go test ./internal/picker` → exit 0 and package reports `ok`.

### Step 5: Document the automatic native-picker behavior

1. Update the README feature list to say the native picker identifies linked
   worktree workspaces and their open parent workspace when resolvable.
2. Extend the native-picker behavior paragraph in `docs/config.md` near the
   agent-status indicators. Document:
   - `↳` means the Herdr workspace is a linked Git worktree;
   - wide layouts show `worktree of <parent>` when one open parent is
     unambiguous;
   - the marker is automatic and independent of `show_icons`;
   - resolved parents appear immediately above their linked children in both
     workspace and recent sort modes, matching Herdr's sidebar pattern;
   - no relationship or grouping is invented when parent metadata is absent or
     ambiguous.
3. Keep fzf documentation unchanged.

**Verify**:
`grep -n "linked.*worktree\|worktree of" README.md docs/config.md` → matches in
both files and exits 0.

### Step 6: Run full validation and inspect the final diff

1. Run `just fmt` once implementation and tests are complete.
2. Run `just check`, then `just build`.
3. Run both CLI smoke commands from **Commands you will need**.
4. If a Herdr session with an open root workspace and linked worktree is
   available, open the native picker and manually confirm the wide and narrow
   visual contract. This supplements tests; it does not replace them.
5. Inspect scope:

   ```bash
   git status --short
   git diff --check
   git diff --stat
   ```

6. Update the row in `plans/README.md` to `DONE` only after all automated checks
   pass. Record a manual-smoke limitation in the PR if no representative Herdr
   worktree was available.

**Verify**:
- `just check` → exit 0.
- `just build` → exit 0 and `bin/herdr-sesh` exists.
- `git diff --check` → no output, exit 0.
- `git status --short` → only in-scope source/docs plus the plan status file.

## Test plan

| Layer | File | Required cases |
|---|---|---|
| Herdr decode | `internal/herdr/client_test.go` | nested worktree metadata in envelope; omitted metadata in legacy array |
| Internal model | `internal/model/session_test.go` | worktree relationship omitted from JSON; session key unchanged by presentation metadata |
| Source mapping | `internal/sources/herdr_workspaces_test.go` | unique parent, child-before-parent, root, absent parent, ambiguous parents, different repo, absent metadata |
| Family ordering | `internal/picker/tea_test.go` | parent-first workspace order, recent-ranked family, sibling stability, mixed-source slots, unresolved child, toggle and reload |
| Row rendering | `internal/picker/tea_test.go` | wide/narrow, icons on/off, selected/unselected, exact width, no wrap, resolved/unresolved parent |
| Full picker | `internal/picker/tea_test.go` | grouped rows, marker, and preview relationship coexist with status, source, dimensions, and preview |
| Regression | existing package tests | filtering, close/reload, last workspace, smear animation, and fzf remain green |

Use the existing tests cited in **Current state** as structural patterns. Prefer
focused named tests over a large table when failure output needs to show a full
rendered row.

## Done criteria

- [ ] `WorkspaceList` decodes the nested worktree object from the existing
      command without adding CLI calls.
- [ ] Every linked worktree session has `Linked=true`; a parent is attached only
      when exactly one non-linked workspace shares its non-empty `repo_key`.
- [ ] Worktree relationship metadata is absent from `list --json` and does not
      change `model.Key`.
- [ ] Native rows show `↳` for linked children with icons both enabled and
      disabled.
- [ ] Wide rows and preview titles name an unambiguous parent; unresolved cases
      say `linked worktree` and do not guess.
- [ ] Narrow rows retain the marker, stay one line, and have exact display width.
- [ ] Every resolved parent immediately precedes its contiguous child rows in
      workspace and recent sort modes; recent rank moves the family as a unit.
- [ ] Non-Herdr slots, filtering, selection, close/reload, status refresh, and
      fzf behavior are unchanged.
- [ ] `just check`, `just build`, both CLI smoke commands, and
      `git diff --check` succeed.
- [ ] No files outside **Scope** are modified.
- [ ] `plans/README.md` status is updated.

## STOP conditions

Stop and report back instead of improvising if:

- Any current-state excerpt has materially drifted from commit `d84f49b`.
- The supported Herdr workspace-list schema does not expose `repo_key` and
  `is_linked_worktree` as described; do not compensate with Git discovery or
  N+1 `herdr worktree list` calls.
- Correct parent resolution requires changing Herdr itself or depending on a
  field unavailable to this plugin.
- Parent-first grouping cannot be implemented without moving non-Herdr slots,
  injecting nonmatching parents into filtered results, changing public JSON,
  adding configuration, or touching fzf.
- ANSI-aware width tests cannot preserve both the marker and exact one-line row
  geometry; report the conflicting terminal width and rendered output.
- A verification command still fails after two reasonable focused fix attempts.
- A required change falls outside the explicit in-scope file list.

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| A `repo_name`/path heuristic assigns the wrong parent | MED | Match only one non-linked workspace with the same non-empty `repo_key`; generic text otherwise |
| Relationship text causes wrapping or hides selection/status | MED | Reserve marker width explicitly and assert exact Lip Gloss width at wide and narrow sizes |
| Older Herdr versions omit metadata | LOW | Pointer/zero-value decoding keeps current behavior unchanged |
| Grouping weakens recent-sort meaning | MED | Rank a family by its most recent member, keep parent first, and order children by recent rank |
| Presentation metadata leaks into CLI JSON/cache keys | LOW | Use `json:"-"`, extend JSON regression test, and leave `model.Key` untouched |

## Maintenance notes

- Reviewers should focus on ambiguous-parent handling and ANSI display-width
  accounting; these are the two places a plausible implementation can silently
  mislead or corrupt layout.
- If Herdr later exposes an explicit source/parent workspace ID in
  `workspace list`, replace repo-key inference with that authoritative field and
  keep the generic unresolved fallback.
- If fzf parity is requested later, plan it separately because its tab-delimited
  hidden fields and preview shell have different width/search constraints.
- Preserve the family ordering contract when future sort modes are added:
  groups take the best rank of any member, parents render first, and child
  siblings follow the active mode's rank with stable fallback.

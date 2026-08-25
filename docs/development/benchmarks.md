---
icon: lucide/gauge
---

# Benchmarks

The benchmark suite measures the paths users notice most: loading Herdr
workspaces, filtering and rendering the picker, and refreshing previews during
rapid navigation. It uses Go's standard benchmark runner and
[`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) for statistical
before-and-after comparisons.

!!! tip "Measure a change against its own baseline"

    Benchmark results depend on the machine, power mode, and background load.
    Compare repeated runs from the same machine instead of treating one result
    as a universal performance budget.

## Run the suite

Install the pinned development tools with `mise install` before running these
commands.

=== "Quick check"

    Run every benchmark once:

    ```bash
    just bench
    ```

=== "Before and after"

    Collect multiple samples before and after the change, then compare them:

    ```bash
    just bench 10 | tee /tmp/herdr-sesh-base.txt
    # Make the change.
    just bench 10 | tee /tmp/herdr-sesh-candidate.txt
    just bench-compare /tmp/herdr-sesh-base.txt /tmp/herdr-sesh-candidate.txt
    ```

    `benchstat` reports the distributions, relative delta, and statistical
    significance. Ten samples are a practical starting point; increase the
    count when results are noisy.

The `bench` recipe skips unit tests with `-run '^$'`, runs benchmarks in
`internal/sources` and `internal/picker`, and includes allocation metrics with
`-benchmem`.

## What is measured

| Benchmark | Workload | Why it exists |
| --- | --- | --- |
| `HerdrWorkspacesList` | Decode and combine 100 generated workspaces and panes through an in-memory Herdr runner, with complete paths and with one path requiring pane fallback. | Isolates workspace-list processing from process startup. |
| `HerdrWorkspacesListProcessBoundary` | Run the same cases through a real helper subprocess. | Captures command-spawn, pipe, JSON, and application overhead at the CLI boundary. |
| `FilterSessions` | Search 100 and 1,000 sessions for queries that match none or some entries. | Tracks interactive filtering cost as the workspace list grows. |
| `RenderPicker` | Render 1,000 sessions in a 120x40 picker with the preview shown and hidden. | Measures the full view path and the effect of preview layout. |
| `PreviewNavigationBurst` | Start eight preview renders while rapidly moving between sessions. | Exposes stale preview work and provides cancellation metrics for future improvements. |

??? info "Why both an in-memory runner and a subprocess?"

    The in-memory benchmark makes changes to decoding and session construction
    visible without process noise. The process-boundary benchmark represents
    the cost paid by the real plugin. Keep both: improving one layer can
    otherwise hide a regression in the other.

## Read the results

Go reports the standard time and allocation columns. The suite also emits
behavioral metrics that protect the shape of an optimization:

| Metric | Target | Meaning |
| --- | --- | --- |
| `ns/op` | Lower | Nanoseconds per benchmark operation. |
| `B/op` | Lower | Bytes allocated per operation. |
| `allocs/op` | Lower | Allocations per operation. |
| `commands/op` | Lower | Herdr commands executed by one workspace-list operation; treated as an exact value. |
| `canceled/op` | Exactly `7` | The seven superseded preview renders are canceled. |
| `completed/op` | Exactly `1` | The final, active preview render completes. |

!!! warning "Preview cancellation is a target metric"

    `PreviewNavigationBurst` currently reports `0 canceled/op` and
    `8 completed/op` because preview rendering uses a background context. Once
    stale preview cancellation is implemented, the correct result is exactly
    `7 canceled/op` and `1 completed/op`: seven stale renders stop while the
    active selection still updates. Any other pair is a behavioral regression,
    even if one value moved farther in an apparently favorable direction.

## Benchmark strategy

1. Benchmark user-facing hot paths with representative, generated data.
2. Check expected results where the workload has a known answer, and retain
   rendered output so the compiler cannot discard the measured work.
3. Keep isolated and process-boundary measurements separate.
4. Compare repeated samples with `benchstat`; do not judge a change from a
   single run or from results collected on different machines.
5. Review time, allocations, and behavioral metrics together. A lower `ns/op`
   does not justify extra commands or missing the exact `7` canceled / `1`
   completed preview target.

The suite is intentionally a developer tool rather than a CI threshold. Shared
CI runners are noisy, and fixed performance limits would fail for machine
variation instead of meaningful regressions.

[Picker benchmarks](https://github.com/fullerzz/herdr-plugin-sesh/blob/main/internal/picker/tea_benchmark_test.go){ .md-button }
[Workspace-source benchmarks](https://github.com/fullerzz/herdr-plugin-sesh/blob/main/internal/sources/herdr_workspaces_benchmark_test.go){ .md-button }

# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go CLI plugin for Herdr named `herdr-sesh`.

- `cmd/herdr-sesh/` contains the executable entry point.
- `internal/app/` owns CLI routing and command behavior.
- `internal/config/`, `internal/model/`, `internal/sources/`, `internal/state/`, and related packages hold domain logic.
- `docs/` contains user-facing configuration and keybinding notes.
- `testdata/` stores fixture TOML used by tests and smoke checks.
- `herdr-plugin.toml` is the plugin manifest and release version source.
- `bin/` is build output; do not treat generated binaries as source.

## Build, Test, and Development Commands

- `mise install` installs pinned tools from `mise.toml`: Go, `just`, `golangci-lint`, `gotestsum`, `fzf`, and `bat`.
- `just` lists available development recipes from the `justfile`.
- `just fmt-check` checks formatting; `just fmt` applies it.
- `just lint` runs `golangci-lint run ./...`.
- `just test` runs the race-enabled test suite through `gotestsum`.
- `just build` builds the local plugin binary at `bin/herdr-sesh`.
- `just run` runs `./cmd/herdr-sesh` through `go run`.
- `./bin/herdr-sesh --version` smoke-tests the built CLI.
- `./bin/herdr-sesh list --json --config testdata/sesh.toml` checks fixture-backed session listing.

The Forgejo workflow runs formatting, vet, tests, build, and CLI smoke checks; mirror it before opening a pull request. Prefer `just` recipes so local checks use pinned tools.

## Coding Style & Naming Conventions

Use idiomatic Go formatting and short, lowercase package names. Keep command orchestration in `internal/app` and reusable logic in narrow `internal/*` packages. Prefer table tests only when they reduce repetition. Use `xh` instead of `curl` and `uv` when Python is needed.

Tool versions belong in `mise.toml`; update that file when adding or changing shared developer tooling.

## Testing Guidelines

Tests use Go's standard `testing` package and live beside the code as `*_test.go`. Name tests as `Test<Behavior>` and keep file fixtures in `testdata/`. New behavior should include the smallest focused regression test.

## Commit & Pull Request Guidelines

Recent commits use Conventional Commit-style subjects, for example `feat: cache session list when enabled` and `ci: add forgejo test and release workflows`. Keep subjects imperative and scoped when useful (`feat:`, `fix:`, `ci:`, `docs:`).

Pull requests should include a short description, linked issue when applicable, and the exact validation commands run. Include CLI output or screenshots only when changing user-visible command behavior.

## Release & Configuration Notes

Release tags must start with `v` and match `version` in `herdr-plugin.toml`. Configuration lookup order is documented in `README.md`; preserve compatibility with Sesh-style TOML and existing `testdata/sesh.toml` fixtures.

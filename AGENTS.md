# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go CLI plugin for Herdr named `herdr-sesh`.

- `cmd/herdr-sesh/` contains the executable entry point.
- `internal/app/` owns CLI routing and command behavior.
- `internal/config/`, `internal/model/`, `internal/sources/`, `internal/state/`, and related packages hold focused domain logic.
- `docs/` contains user-facing configuration and keybinding notes.
- `testdata/` stores fixture TOML used by tests and smoke checks.
- `herdr-plugin.toml` is the plugin manifest and release version source.
- `bin/` is build output; do not treat generated binaries as source.

## Build, Test, and Development Commands

- `go test ./...` runs the full test suite.
- `go vet ./...` runs Go's standard static checks.
- `gofmt -w .` formats Go files before committing.
- `go build -o bin/herdr-sesh ./cmd/herdr-sesh` builds the local plugin binary.
- `./bin/herdr-sesh --version` smoke-tests the built CLI.
- `./bin/herdr-sesh list --json --config testdata/sesh.toml` checks fixture-backed session listing.

The Forgejo test workflow runs formatting, vet, tests, build, and CLI smoke checks; mirror that sequence before opening a pull request.

## Coding Style & Naming Conventions

Use idiomatic Go formatting and package names: short, lowercase, and purpose-specific. Keep command orchestration in `internal/app` and reusable logic in narrow `internal/*` packages. Prefer table tests only when they reduce repetition; simple single-case tests are fine. Use `xh` instead of `curl`, `uv` when Python is needed, and install developer tools with `mise`.

## Testing Guidelines

Tests use Go's standard `testing` package and live beside the code as `*_test.go`. Name tests as `Test<Behavior>` and keep fixtures in `testdata/` when file input is needed. New behavior should include the smallest focused test that would fail if the behavior regressed.

## Commit & Pull Request Guidelines

Recent commits use Conventional Commit-style subjects, for example `feat: cache session list when enabled` and `ci: add forgejo test and release workflows`. Keep subjects imperative and scoped when useful (`feat:`, `fix:`, `ci:`, `docs:`).

Pull requests should include a short description, linked issue when applicable, and the exact validation commands run. Include CLI output or screenshots only when changing user-visible command behavior.

## Release & Configuration Notes

Release tags must start with `v` and match `version` in `herdr-plugin.toml`. Configuration lookup order is documented in `README.md`; preserve compatibility with Sesh-style TOML and existing `testdata/sesh.toml` fixtures.

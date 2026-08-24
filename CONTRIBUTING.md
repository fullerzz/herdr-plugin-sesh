# Contributing

Thanks for helping improve `herdr-plugin-sesh`.

## Before you start

- Use the issue templates to report bugs or propose features.
- For larger changes, open an issue first so the approach can be discussed.
- Keep changes focused and include tests for new behavior or bug fixes.

You do not need repository access to contribute. Fork the repository, create a
branch in your fork, and open a pull request against `main`.

## Development setup

Install the pinned development tools, then run the checks:

```bash
mise install
just check
just build
```

To build and link your checkout as a local Herdr plugin:

```bash
just install-plugin
```

Run `just` to see the other available development commands.

## Documentation

The published Zensical site is built from the Markdown and assets in `docs/`.
Navigation and theme settings live in `zensical.toml`, while `pyproject.toml` and
`uv.lock` pin the documentation toolchain.

Preview documentation changes locally with:

```bash
just serve-docs
```

Before submitting documentation changes, run the same strict build used by the
GitHub Pages workflow:

```bash
just build-docs
```

The generated `site/` directory is build output and should not be committed.

## Pull requests

Before opening a pull request:

1. Run `just check` and `just build`; also run `just build-docs` when changing
   documentation, Zensical configuration, or the documentation toolchain.
2. Keep commits and the pull request limited to one logical change.
3. Use an imperative, Conventional Commit-style title such as `fix: handle an
   empty session path`.
4. Describe the change, link any related issue, and list the validation commands
   you ran.
5. Update user-facing documentation when behavior or configuration changes.

All pull requests require review before merging. Please address review feedback
with new commits rather than rewriting published history while review is in
progress.

## Code style and tests

- Follow idiomatic Go style and keep reusable logic in the narrowest appropriate
  `internal` package.
- Format code with `just fmt`.
- Place tests beside the code as `*_test.go` files and use fixtures from
  `testdata/` when needed.
- Avoid committing generated binaries from `bin/`.

By contributing, you agree that your contributions will be licensed under the
repository's [MIT License](LICENSE).

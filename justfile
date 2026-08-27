_default:
    @just --list

run:
    go run ./cmd/herdr-sesh

# Build the documentation site
build-docs:
    uv run --frozen zensical build --clean --strict

# Preview the documentation site locally
serve-docs:
    uv run --frozen zensical serve

# Clean build artifacts
clean:
    @echo "{{ BOLD + RED + BG_BLACK }}󰿞 Cleaning build artifacts...{{ NORMAL }}"
    rm ./bin/herdr-sesh || true

# Build the binary with Go's greenteagc garbage collector enabled
build:
    @echo "{{ BOLD + BLUE + BG_BLACK }} Building the project...{{ NORMAL }}"
    mkdir -p bin
    go build -o bin/herdr-sesh ./cmd/herdr-sesh

# Rebuild and relink this checkout as a local Herdr plugin
install-plugin: build
    @echo "{{ BOLD + BLUE + BG_BLACK }} Relinking Herdr plugin...{{ NORMAL }}"
    herdr plugin link "$PWD"

# Run linters on the codebase
lint:
    @echo "{{ BOLD + GREEN + BG_BLACK }} Running linters...{{ NORMAL }}"
    mise exec -- golangci-lint run ./...

# Format the codebase
fmt:
    @echo "{{ BOLD + MAGENTA + BG_BLACK }} Formatting the code...{{ NORMAL }}"
    mise exec -- golangci-lint fmt ./...

# Check formatting without rewriting files
fmt-check:
    @echo "{{ BOLD + MAGENTA + BG_BLACK }} Checking formatting...{{ NORMAL }}"
    mise exec -- golangci-lint fmt --diff ./...

# Run tests
test:
    @echo "{{ BOLD + BLUE + BG_BLACK }} Running tests...{{ NORMAL }}"
    gotestsum --format-icons=octicons --format=pkgname -- -race ./...

# Run the application benchmark suite
bench count='1':
    @echo 'Unit commands/op better=lower assume=exact'
    @echo 'Unit canceled/op assume=exact'
    @echo 'Unit completed/op assume=exact'
    go test -run '^$' -bench=. -benchmem -count={{count}} ./internal/sources ./internal/picker

# Run the benchmark suite under the CodSpeed walltime instrument
bench-codspeed:
    go test -bench=. ./internal/sources ./internal/picker

# Compare two saved benchmark runs
bench-compare base candidate:
    go tool benchstat "base={{base}}" "candidate={{candidate}}"

# Exercise release tag resolution against a same-named branch/tag collision
test-release-ref:
    bash .github/scripts/test-release-ref.sh

# Preview the changelog on stdout, optionally using the next release version
preview-changelog $version='':
    #!/usr/bin/env bash
    set -euo pipefail

    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        echo "GITHUB_TOKEN is required for GitHub changelog metadata" >&2
        exit 1
    fi

    if [[ -n "$version" ]]; then
        mise exec -- git-cliff --tag "$version"
    else
        mise exec -- git-cliff
    fi

# Validate, tag, and trigger the GitHub release workflow
[confirm("Create and push release " + tag + "?")]
release $tag:
    #!/usr/bin/env bash
    set -euo pipefail

    expected_tag="v$(sed -nE 's/^version = "([^"]+)".*/\1/p' herdr-plugin.toml)"
    if [[ "$tag" != "$expected_tag" ]]; then
        echo "Tag mismatch: $tag != manifest $expected_tag" >&2
        exit 1
    fi
    current_branch=$(git branch --show-current)
    if [[ "$current_branch" != "main" ]]; then
        echo "Releases must be created from main, got: ${current_branch:-detached HEAD}" >&2
        exit 1
    fi
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Working tree must be clean before releasing" >&2
        exit 1
    fi
    if git rev-parse --verify --quiet "refs/tags/$tag" >/dev/null; then
        echo "Tag already exists: $tag" >&2
        exit 1
    fi
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        echo "GITHUB_TOKEN is required for GitHub changelog metadata" >&2
        exit 1
    fi

    just check
    just build
    ./bin/herdr-sesh --version
    ./bin/herdr-sesh list --json --config testdata/herdr-sesh.toml >/dev/null
    mise exec -- git-cliff --tag "$tag" --output CHANGELOG.md
    if ! grep -Fq "## ${tag} " CHANGELOG.md; then
        echo "Generated changelog is missing $tag" >&2
        exit 1
    fi
    if git diff --quiet -- CHANGELOG.md; then
        echo "Changelog is already up to date for $tag" >&2
        exit 1
    fi
    git add CHANGELOG.md
    git tag -a "$tag" -m "Release $tag"
    if ! git commit -m "docs(CHANGELOG): update CHANGELOG.md [skip ci]"; then
        git tag -d "$tag"
        git restore --staged --worktree -- CHANGELOG.md
        exit 1
    fi
    git push --atomic origin HEAD "refs/tags/$tag"

# Run all checks for code changes
check: lint fmt-check test test-release-ref
    @echo "{{ BOLD + GREEN + BG_BLACK }} All checks passed!{{ NORMAL }}"

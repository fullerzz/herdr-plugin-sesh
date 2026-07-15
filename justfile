_default:
    @just --list

run:
    go run ./cmd/herdr-sesh

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

# Exercise release tag resolution against a same-named branch/tag collision
test-release-ref:
    bash .github/scripts/test-release-ref.sh

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
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Working tree must be clean before releasing" >&2
        exit 1
    fi

    just check
    just build
    ./bin/herdr-sesh --version
    ./bin/herdr-sesh list --json --config testdata/sesh.toml >/dev/null
    git tag -a "$tag" -m "Release $tag"
    git push origin "refs/tags/$tag"

# Run all checks for code changes
check: lint fmt-check test test-release-ref
    @echo "{{ BOLD + GREEN + BG_BLACK }} All checks passed!{{ NORMAL }}"

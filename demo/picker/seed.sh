#!/bin/bash
# Runs once in a fresh recording clone after the host stages ~/picker-demo.
set -euo pipefail
[[ $(id -un) == lume && $(sysctl -n hw.model) == VirtualMac* ]]
cd "$HOME/picker-demo"
export PATH="$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
swift - <<'SWIFT'
import AppKit
for face in ["FiraCodeNF-Ret", "FiraCodeNF-Bold"] where NSFont(name: face, size: 15)?.fontName != face {
    FileHandle.standardError.write(Data("Missing demo font face: \(face)\n".utf8))
    exit(1)
}
SWIFT
# Refuse to mingle fixture data with an existing scene.
mkdir "$HOME/projects"
mkdir -p ~/.config/{ghostty,herdr} ~/.local/bin
install -m 644 ghostty.conf ~/.config/ghostty/config
install -m 644 herdr.toml ~/.config/herdr/config.toml
cat > ~/.zshrc <<'ZSH'
export PATH="$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PROMPT='%F{blue}%1~%f %F{magenta}›%f '
export COLORTERM=truecolor
ZSH
export GIT_AUTHOR_DATE='2026-06-12T12:00:00Z' GIT_COMMITTER_DATE='2026-06-12T12:00:00Z'
export GIT_AUTHOR_NAME='Demo Maintainer' GIT_AUTHOR_EMAIL='demo@example.invalid'
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME" GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
seed_project() {
    local project=$1 path="$HOME/projects/$1"
    git init --quiet --initial-branch=main "$path"
    mkdir -p "$path"/{src,tests,docs}
    printf '# %s\n' "$project" > "$path/README.md"
    printf 'version = 1\n' > "$path/project.toml"
    printf '# Getting started\n' > "$path/docs/guide.md"
    printf '%s\n' "$project" > "$path/src/name.txt"
    printf 'list\ncreate\nread\nupdate\ndelete\n' > "$path/tests/operations.txt"
    git -C "$path" add .
    git -C "$path" commit --quiet -m 'Add project skeleton'
}
for project in atlas-web atlas-api design-system; do
    seed_project "$project"
done
while IFS= read -r project || [[ -n "$project" ]]; do
    [[ -n "$project" ]] || continue
    seed_project "$project"
    zoxide add "$HOME/projects/$project"
done < zoxide-projects.txt
herdr plugin link "$HOME/picker-demo/plugin" --enabled
open -a /Applications/Ghostty.app

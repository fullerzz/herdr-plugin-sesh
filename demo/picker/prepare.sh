#!/bin/bash
# Boot and seed a fresh clone on Apple Silicon.
set -euo pipefail
vm=${1:?usage: prepare.sh <new-clone-name> [revision]}
revision=$(git rev-parse --verify "${2:-HEAD}^{commit}")
[[ "$vm" == sesh-demo-take-* ]]
[[ $(uname -sm) == 'Darwin arm64' ]]
root=$(git rev-parse --show-toplevel)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/source" "$stage/picker-demo/plugin/bin"
git archive "$revision" | tar -xf - -C "$stage/source"
go_bin=$(mise which go)
(cd "$stage/source" && "$go_bin" build -trimpath -ldflags "-s -w -X github.com/fullerzz/herdr-plugin-sesh/internal/app.Version=${revision:0:7}" -o "$stage/picker-demo/plugin/bin/herdr-sesh" ./cmd/herdr-sesh)
# Ship the prebuilt arm64 executable. All runtime hooks and actions are original.
awk '/^\[\[/{skip=($0=="[[build]]")} !skip' "$stage/source/herdr-plugin.toml" > "$stage/picker-demo/plugin/herdr-plugin.toml"
cp "$root"/demo/picker/{ghostty.conf,herdr.toml,sesh.toml,zoxide-projects.txt,seed.sh,scene.sh,clear-banners.sh,actions.sh,frame.swift,launch.sh,preview.sh,check-routes.sh} "$stage/picker-demo/"
COPYFILE_DISABLE=1 tar -czf "$stage/demo.tar.gz" -C "$stage" picker-demo
lume get "$vm" -f json | jq -e '.[0] | .status == "stopped" and (.sharedDirectories | length == 0)' >/dev/null
# Keep macOS banners at the right edge outside the recorded rectangle.
lume set "$vm" --display 2200x1200
lume run "$vm" --detach --display none --network nat >/dev/null
for ((i=0; i<90; i++)); do
    if ip=$(lume get "$vm" -f json | jq -er '.[0] | select(.sshAvailable and (.sharedDirectories | length == 0)) | .ipAddress'); then break; fi
    sleep 1
done
[[ -n "$ip" && "$ip" != null ]]
"$root/demo/picker/transfer.sh" "$vm" "$stage/demo.tar.gz" picker-demo .
lume ssh "$vm" 'set -e; ~/.local/bin/cua-driver telemetry disable; open -g -a /Applications/CuaDriver.app --args serve; for i in {1..30}; do ~/.local/bin/cua-driver permissions status --json | jq -e ".accessibility and .screen_recording and .daemon_running" >/dev/null && break; sleep 1; done; ~/.local/bin/cua-driver permissions status --json | jq -e ".accessibility and .screen_recording"; swift ~/picker-demo/frame.swift; ~/picker-demo/seed.sh; ~/picker-demo/scene.sh'

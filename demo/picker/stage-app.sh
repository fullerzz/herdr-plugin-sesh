#!/bin/bash
# Stage the matching local release bundles and their resources in a demo clone.
set -euo pipefail
vm=${1:?usage: stage-app.sh <recording-clone> [revision]}
[[ "$vm" == sesh-demo-take-* ]]
revision=$(git rev-parse --verify "${2:-HEAD}^{commit}")
root=$(git rev-parse --show-toplevel)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
app=${HERDR_DEMO_APP:-$HOME/Applications/Herdr.app}
core=${HERDR_DEMO_CORE:-$HOME/.local/share/herdr-core/active/Herdr Core.app}
codesign --verify --deep --strict "$app"
codesign --verify --deep --strict "$core"
mkdir "$stage/desktop"
ditto -c -k --keepParent "$app" "$stage/desktop/app.zip"
ditto -c -k --keepParent "$core" "$stage/desktop/core.zip"
git archive "$revision" herdr-app/mods/sesh | tar -xf - -C "$stage/desktop"
cp "$root/demo/picker/"{app-theme.json,app-settings.json,app-install.sh,app-actions.sh,app-scene.sh} "$stage/desktop/"
cp "${HERDR_DEMO_FONT:-$HOME/Library/Fonts/MonoLisaCodeUpright.ttf}" "$stage/desktop/MonoLisaCodeUpright.ttf"
COPYFILE_DISABLE=1 tar -czf "$stage/files.tgz" -C "$stage" desktop
"$root/demo/picker/transfer.sh" "$vm" "$stage/files.tgz" picker-demo/desktop picker-demo
lume ssh "$vm" 'bash ~/picker-demo/desktop/app-install.sh && bash ~/picker-demo/desktop/app-scene.sh'

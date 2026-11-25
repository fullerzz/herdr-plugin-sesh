#!/bin/bash
set -euo pipefail
[[ $(id -un) == lume && $(sysctl -n hw.model) == VirtualMac* ]]
cd "$HOME/picker-demo/desktop"
mkdir -p ~/Applications ~/.local/share/herdr-core/active ~/.local/bin ~/.config/herdr-app/{mods,themes} ~/.config/herdr-sesh
ditto -x -k app.zip ~/Applications
ditto -x -k core.zip ~/.local/share/herdr-core/active
codesign --verify --deep --strict ~/Applications/Herdr.app
codesign --verify --deep --strict "$HOME/.local/share/herdr-core/active/Herdr Core.app"
ln -sfn "$HOME/.local/share/herdr-core/active/Herdr Core.app" ~/.local/share/herdr-core/current
ln -sfn "$HOME/.local/share/herdr-core/current/Contents/MacOS/herdr-core" ~/.local/bin/hc
ln -sfn "$HOME/Applications/Herdr.app/Contents/MacOS/Herdr" ~/.local/bin/herdr-app
ln -sfn "$HOME/picker-demo/plugin/bin/herdr-sesh" ~/.local/bin/herdr-sesh
ln -sfn "$PWD/herdr-app/mods/sesh" ~/.config/herdr-app/mods/sesh
cp ../sesh.toml ~/.config/herdr-sesh/config.toml
install -m 644 MonoLisaCodeUpright.ttf ~/Library/Fonts/
install -m 644 app-theme.json ~/.config/herdr-app/themes/catppuccin.json
install -m 644 app-settings.json ~/.config/herdr-app/settings.json
# Expand HOME when the guest login shell reads its profile.
# shellcheck disable=SC2016
printf 'export PATH="$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"\n' > ~/.zprofile
export PATH="$HOME/.local/bin:$PATH"
hc --version
herdr-app --version
herdr-app check ~/.config/herdr-app/mods/sesh
open -a "$HOME/Applications/Herdr.app"

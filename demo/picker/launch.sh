#!/bin/sh
set -eu
export PATH="$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export COLORTERM=truecolor
export HERDR_SESH_CONFIG="$HOME/picker-demo/sesh.toml"
cd "$HOME/projects/atlas-web"
exec herdr

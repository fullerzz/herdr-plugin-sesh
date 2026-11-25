#!/bin/bash
set -euo pipefail
cd "$HOME/picker-demo"
export PATH="$HOME/.local/bin:$PATH"
mode=${1:-full}
target=$(cua-driver call list_windows '{"app":"Ghostty","on_screen_only":true}' | jq -ec '.windows[] | select(.layer==0 and (.title | contains("atlas-web"))) | {pid,window_id,session:"Preview"}')
read_pane() { herdr pane read "$picker"; }
expect() {
    for ((i=0; i<40; i++)); do
        if [[ $(read_pane) == *"$1"* ]]; then return; fi
        sleep 0.1
    done
    echo "Picker did not show $1" >&2
    return 1
}
pointer() {
    cua-driver call "$1" "$(jq -c --argjson args "$2" '.+$args+{delivery_mode:"foreground"}' <<<"$target")" >/dev/null
}
sleep 0.7
herdr plugin action invoke fullerzz.sesh.open-picker >/dev/null
for ((i=0; i<30; i++)); do
    if picker=$(herdr pane list | jq -er '.result.panes[]|select(.cwd=="/Users/lume/picker-demo/plugin")|.pane_id'); then break; fi
    sleep 0.1
done
expect '5 workspaces'
expect '└─ route-cache'
if [[ "$mode" == full ]]; then sleep 2; fi
for letter in a t l a s; do herdr pane send-text "$picker" "$letter"; sleep 0.12; done
expect '2/5 workspaces'
herdr pane send-keys "$picker" ctrl+j
herdr pane send-keys "$picker" ctrl+j
expect 'PREVIEW [ctrl+o] · atlas-api'
if [[ "$mode" == full ]]; then sleep 1.1; fi
herdr pane send-keys "$picker" ctrl+o
expect 'Route checks'
if [[ "$mode" == full ]]; then sleep 1.4; fi
# These window-local pixels are calibrated to the fixed font, frame and sidebar.
cua-driver call get_window_state "$(jq -c '.+{max_image_dimension:0,screenshot_out_file:"/tmp/sesh-picker-before.png"}' <<<"$target")" >/dev/null
pointer scroll '{"direction":"down","by":"line","amount":6,"x":1130,"y":389}'
for ((i=0; i<20; i++)); do
    view=$(read_pane)
    if [[ "$view" != *'Route checks'* && "$view" == *'PANE [ctrl+o] · atlas-api'* ]]; then break; fi
    sleep 0.1
done
[[ "$view" != *'Route checks'* && "$view" == *'PANE [ctrl+o] · atlas-api'* ]]
if [[ "$mode" == full ]]; then sleep 1.2; fi
pointer drag '{"from_x":854,"from_y":458,"to_x":710,"to_y":458,"duration_ms":800,"steps":24}'
expect 'response matches contract'
if [[ "$mode" == full ]]; then sleep 2; fi
herdr pane send-keys "$picker" enter
for ((i=0; i<20; i++)); do
    focused=$(herdr workspace list | jq -er '.result.workspaces[]|select(.focused)|.label')
    [[ "$focused" == atlas-api ]] && break
    sleep 0.1
done
[[ "$focused" == atlas-api ]]
sleep 1.8

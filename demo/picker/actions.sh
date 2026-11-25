#!/bin/bash
set -euo pipefail
cd "$HOME/picker-demo"
export PATH="$HOME/.local/bin:$PATH"
mode=${1:-full}
target=$(cua-driver call list_windows '{"app":"Ghostty","on_screen_only":true}' | jq -ec '.windows[] | select(.layer==0 and (.title | contains("atlas-web"))) | {pid,window_id,session:"Preview"}')
read_pane() { herdr pane read "$picker" --source visible --format text; }
expect() {
    for ((i=0; i<40; i++)); do
        if [[ $(read_pane) == *"$1"* ]]; then return; fi
        sleep 0.1
    done
    echo "Picker did not show $1" >&2
    read_pane >&2
    return 1
}
pointer() {
    cua-driver call "$1" "$(jq -c --argjson args "$2" '.+$args+{delivery_mode:"foreground"}' <<<"$target")" >/dev/null
}
total=$(jq length catalog.json)
cua-driver call set_agent_cursor_enabled "$(jq -c '{session,enabled:false}' <<<"$target")" >/dev/null
sleep 0.7
herdr plugin action invoke fullerzz.sesh.open-picker >/dev/null
for ((i=0; i<30; i++)); do
    if picker=$(herdr pane list | jq -er '.result.panes[]|select(.cwd=="/Users/lume/picker-demo/plugin")|.pane_id'); then break; fi
    sleep 0.1
done
expect "$total workspaces"
expect '└─ route-cache'
if [[ "$mode" == full ]]; then sleep 2; fi
for letter in a t l a s; do herdr pane send-text "$picker" "$letter"; sleep 0.12; done
expect " 2/$total workspaces"
herdr pane send-keys "$picker" ctrl+j
herdr pane send-keys "$picker" ctrl+j
expect 'PREVIEW [ctrl+o] · atlas-api'
if [[ "$mode" == full ]]; then sleep 1.1; fi
herdr pane send-keys "$picker" ctrl+o
expect 'Route checks'
if [[ "$mode" == full ]]; then sleep 1.4; fi
# These window-local pixels are calibrated to the fixed font, frame and sidebar.
cua-driver call get_window_state "$(jq -c '.+{max_image_dimension:0,screenshot_out_file:"/tmp/sesh-picker-before.png"}' <<<"$target")" >/dev/null
before=$(read_pane)
[[ "$before" == *'Route checks'* && "$before" != *'30 checks passed'* ]]
pointer drag '{"from_x":1374,"from_y":228,"to_x":1374,"to_y":676,"duration_ms":850,"steps":32}'
expect '30 checks passed'
if [[ "$mode" == full ]]; then sleep 1.8; fi
after=$(read_pane)
[[ "$after" != *'Route checks'* && "$after" == *'30 checks passed'* ]]
pointer drag '{"from_x":854,"from_y":458,"to_x":710,"to_y":458,"duration_ms":800,"steps":24}'
expect 'response matches contract'
if [[ "$mode" == full ]]; then sleep 2; fi
herdr pane send-keys "$picker" enter
for ((i=0; i<20; i++)); do
    focused=$(herdr workspace list | jq -er '.result.workspaces[]|select(.focused)|.label') || focused=
    [[ "$focused" == atlas-api ]] && break
    sleep 0.1
done
[[ "$focused" == atlas-api ]]
sleep 1.8

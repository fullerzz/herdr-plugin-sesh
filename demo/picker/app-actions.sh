#!/bin/bash
set -euo pipefail
export PATH="$HOME/.local/bin:$PATH"
cd "$HOME/picker-demo"
target=$(cua-driver call list_windows '{"app":"Herdr","on_screen_only":true}' | jq -ec '.windows[]|select(.layer==0 and .title=="Herdr")|{pid,window_id,session:"Preview"}')
expect() {
    for ((i=0; i<30; i++)); do
        if [[ $(hc read "$picker") == *"$1"* ]]; then return; fi
        sleep 0.1
    done
    echo "Desktop picker did not show $1" >&2
    return 1
}
pointer() {
    cua-driver call "$1" "$(jq -c --argjson args "$2" '.+$args+{delivery_mode:"foreground"}' <<<"$target")" >/dev/null
}
sleep 0.7
herdr-app "do" mod.open '{"name":"sesh"}' >/dev/null
for ((i=0; i<30; i++)); do
    if picker=$(hc ls --label sesh=picker --json | jq -er '.terminals[]|select(.state=="live")|.address'); then break; fi
    sleep 0.1
done
expect '5 workspaces'
expect 'route-cache'
sleep 2
for letter in a t l a s; do hc keys "$picker" "$letter" >/dev/null; sleep 0.12; done
expect '2/5 workspaces'
hc keys "$picker" ctrl+j ctrl+j >/dev/null
expect 'PREVIEW [ctrl+o] · atlas-api'
sleep 1
hc keys "$picker" ctrl+o >/dev/null
expect 'PANE [ctrl+o] · atlas-api'
expect 'response matches'
sleep 1.4
cua-driver call get_window_state "$(jq -c '.+{max_image_dimension:0,screenshot_out_file:"/tmp/app-before-scroll.png"}' <<<"$target")" >/dev/null
for ((i=0; i<3; i++)); do
    pointer scroll '{"direction":"down","by":"line","amount":8,"x":1030,"y":390}'
    sleep 0.3
    view=$(hc read "$picker")
    [[ "$view" != *"list     projects"* && "$view" == *'PANE [ctrl+o] · atlas-api'* ]] && break
done
[[ "$view" != *"list     projects"* && "$view" == *'PANE [ctrl+o] · atlas-api'* ]]
sleep 1
pointer drag '{"from_x":717,"from_y":441,"to_x":587,"to_y":441,"duration_ms":800,"steps":24}'
expect 'response matches contract'
sleep 1.6
hc keys "$picker" enter >/dev/null
for ((i=0; i<30; i++)); do
    focused=$(herdr-app ls | jq -er '.spaces[]|select(.active)|.name')
    [[ "$focused" == atlas-api ]] && break
    sleep 0.1
done
[[ "$focused" == atlas-api ]]
hc ls --label sesh=picker --json | jq -e 'all(.terminals[]; .state!="live")' >/dev/null
sleep 1.8

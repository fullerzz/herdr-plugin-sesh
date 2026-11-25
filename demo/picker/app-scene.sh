#!/bin/bash
# Prepare real desktop spaces in the same synthetic repositories as the TUI.
set -euo pipefail
export PATH="$HOME/.local/bin:$PATH"
cd "$HOME/picker-demo"
for ((i=0; i<40; i++)); do
    if window=$(cua-driver call list_windows '{"app":"Herdr","on_screen_only":true}' | jq -ec '.windows[]|select(.layer==0 and .title=="Herdr")|{pid,window_id}'); then break; fi
    sleep 0.5
done
cua-driver call set_window_frame "$(jq -c '.+{x:85,y:64,width:1420,height:840}' <<<"$window")" | jq -e '.effect=="confirmed"' >/dev/null
./clear-banners.sh
cua-driver call bring_to_front "$window" | jq -e '.exact_window_effect|.verified and .unobstructed' >/dev/null
if herdr-app status | jq -e '.overlays|index("frame-welcome")!=null' >/dev/null; then
    # Close the first-run welcome panel at its X in the fixed-size guest window.
    cua-driver call get_window_state "$(jq -c '.+{session:"setup",max_image_dimension:0,screenshot_out_file:"/tmp/app-welcome.png"}' <<<"$window")" >/dev/null
    cua-driver call click "$(jq -c '.+{session:"setup",x:1015,y:147,delivery_mode:"foreground"}' <<<"$window")" >/dev/null
    cua-driver call set_agent_cursor_enabled '{"session":"setup","enabled":false}' >/dev/null
fi
for project in atlas-web atlas-api route-cache; do
    if ! herdr-app ls | jq -e --arg p "$HOME/projects/$project" '.spaces[]|select(.folder==$p)' >/dev/null; then
        herdr-app "do" space.new --name "$project" --folder "$HOME/projects/$project" >/dev/null
    fi
done
spaces=$(herdr-app ls)
terminal() { jq -er --arg n "$1" '.spaces[]|select(.name==$n)|.tabs[0].terminals[0].terminal' <<<"$spaces"; }
web=$(jq -er '.spaces[]|select(.name=="atlas-web")|.id' <<<"$spaces")
hc send "$(terminal atlas-api)" /Users/lume/picker-demo/check-routes.sh >/dev/null
hc send "$(terminal atlas-web)" 'clear && git status --short --branch' >/dev/null
herdr-app "do" space.select --id "$web" >/dev/null
herdr-app ls | jq -e '.spaces[]|select(.active)|.name=="atlas-web"' >/dev/null
herdr-app status | jq -e '.overlays|length==0' >/dev/null
HERDR_SESH_BACKEND=app herdr-sesh list --json > app-catalog.json
jq -s -e 'map(map([.name,.source,.path])|sort) | .[0]==.[1]' catalog.json app-catalog.json >/dev/null

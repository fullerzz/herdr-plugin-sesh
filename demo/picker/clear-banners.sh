#!/bin/bash
# Resolve first-login UI only on the disposable, synthetic guest.
set -eu
[[ $(id -un) == lume && $(sysctl -n hw.model) == VirtualMac* ]]
export PATH="$HOME/.local/bin:$PATH"
cua-driver call set_agent_cursor_enabled '{"session":"demo-prep","enabled":false}' >/dev/null

dismiss() {
    local process=$1 filter=$2 expected=$3 control=$4 operation=$5 pid id args state row token
    pid=$(pgrep -ix "$process" | head -1)
    [[ -n "$pid" ]] || return 0
    for id in $(cua-driver call list_windows "{\"pid\":$pid}" | jq -r ".windows[] | select(.is_on_screen and ($filter)) | .window_id"); do
        args=$(jq -nc --argjson p "$pid" --argjson w "$id" '{pid:$p,window_id:$w,session:"demo-prep"}')
        state=$(cua-driver call get_window_state "$(jq -c '.+{include_screenshot:false}' <<<"$args")") || continue
        [[ $(jq -r '.tree_markdown' <<<"$state") == *"$expected"* ]] || continue
        row=$(jq -r '.tree_markdown' <<<"$state" | awk -v text="$control" 'index($0,text) {if (match($0,/\[[0-9]+\]/)) {print substr($0,RSTART+1,RLENGTH-2); exit}}')
        [[ -n "$row" ]] || continue
        token=$(jq -r '.snapshot_id' <<<"$state"):$row
        cua-driver call click "$(jq -c --arg t "$token" --arg action "$operation" '.+{element_token:$t,action:$action}' <<<"$args")" >/dev/null
    done
}
dismiss Ghostty '.title==""' 'Quit Ghostty?' 'AXButton "Cancel"' press
dismiss loginwindow '.layer>0' 'logged out' 'AXButton "Cancel"' press
dismiss loginwindow '.layer>0' 'interrupted logout' 'AXButton "Cancel"' press
dismiss NotificationCenter '.title=="Notification Center"' 'Dock Tile Extension Added' 'AXGroup (Login Items, Dock Tile Extension Added' Close
dismiss NotificationCenter '.title=="Notification Center"' 'Notifications may include alerts' 'Notifications, Notifications may include alerts' Close

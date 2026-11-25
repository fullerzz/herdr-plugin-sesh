#!/bin/bash
set -euo pipefail
trap 'echo "TUI scene failed at line $LINENO: $BASH_COMMAND" >&2' ERR
export PATH="$HOME/.local/bin:$PATH"
cd "$HOME/picker-demo"
export HERDR_SESH_CONFIG="$PWD/sesh.toml"
for ((i=0; i<30; i++)); do
    if window=$(cua-driver call list_windows '{"app":"Ghostty","on_screen_only":true}' | jq -ec '.windows[] | select(.layer==0 and (.title | contains("atlas-web"))) | {pid,window_id}'); then break; fi
    sleep 1
done
[[ -n "$window" ]]
cua-driver call set_window_frame "$(jq -c '.+{x:85,y:64,width:1420,height:840}' <<<"$window")" | jq -e '.effect=="confirmed"' >/dev/null
# A newly cloned Mac can repeat the optional iCloud invitation after first login.
# Dismiss only that invitation, after confirming the base completed setup.
buddy=$(pgrep -x 'Setup Assistant' || true)
if [[ -n "$buddy" && $(defaults read com.apple.SetupAssistant DidSeeCloudSetup) == 1 ]]; then
    if win=$(cua-driver call list_windows "{\"pid\":$buddy}" | jq -ec '[.windows[]|select(.layer==0 and .is_on_screen)][0] | select(.) | {pid,window_id,include_screenshot:false}'); then
        tree=$(cua-driver call get_window_state "$win" | jq -r '.tree_markdown')
        if [[ "$tree" == *'id=iCloudLogin '* ]]; then kill -TERM "$buddy"; fi
    fi
fi
for ((i=0; i<12; i++)); do
    ./clear-banners.sh
    if cua-driver call bring_to_front "$window" | jq -e '.exact_window_effect | .verified and .unobstructed' >/dev/null; then break; fi
    sleep 1
done
cua-driver call bring_to_front "$window" | jq -e '.exact_window_effect.unobstructed' >/dev/null
./clear-banners.sh
web=$(herdr workspace list | jq -er '.result.workspaces[] | select(.label=="atlas-web") | .workspace_id')
api=$(herdr workspace create --cwd "$HOME/projects/atlas-api" --label atlas-api --no-focus)
api_id=$(jq -er '.result.workspace.workspace_id' <<<"$api")
api_pane=$(jq -er '.result.root_pane.pane_id' <<<"$api")
herdr worktree create --workspace "$api_id" --base main --branch route-cache --path "$HOME/projects/route-cache" --label route-cache --no-focus --trust-repository | jq -e '.result.workspace.worktree.is_linked_worktree' >/dev/null
herdr pane run "$api_pane" /Users/lume/picker-demo/check-routes.sh
herdr workspace focus "$api_id" >/dev/null
herdr workspace focus "$web" >/dev/null
web_pane=$(herdr pane list --workspace "$web" | jq -er '.result.panes[0].pane_id')
herdr pane run "$web_pane" 'git status --short --branch'
./plugin/bin/herdr-sesh list --json > catalog.json
jq -e --rawfile projects zoxide-projects.txt '
  ($projects | split("\n") | map(select(length > 0) | [., "zoxide", ("/Users/lume/projects/" + .)])) as $zoxide |
  map([.name,.source,.path]) | sort == ([
  ["atlas-web","herdr","/Users/lume/projects/atlas-web"],
  ["atlas-api","herdr","/Users/lume/projects/atlas-api"],
  ["route-cache","herdr","/Users/lume/projects/route-cache"],
  ["design-system","config","/Users/lume/projects/design-system"]
] + $zoxide | sort)' catalog.json >/dev/null

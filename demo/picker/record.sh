#!/bin/bash
set -euo pipefail
surface=${1:-both}
case "$surface" in both|tui|app) ;; *) echo 'Choose both, tui, or app' >&2; exit 2 ;; esac
revision=$(git rev-parse --verify "${2:-HEAD}^{commit}")
root=$(git rev-parse --show-toplevel)
cd "$root"
vm="sesh-demo-take-$(date +%Y%m%d-%H%M%S)-$$"
out="scratch/picker-demo/$vm"
lume get sesh-demo-base -f json | jq -e '.[0] | .status=="stopped" and (.sharedDirectories|length==0)' >/dev/null
lume clone sesh-demo-base "$vm"
# The clone belongs to this run. Keep its disk available for inspection.
stop_clone() {
    if lume get "$vm" -f json | jq -e '.[0].status=="running"' >/dev/null; then
        lume shutdown "$vm"
    fi
}
trap stop_clone EXIT
demo/picker/prepare.sh "$vm" "$revision"
capture() {
    uv run --script demo/picker/capture.py "$vm" "$out/$1" --surface "$1"
    demo/picker/export.sh "$out/$1"
    echo "Review $out/$1/picker-demo.gif and $out/$1/contact-sheet.png"
}
if [[ "$surface" != app ]]; then capture tui; fi
if [[ "$surface" != tui ]]; then
    demo/picker/stage-app.sh "$vm" "$revision"
    capture app
fi

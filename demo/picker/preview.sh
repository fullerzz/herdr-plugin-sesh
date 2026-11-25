#!/bin/sh
set -eu
cd "$1"
printf '\033[38;2;203;166;247m%s\033[0m\n\n' "${PWD##*/}"
for entry in *; do
    if test -d "$entry"; then
        printf '\033[38;2;137;180;250m󰉋  %s/\033[0m\n' "$entry"
    else
        printf '\033[38;2;205;214;244m󰈙  %s\033[0m\n' "$entry"
    fi
done

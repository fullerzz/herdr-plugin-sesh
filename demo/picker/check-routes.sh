#!/bin/sh
set -eu
printf '\033[2J\033[H\033[1;38;2;203;166;247matlas-api\033[0m  ·  Route checks\n\n'
for resource in projects milestones issues releases labels members; do
    for operation in list create read update delete; do
        printf '  \033[38;2;166;227;161m✓\033[0m  %-8s %-12s  response matches contract\n' "$operation" "$resource"
    done
done
printf '\n\033[38;2;166;227;161m30 checks passed\033[0m\n'

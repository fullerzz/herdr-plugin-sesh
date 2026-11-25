#!/bin/bash
# Transfer one archive using a fresh key and a private per-run host-key file.
set -euo pipefail
vm=$1 archive=$2 dest=$3 parent=$4
[[ "$vm" == sesh-demo-take-* ]]
ip=$(lume get "$vm" -f json | jq -er '.[0] | select(.sshAvailable and (.sharedDirectories | length == 0)) | .ipAddress')
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
ssh-keygen -q -t ed25519 -N '' -C picker-stage -f "$stage/key"
public_key=$(cat "$stage/key.pub")
opts=(-i "$stage/key" -o IdentitiesOnly=yes -o ForwardAgent=no -o BatchMode=yes -o "UserKnownHostsFile=$stage/known_hosts" -o StrictHostKeyChecking=accept-new)
lume ssh "$vm" "umask 077; mkdir -p ~/.ssh; printf '%s\n' '$public_key' >> ~/.ssh/authorized_keys"
cleanup() {
    local status=$?
    if ! lume ssh "$vm" "set -e; cd ~/.ssh; tmp=\$(mktemp keys.XXXXXX); awk -v key='$public_key' '\$0 != key' authorized_keys > \"\$tmp\"; chmod 600 \"\$tmp\"; mv \"\$tmp\" authorized_keys"; then
        echo "Could not remove the temporary public key from $vm" >&2
        status=1
    fi
    rm -rf "$stage"
    exit "$status"
}
trap cleanup EXIT
# The two relative archive paths are chosen by the staging scripts.
# shellcheck disable=SC2029
ssh "${opts[@]}" "lume@$ip" "set -e; cd; test ! -e '$dest'; cd '$parent'; tar -xzf -" < "$archive"

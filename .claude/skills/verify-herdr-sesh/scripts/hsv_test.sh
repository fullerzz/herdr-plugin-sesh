#!/usr/bin/env bash
# Regressions: `hsv.sh down` must never signal a recorded PID it cannot prove it started,
# `x` must refuse a run it cannot validate, and a failed `up` must tear itself down.
# Uses decoy processes and a stub herdr; touches no real Herdr.
set -euo pipefail

hsv="$(cd "$(dirname "$0")" && pwd)/hsv.sh"
pids=()
tmp=$(mktemp -d)
trap 'kill "${pids[@]}" 2>/dev/null || true; rm -rf "${run:-}" "$tmp"' EXIT

# Fake run whose server.pid is a live decoy with a child; $1 = recorded start time.
fake_run() {
	local run decoy child
	run=$(mktemp -d /tmp/hsv.XXXXXX)
	mkdir -p "$run/bin"
	printf '#!/bin/sh\nexit 1\n' >"$run/bin/herdr"
	chmod +x "$run/bin/herdr"
	cat >"$run/env.sh" <<EOF
export HOME='$run' XDG_CONFIG_HOME='$run' XDG_STATE_HOME='$run'
export HERDR_SOCKET_PATH='$run/none.sock' HSV_TMUX='hsv-test-$$' HSV_EVIDENCE='$run/evidence'
export PATH='$run/bin':"\$PATH"
EOF
	bash -c 'exec -a "herdr server" bash -c "sleep 300 & wait"' </dev/null >/dev/null 2>&1 &
	decoy=$!
	for _ in $(seq 20); do
		child=$(pgrep -P "$decoy" || true)
		[[ -n $child ]] && break
		sleep 0.1
	done
	[[ -n $child ]] || { echo "FAIL decoy child never started" >&2; exit 1; }
	echo "$decoy" >"$run/server.pid"
	if [[ $1 == real ]]; then ps -o lstart= -p "$decoy" >"$run/server.start"; else echo "$1" >"$run/server.start"; fi
	echo "$run $decoy $child"
}

alive() { kill -0 "$1" 2>/dev/null; }

# Reused PID: recorded start time belongs to a process that no longer exists.
read -r run decoy child < <(fake_run "Thu Jan  1 00:00:00 1970")
pids+=("$decoy" "$child")
"$hsv" down "$run" >/dev/null 2>&1
if ! alive "$decoy" || ! alive "$child"; then echo "FAIL down signaled a process it did not start" >&2; exit 1; fi
echo "ok   reused pid: decoy server and child left running"

# Owned PID: start time matches, so down stops the server and its children.
read -r run decoy child < <(fake_run real)
pids+=("$decoy" "$child")
"$hsv" down "$run" >/dev/null 2>&1
sleep 0.3
if alive "$decoy" || alive "$child"; then echo "FAIL down left its own server or child running" >&2; exit 1; fi
echo "ok   owned pid: server and child stopped"

# x on a torn-down, missing, or foreign RUN dies before running anything.
for bad in "$run" /tmp/hsv.missing "$tmp"; do
	if "$hsv" x "$bad" touch "$tmp/marker" 2>/dev/null || [[ -e $tmp/marker ]]; then
		echo "FAIL x ran a command for invalid RUN $bad" >&2
		exit 1
	fi
done
echo "ok   x refuses torn-down, missing, and foreign RUN"

# up failing after the server starts (plugin link fails) stops the server and removes RUN.
mkdir -p "$tmp/bin" "$tmp/repo"
cat >"$tmp/bin/herdr" <<EOF
#!/bin/sh
case "\$1 \$2" in
'server stop') exit 1 ;;
server*) echo \$\$ >'$tmp/server.pid'; exec sleep 300 ;;
workspace*) exit 0 ;;
*) exit 1 ;;
esac
EOF
printf '#!/bin/sh\nexit 0\n' >"$tmp/bin/go"
chmod +x "$tmp/bin/herdr" "$tmp/bin/go"
git -C "$tmp/repo" init -q
touch "$tmp/repo/herdr-plugin.toml"
if PATH="$tmp/bin:$PATH" "$hsv" up "$tmp/repo" >"$tmp/up.out" 2>"$tmp/up.err"; then
	echo "FAIL up succeeded with a failing plugin link" >&2
	exit 1
fi
failed=$(sed -n 's/^hsv: RUN=//p' "$tmp/up.err")
[[ -n $failed ]] || { echo "FAIL up did not print RUN on stderr" >&2; exit 1; }
[[ ! -s $tmp/up.out ]] || { echo "FAIL up printed to stdout on failure" >&2; exit 1; }
[[ ! -e $failed ]] || { echo "FAIL up left $failed behind" >&2; exit 1; }
server=$(cat "$tmp/server.pid")
pids+=("$server")
if alive "$server"; then echo "FAIL up left its server running" >&2; exit 1; fi
echo "ok   failed up stops its server and removes RUN"

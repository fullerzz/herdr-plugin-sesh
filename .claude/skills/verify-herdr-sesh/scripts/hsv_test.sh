#!/usr/bin/env bash
# Regressions: `hsv.sh down` must never signal a recorded PID it cannot prove it started,
# `x` must refuse a run it cannot validate, and a failed `up` must tear itself down.
# Uses decoy processes and a stub herdr; touches no real Herdr.
set -euo pipefail

# Fail before spawning fixtures when process inspection is sandboxed.
ps -o lstart= -p "$$" >/dev/null

hsv="$(cd "$(dirname "$0")" && pwd)/hsv.sh"
pids=()
tmp=$(mktemp -d)
tmp=$(cd "$tmp" && pwd -P)
trap 'kill -KILL "${pids[@]}" 2>/dev/null || true; rm -rf "${run:-}" "$tmp"' EXIT

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
	cat >"$run/child.sh" <<'EOF'
trap '[[ -f $1/release ]] && exit 0' TERM
touch "$1/ready"
while :; do sleep 0.1; done
EOF
	bash -c '
		case "$1" in
		delayed) trap "sleep 0.75; exit" TERM ;;
		resistant) trap "" TERM ;;
		esac
		if [[ $1 == child-resistant ]]; then
			bash "$2/child.sh" "$2" &
		else
			sleep 300 &
		fi
		wait
	' _ "${2:-normal}" "$run" </dev/null >/dev/null 2>&1 &
	decoy=$!
	for _ in $(seq 20); do
		child=$(pgrep -P "$decoy" || true)
		[[ -n $child ]] && break
		sleep 0.1
	done
	[[ -n $child ]] || { echo "FAIL decoy child never started" >&2; exit 1; }
	if [[ ${2:-} == child-resistant ]]; then
		for _ in $(seq 20); do [[ -f $run/ready ]] && break; sleep 0.1; done
		[[ -f $run/ready ]] || { echo "FAIL resistant child never ready" >&2; exit 1; }
	fi
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
if alive "$decoy" || alive "$child"; then echo "FAIL down left its own server or child running" >&2; exit 1; fi
echo "ok   owned pid: server and child stopped"

# SIGTERM can take time; successful down must mean the process has exited.
read -r run decoy child < <(fake_run real delayed)
pids+=("$decoy" "$child")
"$hsv" down "$run" >/dev/null 2>&1
if alive "$decoy" || alive "$child"; then echo "FAIL down returned before delayed exit" >&2; exit 1; fi
echo "ok   delayed shutdown: down waits for exit"

# A denied signal probe must not be mistaken for an exited owned process.
read -r run decoy child < <(fake_run real)
pids+=("$decoy" "$child")
printf 'kill() { return 1; }\n' >"$tmp/deny-kill.sh"
if BASH_ENV="$tmp/deny-kill.sh" "$hsv" down "$run" >/dev/null 2>&1; then
	echo "FAIL down accepted signal denial as process exit" >&2; exit 1
fi
[[ -f $run/server.start && -s $run/children ]] || { echo "FAIL signal denial discarded records" >&2; exit 1; }
if ! alive "$decoy" || ! alive "$child"; then echo "FAIL denied signals stopped a fixture" >&2; exit 1; fi
"$hsv" down "$run" >/dev/null 2>&1
echo "ok   denied signals: keeps records for a cleanup retry"

# Keep identities for a retry, including children orphaned after the server exits.
for mode in resistant child-resistant; do
	read -r run decoy child < <(fake_run real "$mode")
	pids+=("$decoy" "$child")
	if "$hsv" down "$run" >"$tmp/down.out" 2>"$tmp/down.err"; then
		echo "FAIL down succeeded with a $mode process" >&2; exit 1
	fi
	[[ -f $run/server.start && -s $run/children ]] || { echo "FAIL down discarded ownership records" >&2; exit 1; }
	alive "$child" || { echo "FAIL resistant child unexpectedly exited" >&2; exit 1; }
	if [[ $mode == resistant ]]; then
		alive "$decoy" || { echo "FAIL resistant server unexpectedly exited" >&2; exit 1; }
		kill -KILL "$decoy"
	else
		if alive "$decoy"; then echo "FAIL normal server did not exit" >&2; exit 1; fi
	fi
	if [[ $mode == child-resistant ]]; then
		# A retry must stop the orphan using its saved identity.
		touch "$run/release"
	else
		kill -KILL "$child"
		for _ in $(seq 20); do alive "$child" || break; sleep 0.1; done
	fi
	"$hsv" down "$run" >/dev/null 2>&1
	if alive "$child"; then echo "FAIL cleanup retry left an orphaned child running" >&2; exit 1; fi
	[[ ! -e $run ]] || { echo "FAIL cleanup retry retained an exited run" >&2; exit 1; }
	echo "ok   $mode: preserves records until cleanup retry"
done

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

# Every emitted path must survive shell parsing, including quotes and command syntax.
quoted_repo="$tmp/O'Brien;\$(touch HSV_INJECTED);#"
quoted_bin="$tmp/O'Brien bin"
mkdir -p "$quoted_repo/testdata" "$quoted_bin"
git -C "$quoted_repo" init -q
touch "$quoted_repo/herdr-plugin.toml" "$quoted_repo/testdata/herdr-sesh.toml"
cat >"$quoted_bin/herdr" <<'EOF'
#!/bin/sh
case "$1 $2" in
'server stop') exit 1 ;;
server*) echo $$ >"$HSV_STUB_SERVER"; exec sleep 300 ;;
workspace*|'plugin link') exit 0 ;;
'plugin config-dir') printf "%s/config O'Brien;\$(touch HSV_INJECTED);#\n" "$HOME" ;;
*) exit 1 ;;
esac
EOF
printf '#!/bin/sh\nexit 0\n' >"$quoted_bin/go"
cp "$quoted_bin/go" "$quoted_bin/zoxide"
chmod +x "$quoted_bin/herdr" "$quoted_bin/go" "$quoted_bin/zoxide"
run=$(cd "$tmp" && HSV_STUB_SERVER="$tmp/quoted-server.pid" PATH="$quoted_bin:$PATH" "$hsv" up "$quoted_repo")
pids+=("$(cat "$tmp/quoted-server.pid")")
cat >"$tmp/assert-env.sh" <<'EOF'
	[[ $HSV_TARGET == "$1" && $HSV_EVIDENCE == "$1/scratch/verify-herdr-sesh/${HSV_RUN##*/hsv.}" ]]
	[[ $HERDR_BIN_PATH == "$2/herdr" && $HERDR_PLUGIN_CONFIG_DIR == "$HOME/config O'Brien;\$(touch HSV_INJECTED);#" ]]
	[[ $PATH == "$HSV_RUN/plugin/bin:"* && -f $HERDR_PLUGIN_CONFIG_DIR/config.toml ]]
EOF
"$hsv" x "$run" bash -e "$tmp/assert-env.sh" "$quoted_repo" "$quoted_bin"
[[ ! -e $tmp/HSV_INJECTED ]] || { echo "FAIL sourcing env executed path contents" >&2; exit 1; }
"$hsv" down "$run" >/dev/null 2>&1
[[ ! -e $run ]] || { echo "FAIL quoted checkout cleanup failed" >&2; exit 1; }
echo "ok   quoted paths: up, x, and down preserve literal values"

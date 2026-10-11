#!/usr/bin/env bash
# hsv: isolated Herdr + herdr-sesh harness for the verify-herdr-sesh skill.
# Every run owns a throwaway HOME/XDG tree, Herdr server, and tmux socket.
set -euo pipefail

die() { echo "hsv: $*" >&2; exit 1; }

usage() {
	cat <<'EOF'
usage: hsv.sh up TARGET_REPO             build into RUN, start isolated Herdr, link plugin; prints RUN
       hsv.sh rebuild RUN                rebuild RUN's plugin binary from the checkout
       hsv.sh doctor RUN                 read-only check that RUN is ours and healthy
       hsv.sh x RUN CMD...               exec CMD in RUN's isolated env (the only way to run herdr)
       hsv.sh overlay RUN LABEL [SEC]    wait until the focused pane is LABEL; print its pane ID
       hsv.sh cli RUN LABEL CMD...       run CMD in RUN env; save LABEL.{cmd,out,err,exit}
       hsv.sh tui RUN NAME CMD...        start CMD in isolated tmux session NAME (120x32)
       hsv.sh keys RUN NAME KEYS...      tmux send-keys to NAME (-l KEYS for literal text)
       hsv.sh wait RUN NAME TEXT [SEC]   wait until NAME's screen contains TEXT (default 10s)
       hsv.sh snap RUN NAME LABEL        save NAME's screen to evidence as LABEL.txt
       hsv.sh down RUN                   stop what RUN started, delete RUN; keep evidence
Pipelines: hsv.sh x "$RUN" sh -c 'herdr workspace list | jq .'
EOF
	exit 2
}

load() {
	run=${1:-}
	[[ $run == /tmp/hsv.* && -f $run/env.sh && -f $run/server.pid ]] || die "not an hsv run dir: $run"
	# shellcheck disable=SC1091
	. "$run/env.sh"
}

# PID + start time identify a process; a PID alone can be reused by the user's real Herdr.
started() { ps -o lstart= -p "$1" 2>/dev/null || true; }

# True only if the recorded server PID is still the process this run launched.
owned() {
	local pid start
	pid=$(cat "$run/server.pid")
	start=$(cat "$run/server.start" 2>/dev/null) || return 1
	[[ -n $start && $(started "$pid") == "$start" ]]
}

# Builds with the caller's real env (Go caches live under the real HOME); never writes TARGET/bin,
# which the user's real Herdr may be running.
build() {
	(cd "$1" && go build -ldflags "-X github.com/fullerzz/herdr-plugin-sesh/internal/app.Version=$(git describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || printf dev)" \
		-o "$2/plugin/bin/herdr-sesh" ./cmd/herdr-sesh) >&2
}

# EXIT trap for up: a failure after mktemp must not orphan the server or the run dir.
up_failed() {
	local code=$?
	((code)) || return 0
	if [[ -f $run/server.pid ]]; then (down "$run") >&2 || true; else rm -rf "$run"; fi
	rmdir "$evidence" 2>/dev/null || true
	echo "hsv: up failed (exit $code); tore down $run" >&2
	exit "$code"
}

up() {
	local target herdr_bin id cfgdir i
	[[ $# -eq 1 ]] || usage
	target=$(git -C "$1" rev-parse --show-toplevel) || die "not a git checkout: $1"
	[[ -f $target/herdr-plugin.toml ]] || die "no herdr-plugin.toml in $target"
	herdr_bin=$(command -v herdr) || die "herdr not on PATH"
	command -v tmux >/dev/null || die "tmux not on PATH"
	command -v go >/dev/null || die "go not on PATH"

	# Short /tmp path: macOS caps unix socket paths at 104 bytes.
	run=$(mktemp -d /tmp/hsv.XXXXXX)
	echo "hsv: RUN=$run" >&2
	id=${run##*/hsv.}
	evidence=$target/scratch/verify-herdr-sesh/$id
	trap up_failed EXIT
	mkdir -p "$run"/{home,cfg,state,data,cache,plugin/bin} "$evidence"
	cp "$target/herdr-plugin.toml" "$run/plugin/herdr-plugin.toml"
	build "$target" "$run"
	cat >"$run/env.sh" <<EOF
unset \$(env | sed -n 's/^\(HERDR_[A-Za-z0-9_]*\)=.*/\1/p') TMUX _ZO_DATA_DIR
export HSV_RUN='$run' HSV_TARGET='$target' HSV_EVIDENCE='$evidence' HSV_TMUX='hsv-$id'
export HOME='$run/home' XDG_CONFIG_HOME='$run/cfg' XDG_STATE_HOME='$run/state' XDG_DATA_HOME='$run/data' XDG_CACHE_HOME='$run/cache'
export HERDR_SOCKET_PATH='$run/cfg/herdr/herdr.sock' HERDR_BIN_PATH='$herdr_bin'
export PATH='$run/plugin/bin':"\$PATH"
EOF
	(
		# shellcheck disable=SC1091
		. "$run/env.sh"
		cd "$run"
		nohup "$herdr_bin" server >"$run/server.log" 2>&1 &
		echo $! >"$run/server.pid"
		started $! >"$run/server.start"
	)
	load "$run"
	for i in $(seq 50); do
		# `herdr status server` exits 0 even when not running; a real API call does not.
		herdr workspace list >/dev/null 2>&1 && break
		[[ $i == 50 ]] && die "server not ready; see $run/server.log"
		sleep 0.2
	done

	# Fixtures: testdata/herdr-sesh.toml names ~/projects/sesh; zoxide gets one extra dir.
	touch "$HOME/.zshrc"
	mkdir -p "$HOME/projects/sesh" "$HOME/projects/zoxed"
	git -C "$HOME/projects/sesh" init -q
	echo "# sesh fixture" >"$HOME/projects/sesh/README.md"
	echo "# zoxed fixture" >"$HOME/projects/zoxed/README.md"
	if command -v zoxide >/dev/null; then zoxide add "$HOME/projects/zoxed"; fi
	herdr plugin link "$run/plugin" >/dev/null
	cfgdir=$(herdr plugin config-dir fullerzz.sesh)
	mkdir -p "$cfgdir"
	cp "$target/testdata/herdr-sesh.toml" "$cfgdir/config.toml"
	echo "export HERDR_PLUGIN_CONFIG_DIR='$cfgdir'" >>"$run/env.sh"
	trap - EXIT
	echo "$run"
}

rebuild() {
	local target
	[[ $# -eq 1 ]] || usage
	# Validate and read the target in a subshell so the build keeps the caller's env.
	target=$(load "$1" && echo "$HSV_TARGET")
	build "$target" "$1"
	echo "rebuilt $1/plugin/bin/herdr-sesh" >&2
}

# No Go source in the checkout is newer than the run's binary.
fresh() {
	local bin=$run/plugin/bin/herdr-sesh
	[[ -x $bin && -z $(find "$HSV_TARGET" -name .git -prune -o \( -name '*.go' -o -name go.mod -o -name go.sum \) -newer "$bin" -print -quit) ]]
}

# Herdr reports plugin_root resolved (/tmp -> /private/tmp on macOS).
linked() {
	herdr plugin list --json 2>/dev/null | jq -e --arg root "$(cd "$run/plugin" && pwd -P)" \
		'.result.plugins | any(.plugin_id == "fullerzz.sesh" and .enabled and .plugin_root == $root)' >/dev/null
}

doctor() {
	local pid fail=0
	load "$@"
	check() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
	pid=$(cat "$run/server.pid")
	check "server pid $pid is the process this run started" owned
	check "socket answers at $HERDR_SOCKET_PATH" "herdr workspace list >/dev/null 2>&1"
	check "plugin fullerzz.sesh enabled from $run/plugin" linked
	check "binary newer than checkout Go sources (fix: hsv.sh rebuild $run)" fresh
	check "plugin state isolated under $XDG_STATE_HOME" "[[ -d '$XDG_STATE_HOME/herdr/plugins/fullerzz.sesh' ]]"
	check "fixture config at $HERDR_PLUGIN_CONFIG_DIR/config.toml" "[[ -f '$HERDR_PLUGIN_CONFIG_DIR/config.toml' ]]"
	echo "evidence: $HSV_EVIDENCE"
	return $fail
}

x() {
	load "$1"
	shift
	exec "$@"
}

# Plugin overlays open asynchronously; poll the focused pane (HERDR_PANE_ID is unset by env.sh).
overlay() {
	local i limit id
	load "$1"
	limit=$(( ${3:-10} * 5 ))
	for ((i = 0; i < limit; i++)); do
		id=$(herdr pane current 2>/dev/null | jq -r --arg l "$2" '.result.pane | select(.label == $l) | .pane_id' 2>/dev/null) || true
		[[ -n $id ]] && { echo "$id"; return 0; }
		sleep 0.2
	done
	die "timed out waiting for focused pane '$2'"
}

cli() {
	local label code
	load "$1"
	label=$2
	shift 2
	printf '%q ' "$@" >"$HSV_EVIDENCE/$label.cmd"
	echo >>"$HSV_EVIDENCE/$label.cmd"
	set +e
	"$@" >"$HSV_EVIDENCE/$label.out" 2>"$HSV_EVIDENCE/$label.err"
	code=$?
	set -e
	echo "$code" >"$HSV_EVIDENCE/$label.exit"
	cat "$HSV_EVIDENCE/$label.out"
	cat "$HSV_EVIDENCE/$label.err" >&2
	echo "[exit $code -> $HSV_EVIDENCE/$label.*]" >&2
	return "$code"
}

tui() {
	local name
	load "$1"
	name=$2
	shift 2
	cd "$HOME"
	# The pane survives the command so its last screen and exit code stay readable.
	tmux -L "$HSV_TMUX" -f /dev/null new-session -d -s "$name" -x 120 -y 32 \
		sh -c '"$@"; echo "[hsv exit $?]"; exec sleep 86400' sh "$@"
}

keys() {
	load "$1"
	tmux -L "$HSV_TMUX" send-keys -t "$2" "${@:3}"
}

wait_for() {
	local i limit
	load "$1"
	limit=$(( ${4:-10} * 5 ))
	for ((i = 0; i < limit; i++)); do
		tmux -L "$HSV_TMUX" capture-pane -p -t "$2" | grep -qF -- "$3" && return 0
		sleep 0.2
	done
	tmux -L "$HSV_TMUX" capture-pane -p -t "$2" >&2
	die "timed out waiting for '$3' in $2"
}

snap() {
	load "$1"
	tmux -L "$HSV_TMUX" capture-pane -p -t "$2" | tee "$HSV_EVIDENCE/$3.txt"
}

down() {
	local pid k i mine=0
	local -a kids=() kid_starts=()
	load "$@"
	tmux -L "$HSV_TMUX" kill-server 2>/dev/null || true
	pid=$(cat "$run/server.pid")
	if owned; then
		mine=1
		# Children of the server this run started (history watcher, pane shells), with identities.
		for k in $(pgrep -P "$pid" || true); do
			kids+=("$k")
			kid_starts+=("$(started "$k")")
		done
	else
		echo "hsv: pid $pid is not this run's server (exited or reused); signaling nothing" >&2
	fi
	# Addressed by this run's socket path, so it cannot reach another Herdr.
	herdr server stop >/dev/null 2>&1 || true
	if ((mine)); then
		for i in $(seq 20); do
			owned || break
			sleep 0.25
		done
		if owned; then kill "$pid" 2>/dev/null || true; fi
		for i in "${!kids[@]}"; do
			if [[ -n ${kid_starts[i]} && $(started "${kids[i]}") == "${kid_starts[i]}" ]]; then
				kill "${kids[i]}" 2>/dev/null || true
			fi
		done
	fi
	rm -rf "$run"
	echo "removed $run; evidence kept in $HSV_EVIDENCE"
}

cmd=${1:-}
shift || true
case $cmd in
up) up "$@" ;;
rebuild) rebuild "$@" ;;
doctor) doctor "$@" ;;
x) [[ $# -ge 2 ]] || usage; x "$@" ;;
overlay) [[ $# -ge 2 && $# -le 3 ]] || usage; overlay "$@" ;;
cli) [[ $# -ge 3 ]] || usage; cli "$@" ;;
tui) [[ $# -ge 3 ]] || usage; tui "$@" ;;
keys) [[ $# -ge 3 ]] || usage; keys "$@" ;;
wait) [[ $# -ge 3 ]] || usage; wait_for "$@" ;;
snap) [[ $# -eq 3 ]] || usage; snap "$@" ;;
down) down "$@" ;;
*) usage ;;
esac

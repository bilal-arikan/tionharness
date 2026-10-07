# shellcheck shell=bash
# Shared helpers for the macOS/Linux launchers (serve.sh, dev.sh) -- the bash
# counterpart of lib/ports.ps1. Source it; do not execute it.
#
# Must stay compatible with bash 3.2 (stock /bin/bash on macOS) and BSD userland:
# no associative arrays, no ${var,,}, no `wait -n`, no GNU-only flags.

th_api_pkg=github.com/bilal-arikan/tionharness/internal/api

# th_version_ldflags prints the -X flags scripts/ci.sh and build.ps1 inject, so
# /api/version reports the commit and build time instead of "unknown". A broken
# git (e.g. an unaccepted Xcode license on a fresh Mac) must not block a local
# build, so a failed lookup degrades to "unknown".
th_version_ldflags() {
  local commit build_time
  commit=$(git rev-parse --short HEAD 2>/dev/null) || commit=unknown
  [ -n "$commit" ] || commit=unknown
  build_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '%s' "-X $th_api_pkg.BuildCommit=$commit -X $th_api_pkg.BuildDate=$build_time"
}

th_warned_no_port_tool=0

# th_port_pids <port> prints the PIDs listening on TCP <port>, one per line.
# lsof (macOS, most Linux) -> ss (iproute2) -> fuser (psmisc, Linux syntax).
# With none of them installed it warns once and prints nothing: the server then
# fails loudly on bind instead of this script killing the wrong thing.
th_port_pids() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true
  elif command -v ss >/dev/null 2>&1; then
    # Without root ss only shows pid= for our own processes -- which are the
    # orphans this exists for anyway.
    ss -tlnp "sport = :$port" 2>/dev/null | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u || true
  elif command -v fuser >/dev/null 2>&1; then
    # Linux fuser prints the PIDs on stdout and the "8090/tcp:" label on stderr.
    fuser -n tcp "$port" 2>/dev/null | tr -s ' \t' '\n' | grep -E '^[0-9]+$' || true
  elif [ "$th_warned_no_port_tool" = 0 ]; then
    th_warned_no_port_tool=1
    echo "==> WARNING: none of lsof/ss/fuser found; cannot detect a process already holding port $port." >&2
  fi
}

# th_lock_holder_pids prints running TionHarness processes that would hold the
# default data dir's instance.lock. With TIONHARNESS_DATA_DIR set the new server
# uses its own lock, so nothing is reported.
th_lock_holder_pids() {
  [ -z "${TIONHARNESS_DATA_DIR:-}" ] || return 0
  pgrep -x tionharness 2>/dev/null || true
  pgrep -x tionharness-dev 2>/dev/null || true
}

# th_stop_pids <label> <kill:1|0> [pid...] stops the given processes (TERM, then
# KILL after ~3s). With kill=0 it prints an error and returns 1 instead.
th_stop_pids() {
  local label="$1" kill_it="$2" pids
  shift 2
  pids=$(printf '%s\n' "$@" | grep -E '^[0-9]+$' | sort -u | tr '\n' ' ' || true)
  pids="${pids% }"
  [ -n "$pids" ] || return 0
  if [ "$kill_it" = 0 ]; then
    echo "==> ERROR: $label busy (PID $pids). --no-kill-port set, aborting." >&2
    return 1
  fi
  echo "==> Stopping $label holder (PID $pids)"
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
  local _
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    # shellcheck disable=SC2086
    kill -0 $pids 2>/dev/null || break
    sleep 0.3
  done
  # shellcheck disable=SC2086
  kill -9 $pids 2>/dev/null || true
}

# th_descendants <pid> prints every descendant PID, deepest first (pgrep -P is
# available on both macOS and procps Linux).
th_descendants() {
  local child
  for child in $(pgrep -P "$1" 2>/dev/null || true); do
    th_descendants "$child"
    echo "$child"
  done
}

# th_kill_tree <pid> stops a process and everything it spawned (npm -> sh ->
# node, or the backend's agent/shell children): TERM first, KILL after ~3s.
th_kill_tree() {
  local pids _
  pids="$(th_descendants "$1" | tr '\n' ' ')$1"
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    # shellcheck disable=SC2086
    kill -0 $pids 2>/dev/null || return 0
    sleep 0.3
  done
  # shellcheck disable=SC2086
  kill -9 $pids 2>/dev/null || true
}

# th_lan_ip prints the IPv4 address other LAN devices can reach (the default
# route's interface), or 127.0.0.1 when offline / undetectable.
th_lan_ip() {
  local ip="" iface
  if command -v ipconfig >/dev/null 2>&1 && command -v route >/dev/null 2>&1 && [ "$(uname -s)" = Darwin ]; then
    iface=$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}' || true)
    [ -z "$iface" ] || ip=$(ipconfig getifaddr "$iface" 2>/dev/null || true)
  elif command -v ip >/dev/null 2>&1; then
    ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}' || true)
  fi
  printf '%s\n' "${ip:-127.0.0.1}"
}

#!/usr/bin/env bash
# serve.sh -- macOS/Linux counterpart of serve.ps1: clean build + run of the single
# binary (backend only, no Vite). See serve.ps1 for why this builds explicitly
# instead of `go run` (a stale cached link can serve an old binary).
#
# Usage:
#   scripts/serve.sh                  # 0.0.0.0:5173 (LAN-reachable)
#   scripts/serve.sh --loopback       # 127.0.0.1 only
#   scripts/serve.sh --port 8090
#   scripts/serve.sh --ui             # also rebuild the embedded frontend first
#   scripts/serve.sh --clean          # also wipe the Go build cache first
#   scripts/serve.sh --no-kill-port   # abort instead of killing a port squatter
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

port=5173
bind_host=0.0.0.0
ui=0
clean=0
kill_port=1
while [ $# -gt 0 ]; do
  case "$1" in
    --loopback) bind_host=127.0.0.1 ;;
    --port) port="$2"; shift ;;
    --ui) ui=1 ;;
    --clean) clean=1 ;;
    --no-kill-port) kill_port=0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

bin="$root/bin/tionharness"

# 1) Optional frontend build (vite writes into internal/web/dist, embedded by go:embed).
if [ "$ui" = 1 ]; then
  echo "==> Building frontend: npm --prefix frontend run build"
  npm --prefix frontend run build
fi

# 2) Clean build. -buildvcs=false: a broken git shim (e.g. unaccepted Xcode
# license) must not block a local run.
if [ "$clean" = 1 ]; then
  echo "==> go clean -cache"
  go clean -cache
fi
mkdir -p "$root/bin"
echo "==> Building: go build -o bin/tionharness ./cmd/tionharness"
go build -buildvcs=false -o "$bin" ./cmd/tionharness
echo "==> Build OK: $bin ($(du -h "$bin" | cut -f1))"

# 3) Free the port. Without a custom data dir, any other instance also holds the
# ~/.tionharness lock, so it has to go too.
pids="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
if [ -z "${TIONHARNESS_DATA_DIR:-}" ]; then
  pids="$pids $(pgrep -x tionharness 2>/dev/null || true)"
fi
pids="$(echo $pids | tr ' ' '\n' | sort -u | tr '\n' ' ')"
if [ -n "${pids// /}" ]; then
  if [ "$kill_port" = 0 ]; then
    echo "==> ERROR: port $port or data dir busy (PID $pids). --no-kill-port set, aborting." >&2
    exit 1
  fi
  echo "==> Stopping previous instance / port squatter (PID $pids)"
  kill $pids 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 $pids 2>/dev/null || break
    sleep 0.3
  done
  kill -9 $pids 2>/dev/null || true
fi

# 4) Run in the foreground -- Ctrl+C stops it.
export TIONHARNESS_ADDR="${bind_host}:${port}"
export TIONHARNESS_ENABLE_SHELL=1
echo "==> Running: $bin  (${bind_host}:${port})  -- Ctrl+C to stop"
exec "$bin"

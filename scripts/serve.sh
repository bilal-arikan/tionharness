#!/usr/bin/env bash
# serve.sh -- macOS/Linux counterpart of serve.ps1: clean build + run of the single
# binary (backend only, no Vite). See serve.ps1 for why this builds explicitly
# instead of `go run` (a stale cached link can serve an old binary).
#
# Usage:
#   scripts/serve.sh                  # 0.0.0.0:5173 (LAN-reachable)
#   scripts/serve.sh --loopback       # 127.0.0.1 only
#   scripts/serve.sh --port 8090      # serve.ps1's default; also the port the Vite dev
#                                     # server proxies /api to (frontend/vite.config.ts),
#                                     # so `--loopback --port 8090` + `npm --prefix
#                                     # frontend run dev` gives hot reload on :5173.
#                                     # scripts/dev.sh does that pairing in one command.
#   scripts/serve.sh --ui             # also rebuild the embedded frontend first
#   scripts/serve.sh --clean          # also wipe the Go build cache first
#   scripts/serve.sh --no-kill-port   # abort instead of killing a port squatter
#
# The default stays 5173 (not serve.ps1's 8090): this is the address the macOS
# setup has always used. Note that it is also Vite's dev port, so do not run this
# with the default port alongside scripts/dev.sh / `npm run dev`.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
# shellcheck source=lib/common.sh
. "$root/scripts/lib/common.sh"

port=5173
bind_host=0.0.0.0
ui=0
clean=0
kill_port=1
while [ $# -gt 0 ]; do
  case "$1" in
    --loopback) bind_host=127.0.0.1 ;;
    --port) [ $# -ge 2 ] || { echo "--port needs a value" >&2; exit 2; }; port="$2"; shift ;;
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
# license) must not block a local run. The -X version flags are the ones
# scripts/ci.sh and build.ps1 inject (see lib/common.sh).
if [ "$clean" = 1 ]; then
  echo "==> go clean -cache"
  go clean -cache
fi
mkdir -p "$root/bin"
echo "==> Building: go build -o bin/tionharness ./cmd/tionharness"
go build -buildvcs=false -ldflags "$(th_version_ldflags)" -o "$bin" ./cmd/tionharness
echo "==> Build OK: $bin ($(du -h "$bin" | cut -f1))"

# 3) Free the port. Without a custom data dir, any other instance also holds the
# ~/.tionharness lock, so it has to go too.
# shellcheck disable=SC2046
th_stop_pids "port $port / data dir" "$kill_port" $(th_port_pids "$port") $(th_lock_holder_pids) || exit 1

# 4) Run in the foreground -- Ctrl+C stops it.
export TIONHARNESS_ADDR="${bind_host}:${port}"
export TIONHARNESS_ENABLE_SHELL=1
echo "==> Running: $bin  (${bind_host}:${port})  -- Ctrl+C to stop"
exec "$bin"

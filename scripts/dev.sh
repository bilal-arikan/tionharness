#!/usr/bin/env bash
# dev.sh -- macOS/Linux counterpart of dev.ps1: development run (backend + Vite).
#
# Builds the Go backend to bin/tionharness-dev, runs it (default 8090) and starts
# the Vite dev server (npm run dev on :5173, proxies /api to 127.0.0.1:8090 --
# see frontend/vite.config.ts) beside it. Ctrl+C, SIGTERM or either child dying
# tears BOTH process trees down so no orphan node/go server keeps a port.
#
# Like dev.ps1 it builds then runs the binary instead of `go run`: one process,
# real exit codes, and Go runtime fatals land in the stderr capture (the WHY is in
# dev.ps1's header).
#
# By default both servers bind 0.0.0.0, so the UI is reachable from other devices
# at http://<LAN-IP>:5173. The backend has NO auth and CORS is wildcard -- only do
# that on a trusted network; pass --loopback for 127.0.0.1 only.
#
# Usage:
#   scripts/dev.sh                    # backend + frontend on LAN, opens the browser
#   scripts/dev.sh --loopback         # bind 127.0.0.1 only
#   scripts/dev.sh --no-browser       # don't open the browser
#   scripts/dev.sh --backend-only     # only the Go backend
#   scripts/dev.sh --frontend-only    # only the Vite dev server
#   scripts/dev.sh --port 8091        # backend port (Vite still proxies to 8090!)
#   scripts/dev.sh --no-kill-port     # abort instead of killing a port/lock holder
#
# Pre-flight (as in serve.sh): a listener on the backend port or :5173 is killed,
# and without TIONHARNESS_DATA_DIR so is any running tionharness/tionharness-dev,
# because it holds ~/.tionharness/instance.lock. That includes a scripts/serve.sh
# server on its default :5173. Use --no-kill-port, or a separate data dir:
#   TIONHARNESS_DATA_DIR="${TMPDIR:-/tmp}/th-dev-data" scripts/dev.sh --backend-only --port 8097
#
# Diagnostics (same layout as dev.ps1): _devlogs/lifecycle.log records every
# launch/kill across runs; _devlogs/backend-stderr-<stamp>.log captures the
# backend's STDERR (STDOUT stays live on this console and is mirrored by the app to
# <data dir>/logs/tionharness.log). Empty captures are deleted on exit, so a
# surviving file means something went wrong. If the backend dies on its own, the
# tail of the app log is printed too.
#
# Must stay bash 3.2 compatible (stock macOS /bin/bash): no associative arrays,
# no ${var,,}, no `wait -n`.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
# shellcheck source=lib/common.sh
. "$root/scripts/lib/common.sh"

port=8090
vite_port=5173
loopback=0
open_browser=1
backend_only=0
frontend_only=0
kill_port=1
while [ $# -gt 0 ]; do
  case "$1" in
    --port) [ $# -ge 2 ] || { echo "--port needs a value" >&2; exit 2; }; port="$2"; shift ;;
    --loopback) loopback=1 ;;
    --no-browser) open_browser=0 ;;
    --backend-only) backend_only=1 ;;
    --frontend-only) frontend_only=1 ;;
    --no-kill-port) kill_port=0 ;;
    -h | --help) sed -n '2,/^set -euo/p' "$0" | sed '$d'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
if [ "$backend_only" = 1 ] && [ "$frontend_only" = 1 ]; then
  echo "--backend-only and --frontend-only are mutually exclusive" >&2
  exit 2
fi

if [ "$loopback" = 1 ]; then
  bind_host=127.0.0.1
  lan_ip=127.0.0.1
else
  bind_host=0.0.0.0
  lan_ip=$(th_lan_ip)
fi

log_dir="$root/_devlogs"
mkdir -p "$log_dir"
run_stamp=$(date +%Y%m%d-%H%M%S)
backend_err="$log_dir/backend-stderr-$run_stamp.log"
lifecycle_log="$log_dir/lifecycle.log"
bin="$root/bin/tionharness-dev"
app_log="${TIONHARNESS_DATA_DIR:-$HOME/.tionharness}/logs/tionharness.log"

backend_pid=
frontend_pid=
backend_self_exited=0
cleaned=0

lifecycle() {
  printf '%s %s\n' "$(date +%Y-%m-%dT%H:%M:%S%z)" "$*" >>"$lifecycle_log" 2>/dev/null || true
}

# exit_reason turns a POSIX exit status into the sentence lifecycle.log wants
# (dev.ps1's Get-ExitReason decodes NTSTATUS codes; here it is 128+signal).
exit_reason() {
  local code="$1" label="$2" sig
  case "$code" in
    0) echo "clean exit" ;;
    1) echo "general error" ;;
    2)
      if [ "$label" = backend ]; then
        echo "Go runtime fatal (panic/OOM) -- see the stderr capture"
      else
        echo "npm/vite launch failure -- see the console"
      fi
      ;;
    137) echo "SIGKILL -- killed from outside (kill -9) or by the OOM killer" ;;
    *)
      if [ "$code" -gt 128 ] 2>/dev/null; then
        sig=$(kill -l $((code - 128)) 2>/dev/null || echo "?")
        echo "terminated by signal $((code - 128)) (SIG$sig)"
      else
        echo "unknown exit code $code"
      fi
      ;;
  esac
}

# free_port <port> <label>: clear an orphan still listening before we bind.
free_port() {
  local pids
  pids=$(th_port_pids "$1" | tr '\n' ' ')
  [ -n "${pids// /}" ] || return 0
  [ "$kill_port" = 0 ] || lifecycle "dev.sh pre-flight: killing orphan on port $1 (pid=$pids)"
  # shellcheck disable=SC2086
  th_stop_pids "$2 port $1" "$kill_port" $pids
}

# report_child <pid> <label>: reap an exited child and log why it went.
report_child() {
  local code=0
  wait "$1" 2>/dev/null || code=$?
  local reason
  reason=$(exit_reason "$code" "$2")
  echo "==> $2 exited (PID $1, exit $code -- $reason)" >&2
  lifecycle "$2 exited on its own (pid=$1 exit=$code reason=$reason)"
  [ "$2" != backend ] || backend_self_exited=1
}

cleanup() {
  local rc=$?
  set +e
  [ "$cleaned" = 0 ] || return 0
  cleaned=1
  echo "==> Cleanup: stopping processes..."
  lifecycle "dev.sh cleanup started (rc=$rc)"
  local entry pid label
  for entry in "frontend:${frontend_pid}" "backend:${backend_pid}"; do
    label=${entry%%:*}
    pid=${entry#*:}
    [ -n "$pid" ] || continue
    if kill -0 "$pid" 2>/dev/null; then
      lifecycle "dev.sh cleanup: killing tree $label (pid=$pid)"
      th_kill_tree "$pid"
      wait "$pid" 2>/dev/null
    elif [ "$label" = backend ] && [ "$backend_self_exited" = 0 ]; then
      report_child "$pid" backend
    fi
  done

  # The backend died without us: anything it spawned can still own the port.
  # Only on a self-exit -- on a normal teardown a listener there is someone else's.
  if [ "$backend_self_exited" = 1 ] && [ "$frontend_only" = 0 ]; then
    free_port "$port" "Backend (orphan)"
  fi

  if [ -f "$backend_err" ]; then
    if [ -s "$backend_err" ]; then
      local len
      len=$(wc -c <"$backend_err" | tr -d ' ')
      lifecycle "backend stderr captured: $backend_err ($len bytes)"
      echo
      echo "==> backend STDERR captured ($len bytes): $backend_err" >&2
      tail -n 40 "$backend_err" | sed 's/^/    /' >&2
      echo
    else
      rm -f "$backend_err"
    fi
  fi

  # An externally killed process writes nothing to stderr; the app's own log is
  # then the only record of what it was doing.
  if [ "$backend_self_exited" = 1 ] && [ -f "$app_log" ]; then
    echo
    echo "==> Backend died on its own. Tail of the app log ($app_log):" >&2
    tail -n 25 "$app_log" | sed 's/^/    /' >&2
    echo
  fi

  # Keep only the 10 newest captures.
  # shellcheck disable=SC2012
  ls -t "$log_dir"/backend-stderr-*.log 2>/dev/null | tail -n +11 | while IFS= read -r f; do
    rm -f "$f"
  done

  lifecycle "dev.sh cleanup finished"
  echo "==> Stopped."
  exit "$rc"
}
trap cleanup EXIT
trap 'lifecycle "dev.sh got SIGINT"; exit 130' INT
trap 'lifecycle "dev.sh got SIGTERM"; exit 143' TERM
trap 'lifecycle "dev.sh got SIGHUP"; exit 129' HUP

# frontend_deps_ok answers "will `npm run dev` actually start?" by LOADING vite,
# not by looking for files (dev.ps1 Test-FrontendDeps: a half-extracted package
# passes every existence check).
frontend_deps_ok() {
  [ -x "$root/frontend/node_modules/.bin/vite" ] || return 1
  (cd "$root/frontend" && node -e "import('vite').then(()=>process.exit(0),e=>{console.log('vite-load-failed:'+e.code);process.exit(1)})" >/dev/null 2>&1)
}

lifecycle "dev.sh start (bind=$bind_host:$port backendOnly=$backend_only frontendOnly=$frontend_only)"

if [ "$frontend_only" = 0 ]; then
  free_port "$port" "Backend"
  # Without a custom data dir another instance holds ~/.tionharness/instance.lock.
  # shellcheck disable=SC2046
  th_stop_pids "data dir lock" "$kill_port" $(th_lock_holder_pids)

  mkdir -p "$root/bin"
  echo "==> Building backend: go build -o bin/tionharness-dev ./cmd/tionharness"
  # -buildvcs=false: a broken git shim (unaccepted Xcode license) must not block a run.
  if ! go build -buildvcs=false -ldflags "$(th_version_ldflags)" -o "$bin" ./cmd/tionharness; then
    lifecycle "backend build FAILED"
    echo "==> ERROR: backend build failed" >&2
    exit 1
  fi
  echo "==> Starting backend: bin/tionharness-dev  ($bind_host:$port)"
  # Only STDERR is captured; STDOUT (slog) stays live on this console.
  TIONHARNESS_ADDR="$bind_host:$port" TIONHARNESS_ENABLE_SHELL=1 "$bin" 2>"$backend_err" &
  backend_pid=$!
  lifecycle "backend launched (pid=$backend_pid stderr=$backend_err)"
fi

# Wait for /health before starting Vite, or Vite spams ECONNREFUSED meanwhile.
if [ "$frontend_only" = 0 ] && [ "$backend_only" = 0 ]; then
  echo "==> Waiting for backend /health..."
  ready=0
  if command -v curl >/dev/null 2>&1; then
    for _ in $(seq 1 240); do
      if ! kill -0 "$backend_pid" 2>/dev/null; then
        report_child "$backend_pid" backend
        echo "==> ERROR: backend exited before becoming healthy" >&2
        exit 1
      fi
      if curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:$port/health" 2>/dev/null; then
        ready=1
        break
      fi
      sleep 0.5
    done
  else
    sleep 3
  fi
  if [ "$ready" = 1 ]; then
    echo "==> Backend ready."
  else
    echo "==> Backend not confirmed healthy; starting the frontend anyway." >&2
  fi
fi

if [ "$backend_only" = 0 ]; then
  if [ "$port" != 8090 ]; then
    echo "==> WARNING: Vite proxies /api to 127.0.0.1:8090 (frontend/vite.config.ts), not $port." >&2
  fi
  free_port "$vite_port" "Frontend"
  command -v node >/dev/null 2>&1 || { echo "==> ERROR: node not on PATH -- cannot start the frontend" >&2; exit 1; }
  # Repair ladder (dev.ps1): npm install cannot fix a half-extracted package --
  # npm thinks it is installed -- so escalate to a wipe + npm ci.
  if ! frontend_deps_ok; then
    echo "==> frontend deps broken -> npm install..." >&2
    lifecycle "dev.sh pre-flight: frontend deps broken, running npm install"
    npm --prefix "$root/frontend" install
    if ! frontend_deps_ok; then
      echo "==> still broken -> wiping frontend/node_modules + clean install..." >&2
      lifecycle "dev.sh pre-flight: still broken, wiping node_modules"
      rm -rf "$root/frontend/node_modules"
      if [ -f "$root/frontend/package-lock.json" ]; then
        npm --prefix "$root/frontend" ci
      else
        npm --prefix "$root/frontend" install
      fi
      if ! frontend_deps_ok; then
        lifecycle "dev.sh pre-flight: frontend deps UNREPAIRABLE"
        echo "==> ERROR: vite still fails to load -- run: cd frontend && node -e \"import('vite')\"" >&2
        exit 1
      fi
    fi
    lifecycle "dev.sh pre-flight: frontend deps repaired"
  fi
  # Heap ceiling for a long-lived dev server (dev.ps1: Vite died of heap growth
  # after hours of HMR). Respect an existing NODE_OPTIONS.
  case "${NODE_OPTIONS:-}" in
    *max-old-space-size*) ;;
    "") export NODE_OPTIONS="--max-old-space-size=4096" ;;
    *) export NODE_OPTIONS="$NODE_OPTIONS --max-old-space-size=4096" ;;
  esac
  echo "==> Starting frontend: npm run dev  (http://$lan_ip:$vite_port)"
  if [ "$loopback" = 1 ]; then
    (cd "$root/frontend" && exec npm run dev) &
  else
    (cd "$root/frontend" && exec npm run dev -- --host 0.0.0.0) &
  fi
  frontend_pid=$!
  lifecycle "frontend launched (pid=$frontend_pid)"

  if [ "$open_browser" = 1 ]; then
    opener=
    if command -v open >/dev/null 2>&1 && [ "$(uname -s)" = Darwin ]; then
      opener=open
    elif command -v xdg-open >/dev/null 2>&1; then
      opener=xdg-open
    fi
    [ -z "$opener" ] || (sleep 3 && "$opener" "http://$lan_ip:$vite_port" >/dev/null 2>&1) &
  fi
fi

echo
echo "==> Running. Ctrl+C stops everything."
if [ "$backend_only" = 1 ]; then
  echo "==> API: http://127.0.0.1:$port"
elif [ "$loopback" = 1 ]; then
  echo "==> UI: http://127.0.0.1:$vite_port  (this machine only / --loopback)"
else
  echo "==> UI (this machine): http://127.0.0.1:$vite_port"
  echo "==> UI (LAN):          http://$lan_ip:$vite_port"
  echo "==> API has NO auth and CORS is wildcard -- trusted networks only." >&2
fi
echo

# Poll instead of `wait -n` (bash 3.2): if one child dies, take the other down.
while :; do
  sleep 0.5
  if [ -n "$backend_pid" ] && ! kill -0 "$backend_pid" 2>/dev/null; then
    report_child "$backend_pid" backend
    exit 1
  fi
  if [ -n "$frontend_pid" ] && ! kill -0 "$frontend_pid" 2>/dev/null; then
    report_child "$frontend_pid" frontend
    frontend_pid=
    exit 1
  fi
done

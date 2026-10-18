#!/usr/bin/env bash
# service.sh -- run TionHarness as a macOS launchd user agent, and redeploy it
# without cutting a running turn.
#
# Usage:
#   scripts/service.sh install [--loopback] [--port N]   # build, write the plist, start
#   scripts/service.sh uninstall                         # stop and remove the plist
#   scripts/service.sh status                            # launchd state + live workspaces
#   scripts/service.sh deploy [--ui] [--force] [--timeout MIN]
#                                                        # build, wait until idle, swap, restart
#   scripts/service.sh restart [--force] [--timeout MIN] # wait until idle, restart
#
# Why a drain: a plain restart kills every in-flight turn (a CLI subprocess mid-tool
# call, a half-written reply). deploy/restart poll GET /api/workspaces/activity and
# only restart once no workspace has been running for three consecutive polls, or
# give up at --timeout (default 30 min) unless --force. The new binary is built
# next to the old one and moved into place with an atomic rename, so the running
# process never executes a half-written file.
#
# launchd keeps the server alive (KeepAlive) and restarts it after a crash; logs go
# to ~/.tionharness/logs/service.log. Must stay bash 3.2 compatible (stock macOS).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
# shellcheck source=lib/common.sh
. "$root/scripts/lib/common.sh"

label=dev.tionharness.server
plist="$HOME/Library/LaunchAgents/$label.plist"
bin="$root/bin/tionharness"
log_dir="$HOME/.tionharness/logs"
domain="gui/$(id -u)"

[ "$(uname -s)" = Darwin ] || { echo "service.sh is macOS-only (launchd)" >&2; exit 2; }

cmd="${1:-}"
[ $# -gt 0 ] && shift
port=5173
bind_host=0.0.0.0
ui=0
force=0
timeout_min=30
while [ $# -gt 0 ]; do
  case "$1" in
    --loopback) bind_host=127.0.0.1 ;;
    --port) [ $# -ge 2 ] || { echo "--port needs a value" >&2; exit 2; }; port="$2"; shift ;;
    --ui) ui=1 ;;
    --force) force=1 ;;
    --timeout) [ $# -ge 2 ] || { echo "--timeout needs a value" >&2; exit 2; }; timeout_min="$2"; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

# installed_addr reads the address the installed plist serves on, so restart and
# deploy poll the right port even when install used --port/--loopback.
installed_addr() {
  /usr/libexec/PlistBuddy -c "Print :EnvironmentVariables:TIONHARNESS_ADDR" "$plist" 2>/dev/null || echo "127.0.0.1:$port"
}

api_base() {
  local addr host p
  addr="$(installed_addr)"
  host="${addr%:*}"
  p="${addr##*:}"
  [ "$host" = 0.0.0.0 ] && host=127.0.0.1
  printf 'http://%s:%s' "$host" "$p"
}

# build_to <path> builds the server binary (and, with --ui, the embedded frontend).
build_to() {
  if [ "$ui" = 1 ]; then
    echo "==> Building frontend"
    npm --prefix frontend run build
  fi
  mkdir -p "$(dirname "$1")"
  echo "==> Building $1"
  go build -buildvcs=false -ldflags "$(th_version_ldflags)" -o "$1" ./cmd/tionharness
}

# busy_workspaces prints the ids of workspaces with live work, or "?" when the
# server does not answer (treated as idle: nothing to protect).
busy_workspaces() {
  curl -fsS --max-time 5 "$(api_base)/api/workspaces/activity" 2>/dev/null |
    python3 -c 'import json,sys; print(" ".join(w["id"] for w in json.load(sys.stdin) if w.get("running")))' 2>/dev/null ||
    echo "?"
}

# wait_idle blocks until no workspace runs anything for three polls in a row.
wait_idle() {
  local deadline quiet busy
  deadline=$(( $(date +%s) + timeout_min * 60 ))
  quiet=0
  while :; do
    busy="$(busy_workspaces)"
    if [ "$busy" = "?" ]; then
      echo "==> Server not answering; restarting without a drain"
      return 0
    fi
    if [ -z "$busy" ]; then
      quiet=$((quiet + 1))
      [ "$quiet" -ge 3 ] && { echo "==> Idle"; return 0; }
    else
      quiet=0
      echo "==> Waiting for live work in: $busy"
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      if [ "$force" = 1 ]; then
        echo "==> Drain timed out after ${timeout_min}m; --force given, restarting anyway"
        return 0
      fi
      echo "==> Drain timed out after ${timeout_min}m; nothing restarted (use --force to override)" >&2
      return 1
    fi
    sleep 10
  done
}

restart_service() {
  launchctl kickstart -k "$domain/$label"
  echo "==> Restarted $label"
}

case "$cmd" in
  install)
    build_to "$bin"
    mkdir -p "$log_dir" "$(dirname "$plist")"
    # A foreground serve.sh or an older agent would hold the port and the data-dir lock.
    launchctl bootout "$domain/$label" 2>/dev/null || true
    # shellcheck disable=SC2046
    th_stop_pids "port $port / data dir" 1 $(th_port_pids "$port") $(th_lock_holder_pids) || exit 1
    cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key><array><string>$bin</string></array>
  <key>WorkingDirectory</key><string>$root</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>TIONHARNESS_ADDR</key><string>$bind_host:$port</string>
    <key>TIONHARNESS_ENABLE_SHELL</key><string>1</string>
    <key>PATH</key><string>$HOME/.docker/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>LANG</key><string>tr_TR.UTF-8</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ExitTimeOut</key><integer>60</integer>
  <key>StandardOutPath</key><string>$log_dir/service.log</string>
  <key>StandardErrorPath</key><string>$log_dir/service.log</string>
</dict>
</plist>
EOF
    launchctl bootstrap "$domain" "$plist"
    echo "==> Installed $label on $bind_host:$port (logs: $log_dir/service.log)"
    ;;
  uninstall)
    launchctl bootout "$domain/$label" 2>/dev/null || true
    rm -f "$plist"
    echo "==> Removed $label"
    ;;
  status)
    launchctl print "$domain/$label" 2>/dev/null | grep -E '^\s*(state|pid|last exit code)' || echo "$label is not loaded"
    busy="$(busy_workspaces)"
    if [ "$busy" = "?" ]; then echo "server: not answering"; else echo "live workspaces: ${busy:-none}"; fi
    ;;
  deploy)
    [ -f "$plist" ] || { echo "not installed; run: scripts/service.sh install" >&2; exit 1; }
    build_to "$bin.next"
    wait_idle || { rm -f "$bin.next"; exit 1; }
    mv -f "$bin.next" "$bin"
    restart_service
    ;;
  restart)
    [ -f "$plist" ] || { echo "not installed; run: scripts/service.sh install" >&2; exit 1; }
    wait_idle
    restart_service
    ;;
  *)
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac

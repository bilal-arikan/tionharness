#!/usr/bin/env bash
# Repository test runner (Git Bash). Two modes:
#   scripts/test.sh fast   -> Go packages touched by the working tree + staged/committed-
#                             since-main changes, plus frontend vitest when frontend/ changed
#   scripts/test.sh full   -> go test ./... -count=1 + frontend vitest (the delivery gate)
# Both export TIONHARNESS_ENABLE_SHELL=1 so the shell-tool tests do not skip.
set -euo pipefail
cd "$(dirname "$0")/.."
export TIONHARNESS_ENABLE_SHELL=1

mode="${1:-fast}"
status=0

run_go() {
  # $@ = package patterns
  echo "== go test $*"
  go test "$@" -count=1 2>&1 | grep -Ev '^(time=|[0-9]{4}/[0-9]{2}/[0-9]{2} .*(WARN|INFO))' || status=$?
}

run_frontend() {
  echo "== frontend vitest"
  (cd frontend && npm test --silent 2>&1 | tail -8) || status=$?
}

case "$mode" in
  full)
    run_go ./...
    run_frontend
    echo "== depcheck"
    scripts/depcheck.sh || status=$?
    ;;
  fast)
    # Changed files = uncommitted + committed on this branch since main (falls back
    # to uncommitted only when main is not available).
    base="$(git merge-base HEAD main 2>/dev/null || true)"
    changed="$( { git diff --name-only; git diff --name-only --cached; [ -n "$base" ] && git diff --name-only "$base" HEAD; } | sort -u )"
    pkgs="$(printf '%s\n' "$changed" | grep -E '\.go$' | xargs -r -n1 dirname | sort -u | sed 's|^|./|' || true)"
    if [ -n "$pkgs" ]; then
      # shellcheck disable=SC2086
      run_go $pkgs
    else
      echo "== no changed Go packages"
    fi
    if printf '%s\n' "$changed" | grep -q '^frontend/'; then
      run_frontend
    else
      echo "== frontend unchanged, vitest skipped"
    fi
    ;;
  *)
    echo "usage: scripts/test.sh [fast|full]" >&2
    exit 2
    ;;
esac

echo "== git diff --check"
git diff --check || status=$?
exit "$status"

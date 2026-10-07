#!/usr/bin/env bash
# Repository test runner (Windows'ta Git Bash; macOS/Linux'ta yerel bash). Two modes:
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
  local output
  output="$(mktemp)"
  if (cd frontend && npm test --silent > "$output" 2>&1); then
    tail -8 "$output"
  else
    status=$?
    # Keep the assertion/timeout diagnosis, not only the final failure count.
    tail -80 "$output"
  fi
  rm -f "$output"
}

case "$mode" in
  full)
    run_go ./...
    run_frontend
    echo "== depcheck"
    scripts/depcheck.sh || status=$?
    bash scripts/tests/depcheck_test.sh || status=$?
    ;;
  fast)
    # Changed files = uncommitted (incl. untracked) + committed on this branch since
    # main (falls back to uncommitted only when main is not available).
    base="$(git merge-base HEAD main 2>/dev/null || true)"
    # if-block, not `[ -n "$base" ] && ...`: without a local main that list would
    # end in status 1 and set -e/pipefail would exit the script silently.
    changed="$( {
      git diff --name-only
      git diff --name-only --cached
      git ls-files --others --exclude-standard
      if [ -n "$base" ]; then git diff --name-only "$base" HEAD; fi
    } | sort -u )"
    # A while-read loop instead of `xargs -r` (GNU-only; BSD xargs on macOS lacks
    # it). Directories that no longer exist (deleted packages) are skipped.
    pkgs="$(printf '%s\n' "$changed" | grep -E '\.go$' | while IFS= read -r f; do
      d=$(dirname "$f")
      if [ -d "$d" ]; then printf './%s\n' "$d"; fi
    done | sort -u || true)"
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

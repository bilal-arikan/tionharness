#!/usr/bin/env bash
# Dependency-direction guard for the packages split out of internal/agent
# (_Docs/81) plus the memory/awareness leaves (_Docs/94): none of them may
# import internal/agent back, or the split is a cycle waiting to happen. Run by scripts/test.sh full.
set -euo pipefail
if ! command -v go >/dev/null 2>&1; then
  echo "depcheck: required tool not found: go" >&2
  exit 1
fi
cd "$(dirname "$0")/.."
status=0
for pkg in climcp trajectory mcp/repair flow decider notes awareness; do
  if ! dependencies=$(go list -deps "./internal/$pkg"); then
    echo "depcheck: cannot inspect dependencies for internal/$pkg" >&2
    status=1
    continue
  fi
  if [ -z "$dependencies" ]; then
    echo "depcheck: empty dependency list for internal/$pkg" >&2
    status=1
    continue
  fi
  if grep -qx 'github.com/bilal-arikan/tionharness/internal/agent' <<<"$dependencies"; then
    echo "depcheck: internal/$pkg imports internal/agent (forbidden)" >&2
    status=1
  fi
done
[ "$status" -eq 0 ] && echo "depcheck: ok"
exit "$status"

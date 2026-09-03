#!/usr/bin/env bash
# Dependency-direction guard for the packages split out of internal/agent
# (_Docs/81): none of them may import internal/agent back, or the split is a
# cycle waiting to happen. Run by scripts/test.sh full.
set -euo pipefail
cd "$(dirname "$0")/.."
status=0
for pkg in climcp trajectory mcp/repair flows; do
  if go list -deps "./internal/$pkg" | grep -qx 'github.com/bilal-arikan/tionharness/internal/agent'; then
    echo "depcheck: internal/$pkg imports internal/agent (forbidden)" >&2
    status=1
  fi
done
[ "$status" -eq 0 ] && echo "depcheck: ok"
exit "$status"

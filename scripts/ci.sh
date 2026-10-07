#!/usr/bin/env bash
# Shared CI gates. Release frontend bundling stays in build-release.sh.
set -euo pipefail
cd "$(dirname "$0")/.."
export TIONHARNESS_ENABLE_SHELL=1

run_backend() {
  # Fresh checkouts need an embed placeholder before the frontend is built.
  mkdir -p internal/web/dist
  touch internal/web/dist/index.html
  go vet ./...
  local commit build_time
  commit=$(git rev-parse --short HEAD)
  build_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  go build -ldflags "-X github.com/bilal-arikan/tionharness/internal/api.BuildCommit=$commit -X github.com/bilal-arikan/tionharness/internal/api.BuildDate=$build_time" ./...
  go test ./... -count=1 -race -timeout 900s
  bash scripts/depcheck.sh
  bash scripts/tests/depcheck_test.sh
}

run_frontend() {
  (
    cd frontend
    npm ci
    npm test
    # A release builds and typechecks once inside build-release.sh after this gate.
    if [ "$1" = build ]; then
      npm run build
    fi
  )
}

case "${1:-}" in
  backend) run_backend ;;
  frontend) run_frontend build ;;
  release)
    run_backend
    run_frontend release
    ;;
  *)
    echo "usage: scripts/ci.sh [backend|frontend|release]" >&2
    exit 2
    ;;
esac
git diff --check

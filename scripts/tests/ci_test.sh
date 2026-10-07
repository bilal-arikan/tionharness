#!/usr/bin/env bash
# Verify gate ordering and release install ownership without builds, installs or publishing.
set -euo pipefail
source_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_parent=$(CDPATH= cd -- "${TMPDIR:-/tmp}" && pwd)
test_dir=$(mktemp -d "$test_parent/tionharness-ci.XXXXXXXX")
test_dir=$(CDPATH= cd -- "$test_dir" && pwd)
case "$test_dir" in
  "$test_parent"/tionharness-ci.*) ;;
  *) echo "CI test: unsafe temporary directory" >&2; exit 1 ;;
esac
cleanup() {
  local exit_code=$?
  if [ "$exit_code" -ne 0 ] && [ -f "$test_dir/output" ]; then
    cat "$test_dir/output" >&2
  fi
  rm -rf -- "$test_dir"
}
trap cleanup EXIT
export CI_TEST_REPO="$test_dir/repo"
export CI_TEST_LOG="$test_dir/commands"
mkdir -p "$CI_TEST_REPO/scripts/tests" "$CI_TEST_REPO/frontend" "$test_dir/bin"
cp "$source_root/scripts/ci.sh" "$source_root/scripts/depcheck.sh" "$source_root/scripts/build-release.sh" "$CI_TEST_REPO/scripts/"
cp "$source_root/scripts/tests/depcheck_test.sh" "$CI_TEST_REPO/scripts/tests/"

cat >"$test_dir/bin/git" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  'rev-parse --show-toplevel') echo "$CI_TEST_REPO" ;;
  'rev-parse --short HEAD') echo deadbeef ;;
  'diff --check') ;;
  *) echo "Unexpected git command: $*" >&2; exit 1 ;;
esac
EOF
cat >"$test_dir/bin/npm" <<'EOF'
#!/usr/bin/env bash
echo "npm $*" >>"$CI_TEST_LOG"
if [ "$*" = 'run build' ]; then
  mkdir -p "$CI_TEST_REPO/internal/web/dist"
  echo frontend >"$CI_TEST_REPO/internal/web/dist/index.html"
fi
EOF
cat >"$test_dir/bin/go" <<'EOF'
#!/usr/bin/env bash
echo "go $*" >>"$CI_TEST_LOG"
case "$1" in
  vet|test) ;;
  list)
    if [ "${CI_TEST_LIST_FAIL:-0}" = 1 ]; then exit 7; fi
    echo github.com/bilal-arikan/tionharness/internal/climcp
    ;;
  build)
    while (($#)); do
      if [ "$1" = -o ]; then echo binary >"$2"; break; fi
      shift
    done
    ;;
  run) echo archive >"$4" ;;
  *) echo "Unexpected go command: $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$test_dir/bin/git" "$test_dir/bin/npm" "$test_dir/bin/go"
export PATH="$test_dir/bin:$PATH"

assert_count() {
  local command=$1 expected=$2 actual
  actual=$(grep -Fxc "$command" "$CI_TEST_LOG" || true)
  if [ "$actual" -ne "$expected" ]; then
    cat "$CI_TEST_LOG" >&2
    echo "CI test: $command occurred $actual times, expected $expected" >&2
    exit 1
  fi
}

: >"$CI_TEST_LOG"
bash "$CI_TEST_REPO/scripts/ci.sh" release >"$test_dir/output" 2>&1
bash "$CI_TEST_REPO/scripts/build-release.sh" --skip-install 1.2.3 >>"$test_dir/output" 2>&1
assert_count 'npm ci' 1
assert_count 'npm test' 1
assert_count 'npm run build' 1
assert_count 'go vet ./...' 1
assert_count 'go test ./... -count=1 -race -timeout 900s' 1

: >"$CI_TEST_LOG"
bash "$CI_TEST_REPO/scripts/build-release.sh" 1.2.4 >"$test_dir/output" 2>&1
assert_count 'npm ci' 1
assert_count 'npm run build' 1

: >"$CI_TEST_LOG"
bash "$CI_TEST_REPO/scripts/ci.sh" frontend >"$test_dir/output" 2>&1
assert_count 'npm ci' 1
assert_count 'npm test' 1
assert_count 'npm run build' 1

: >"$CI_TEST_LOG"
if CI_TEST_LIST_FAIL=1 bash "$CI_TEST_REPO/scripts/ci.sh" release >"$test_dir/output" 2>&1; then
  echo "CI test: a dependency discovery failure unexpectedly passed the release gate" >&2
  exit 1
fi
assert_count 'npm ci' 0
echo "CI and release install regression checks: ok"

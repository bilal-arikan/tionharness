#!/usr/bin/env bash
# Dependency checks must distinguish forbidden imports from failed discovery.
set -euo pipefail
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_dir=$(mktemp -d)
mkdir "$test_dir/no-tools"
trap 'rm -f "$test_dir/go" "$test_dir/output"; rmdir "$test_dir/no-tools" "$test_dir"' EXIT

cat >"$test_dir/go" <<'EOF'
#!/usr/bin/env bash
case "$DEPCHECK_TEST_MODE" in
  allowed) echo github.com/bilal-arikan/tionharness/internal/climcp ;;
  forbidden) echo github.com/bilal-arikan/tionharness/internal/agent ;;
  failed) echo "synthetic go list failure" >&2; exit 7 ;;
  empty) ;;
esac
EOF
chmod +x "$test_dir/go"

check_case() {
  local mode=$1 expected=$2 exit_code
  if DEPCHECK_TEST_MODE="$mode" PATH="$test_dir:$PATH" bash "$repo_root/scripts/depcheck.sh" >"$test_dir/output" 2>&1; then
    exit_code=0
  else
    exit_code=$?
  fi
  if [ "$exit_code" -ne "$expected" ]; then
    cat "$test_dir/output" >&2
    echo "depcheck test: $mode returned $exit_code, expected $expected" >&2
    exit 1
  fi
}

check_case allowed 0
check_case forbidden 1
check_case failed 1
check_case empty 1
if PATH="$test_dir/no-tools" "$BASH" "$repo_root/scripts/depcheck.sh" >"$test_dir/output" 2>&1; then
  echo "depcheck test: missing Go unexpectedly passed" >&2
  exit 1
fi
grep -q 'required tool not found: go' "$test_dir/output"
echo "depcheck regression checks: ok"

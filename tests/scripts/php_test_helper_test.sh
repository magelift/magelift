#!/usr/bin/env bash
# Prove container context and failure propagation without Docker/network calls.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/docker" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$PHP_TEST_CALLS"
exit "${PHP_TEST_EXIT:-0}"
STUB
chmod +x "$TMP/docker"
export PHP_TEST_CALLS="$TMP/calls"
export MAGELIFT_PHP_TEST_CONTAINER=1
BASH_ENV='' PATH="$TMP:$PATH" bash "$ROOT/scripts/php-test.sh" >/dev/null
expected=5
[[ -f "$ROOT/build/vendor/autoload.php" ]] || expected=6
[[ "$(wc -l < "$PHP_TEST_CALLS")" -eq "$expected" ]]
grep -F -- "$ROOT/images:/workspace/images:ro" "$PHP_TEST_CALLS" >/dev/null
grep -F -- '-w /workspace/build' "$PHP_TEST_CALLS" >/dev/null
: > "$PHP_TEST_CALLS"
status=0
PHP_TEST_EXIT=19 BASH_ENV='' PATH="$TMP:$PATH" bash "$ROOT/scripts/php-test.sh" >/dev/null 2>&1 || status=$?
[[ "$status" -eq 19 && "$(wc -l < "$PHP_TEST_CALLS")" -eq 1 ]]
printf 'php_test_helper_test OK\n'

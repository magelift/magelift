#!/usr/bin/env bash
# Nominal/failure regression for scripts/go-test-modules.sh exit status.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/go-test-modules.sh"

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

[[ -x "$SCRIPT" ]] || fail "missing executable $SCRIPT"

list="$(bash "$SCRIPT" --list-modules)"
printf '%s\n' "$list" | grep -qx root || fail "--list-modules must include root"
printf '%s\n' "$list" | grep -qx sdk || fail "--list-modules must include sdk"
printf '%s\n' "$list" | grep -qx gcp || fail "--list-modules must include gcp"

set +e
bash "$SCRIPT" --only definitely-not-a-module >/dev/null 2>&1
bad_only_rc=$?
set -e
if [[ "$bad_only_rc" -ne 2 ]]; then
  fail "unknown --only module must exit 2, got $bad_only_rc"
fi

set +e
bash "$SCRIPT" --not-a-flag >/dev/null 2>&1
usage_rc=$?
set -e
if [[ "$usage_rc" -ne 2 ]]; then
  fail "invalid flag must exit 2, got $usage_rc"
fi

FAIL_GO="$ROOT/tests/acceptance/fixtures/go-fail-9.sh"
OK_GO="$ROOT/tests/acceptance/fixtures/go-ok.sh"
chmod +x "$FAIL_GO" "$OK_GO"

set +e
MAGELIFT_GO_TEST_WRAPPER="$FAIL_GO" bash "$SCRIPT" --only root >/dev/null 2>&1
wrap_rc=$?
set -e
if [[ "$wrap_rc" -eq 0 ]]; then
  fail "wrapper failure must not succeed, got $wrap_rc"
fi

set +e
MAGELIFT_GO_TEST_WRAPPER="$OK_GO" bash "$SCRIPT" --only root >/dev/null 2>&1
ok_rc=$?
set -e
if [[ "$ok_rc" -ne 0 ]]; then
  fail "wrapper success must exit 0, got $ok_rc"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/go-community" <<'STUB'
#!/usr/bin/env bash
if [[ "$1" == list ]]; then
  printf '%s\n' example/internal/cli example/internal/cloud/kube/runtime example/internal/cloud/aws/runtime example/internal/cloud/ovh/stack example/internal/cloud/scaleway/stack example/internal/external/fastly example/cmd/aws-recovery-acceptance
  exit "${COMMUNITY_LIST_EXIT:-0}"
fi
printf '%s\n' "$@" > "$COMMUNITY_TEST_ARGS"
STUB
chmod +x "$TMP/go-community"
export COMMUNITY_TEST_ARGS="$TMP/args"
MAGELIFT_GO_TEST_WRAPPER="$TMP/go-community" bash "$SCRIPT" --community --only root >/dev/null
grep -Fx 'example/internal/cli' "$COMMUNITY_TEST_ARGS" >/dev/null
grep -Fx 'example/internal/cloud/kube/runtime' "$COMMUNITY_TEST_ARGS" >/dev/null
if grep -E '/(aws|ovh|scaleway|external)/|/cmd/aws-' "$COMMUNITY_TEST_ARGS" >/dev/null; then
  fail 'community gate executed frozen-provider/vendor suites'
fi
status=0
COMMUNITY_LIST_EXIT=51 MAGELIFT_GO_TEST_WRAPPER="$TMP/go-community" bash "$SCRIPT" --community --only root >/dev/null 2>&1 || status=$?
[[ "$status" -ne 0 ]] || fail 'failed package discovery was hidden'

printf 'go_test_modules_test OK\n'

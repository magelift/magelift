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
printf '%s\n' "$list" | grep -qx aws || fail "--list-modules must include aws"
printf '%s\n' "$list" | grep -qx gcp || fail "--list-modules must include gcp"
printf '%s\n' "$list" | grep -qx azure || fail "--list-modules must include azure"

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

FAIL_GO="$ROOT/tests/scripts/fixtures/go-fail-9.sh"
OK_GO="$ROOT/tests/scripts/fixtures/go-ok.sh"
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

printf 'go_test_modules_test OK\n'

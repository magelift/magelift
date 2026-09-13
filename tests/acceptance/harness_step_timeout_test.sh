#!/usr/bin/env bash
# Offline check that harness steps print progress and die at their ceiling.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STEP="$ROOT/scripts/acceptance/run-harness-step.sh"

fail() {
	printf '%s\n' "$*" >&2
	exit 1
}

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

set +e
bash "$STEP" 10 harness-ok -- true >"$out" 2>&1
ok_rc=$?
set -e
if [[ "$ok_rc" -ne 0 ]]; then
	fail "expected 0 from true, got $ok_rc"
fi
grep -Fq '+ acceptance-harness-test: harness-ok (timeout 10s)' "$out" || fail "missing progress line"

set +e
bash "$STEP" 10 harness-fail -- bash -c 'exit 7' >"$out" 2>&1
fail_rc=$?
set -e
if [[ "$fail_rc" -ne 7 ]]; then
	fail "expected exit 7 to be preserved, got $fail_rc"
fi

set +e
bash "$STEP" 1 harness-hang -- sleep 8 >"$out" 2>&1
hang_rc=$?
set -e
if [[ "$hang_rc" -ne 124 ]]; then
	fail "expected 124 from timeout, got $hang_rc"
fi
grep -Fq 'acceptance-harness-test: harness-hang timed out after 1s' "$out" || fail "missing timeout message"

set +e
bash "$STEP" 1 harness-child -- bash -c 'sleep 8' >"$out" 2>&1
child_rc=$?
set -e
if [[ "$child_rc" -ne 124 ]]; then
	fail "expected 124 from child sleep, got $child_rc"
fi

printf 'harness_step_timeout_test OK\n'

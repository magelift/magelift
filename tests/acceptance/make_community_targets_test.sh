#!/usr/bin/env bash
# Exercise the actual Make target with successful and failing Go commands.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for outcome in ok fail-9; do
  status=0
  MAGELIFT_GO_TEST_WRAPPER="$ROOT/tests/acceptance/fixtures/go-${outcome}.sh" \
    make --no-print-directory -C "$ROOT" test-fast >/dev/null 2>&1 || status=$?
  if [[ "$outcome" == ok && "$status" != 0 ]] || [[ "$outcome" == fail-9 && "$status" == 0 ]]; then
    printf 'test-fast with %s returned unexpected exit %s\n' "$outcome" "$status" >&2
    exit 1
  fi
done
printf 'make_community_targets_test OK\n'

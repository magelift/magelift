#!/usr/bin/env bash
# Bounded go test -race suites for local/verify-community use. Full monorepo
# race (go test -race ./...) remains available as test-race-full.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=go-memlimit.sh
source "$ROOT/scripts/go-memlimit.sh"
magelift_apply_go_memlimit
cd "$ROOT"

run() {
  printf '+ test-race: %s\n' "$*"
  "$@"
}

status=0

if ! run go test -race ./internal/lintcoverage/ ./internal/usererr/ -count=1; then
  status=1
fi
if ! run go test -race ./internal/providerhost/... -count=1; then
  status=1
fi
if ! run go test -race ./internal/cli/ ./internal/platform/ -count=1; then
  status=1
fi
if ! run bash "$ROOT/scripts/go-test-modules.sh" --race --only sdk; then
  status=1
fi
if ! (cd "$ROOT/providers/gcp" && run go test -race ./operations/ -count=1); then
  status=1
fi

exit "$status"

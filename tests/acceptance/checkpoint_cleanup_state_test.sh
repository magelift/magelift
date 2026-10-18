#!/usr/bin/env bash
# Offline checkpoint contract: cleanup truth is finalized atomically after the
# provider-owned delete/inventory phase without losing the cell metadata.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/checkpoint.json"
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"

acceptance_checkpoint_ensure
export MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS=502
export MAGELIFT_ACCEPTANCE_CLEANUP_STATE=pending
record_cell "database:mysql" PASS

jq -e '.cells["database:mysql"].durationSeconds == 502 and .cells["database:mysql"].cleanupState == "pending"' "$ACCEPTANCE_CHECKPOINT" >/dev/null
update_cell_cleanup_state "database:mysql" complete
jq -e '.cells["database:mysql"].durationSeconds == 502 and .cells["database:mysql"].cleanupState == "complete"' "$ACCEPTANCE_CHECKPOINT" >/dev/null

if update_cell_cleanup_state "missing" complete; then
	printf 'unrecorded cell cleanup update must fail\n' >&2
	exit 1
fi
if update_cell_cleanup_state "database:mysql" invalid; then
	printf 'invalid cleanup state must fail\n' >&2
	exit 1
fi

printf 'checkpoint_cleanup_state_test OK\n'

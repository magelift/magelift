#!/usr/bin/env bash
# A retained stack must not reuse PASS cells from a different configuration.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/checkpoint.json"
export ACCEPTANCE_CHECKPOINT_FINGERPRINT="release-2.4.8-digest-a"
# shellcheck source=../../scripts/acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"

acceptance_checkpoint_ensure
record_cell "deploy:candidate" PASS

if ! jq -e '.version == 4 and .cells["deploy:candidate"].durationSeconds == 0 and .cells["deploy:candidate"].sessionMode == "baseline" and .cells["deploy:candidate"].fingerprint == "release-2.4.8-digest-a"' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
	printf 'checkpoint did not retain timing and session fingerprint metadata\n' >&2
	exit 1
fi

export ACCEPTANCE_CHECKPOINT_FINGERPRINT="release-2.4.9-digest-b"
acceptance_checkpoint_load
if cell_done "deploy:candidate"; then
	printf 'checkpoint PASS leaked across configuration fingerprints\n' >&2
	exit 1
fi

if ! jq -e --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" '.fingerprint == $fingerprint and (.cells | length == 0)' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
	printf 'checkpoint was not reset to the new fingerprint\n' >&2
	exit 1
fi

if ! compgen -G "$TMP/checkpoint.stale-*.json" >/dev/null; then
	printf 'previous checkpoint was not retained as stale evidence\n' >&2
	exit 1
fi

export ACCEPTANCE_CHECKPOINT="$TMP/empty-keep.json"
printf '%s\n' '{"version":4,"cells":{},"runId":"run-keep-1"}' >"$ACCEPTANCE_CHECKPOINT"
export ACCEPTANCE_CHECKPOINT_FINGERPRINT="empty-stamp"
acceptance_checkpoint_ensure
if ! jq -e --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" '.fingerprint == $fingerprint and .runId == "run-keep-1" and (.cells | length == 0)' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
	printf 'empty KEEP checkpoint must stamp the fingerprint without dropping the run id\n' >&2
	exit 1
fi
if compgen -G "$TMP/empty-keep.stale-*.json" >/dev/null; then
	printf 'empty KEEP checkpoint must not be moved to stale evidence\n' >&2
	exit 1
fi

printf 'checkpoint_fingerprint_test OK\n'

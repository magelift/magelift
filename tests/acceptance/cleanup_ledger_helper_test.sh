#!/usr/bin/env bash
# Offline contract for the restartable cleanup ledger helper.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/acceptance/lib-cleanup-ledger.sh
source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export MAGELIFT_CLEANUP_LEDGER_DIR="$TMP"
export MAGELIFT_CLEANUP_RUN_ID="run-helper"
export MAGELIFT_CLEANUP_MARKER="magelift/scaleway/rdb-recovery/run-helper"
export MAGELIFT_CLEANUP_PROVIDER=scaleway
export MAGELIFT_CLEANUP_REGION=fr-par
export MAGELIFT_CLEANUP_PROJECT=11111111-1111-1111-1111-111111111111
export MAGELIFT_CLEANUP_PROFILE=default

path="$(acceptance_cleanup_ledger_path "$MAGELIFT_CLEANUP_RUN_ID")"
acceptance_cleanup_ledger_claim "$path" rdb-instance source magelift-rdb-run-helper 30
jq -e '.resources[0].status == "intended" and .resources[0].rank == 30' "$path" >/dev/null
acceptance_cleanup_ledger_record "$path" rdb-instance magelift-rdb-run-helper instance-1
jq -e '.resources[0].status == "claimed" and .resources[0].identity == "instance-1"' "$path" >/dev/null
if acceptance_cleanup_ledger_record "$path" rdb-instance missing-name instance-2; then
	printf 'missing claim was recorded\n' >&2
	exit 1
fi

printf 'cleanup_ledger_helper_test OK\n'

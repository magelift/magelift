#!/usr/bin/env bash
# Offline ACCEPT-02: seed checkpoint with cell 1 done; dry-run must skip it and run cell 2+.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/acceptance-checkpoint.json"
export ACCEPTANCE_EVIDENCE="$TMP/matrix-results.md"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$ROOT/scripts/acceptance/cells-aws-preview.txt"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-account
export MAGELIFT_ACCEPTANCE_PROVIDER=aws

# shellcheck source=../../scripts/acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"

CELLS=()
while IFS= read -r line || [[ -n "$line" ]]; do
	[[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
	CELLS+=("$line")
done <"$MAGELIFT_ACCEPTANCE_CELL_CATALOG"
if [[ ${#CELLS[@]} -lt 2 ]]; then
	printf 'catalog needs ≥2 cells\n' >&2
	exit 1
fi

CELL1="${CELLS[0]}"
CELL2="${CELLS[1]}"

acceptance_checkpoint_ensure
record_cell "$CELL1" "PASS"

LOG="$TMP/harness.log"
set +e
bash "$ROOT/scripts/aws-acceptance-local.sh" >"$LOG" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
	printf 'harness exited %s\n' "$rc" >&2
	cat "$LOG" >&2
	exit 1
fi

if ! grep -q "acceptance skip cell=${CELL1}" "$LOG"; then
	printf 'expected skip for %s\n' "$CELL1" >&2
	cat "$LOG" >&2
	exit 1
fi

if ! grep -q "acceptance cell-update cell=${CELL2}" "$LOG"; then
	printf 'expected cell-update for %s\n' "$CELL2" >&2
	cat "$LOG" >&2
	exit 1
fi

if grep -q "acceptance cell-update cell=${CELL1}" "$LOG"; then
	printf 'cell 1 must not be re-executed\n' >&2
	cat "$LOG" >&2
	exit 1
fi

if grep -Eq '\+ magelift (preview|promote|deploy|destroy)|aws (ec2|rds|ecs|elasticache|elbv2|logs) .*(create|delete|run-instances)' "$LOG"; then
	printf 'dry-run must not invoke live mutate APIs\n' >&2
	cat "$LOG" >&2
	exit 1
fi

printf 'checkpoint_resume_test OK skip=%s ran=%s\n' "$CELL1" "$CELL2"

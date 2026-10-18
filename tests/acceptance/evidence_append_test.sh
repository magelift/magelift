#!/usr/bin/env bash
# Offline ACCEPT-01/03: evidence columns + multi-cell create-once log contract.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_EVIDENCE="$TMP/matrix-results.md"
export ACCEPTANCE_CHECKPOINT="$TMP/acceptance-checkpoint.json"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$ROOT/scripts/acceptance/cells-aws-preview.txt"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-account
export MAGELIFT_ACCEPTANCE_PROVIDER=aws

# shellcheck source=../../scripts/acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"

# --- append_row: six columns, header once, two data rows ---
append_row "queueMode:db" "PASS" "1s" "aws" "acct-a" "2026-07-28"
append_row "queueMode:ecs-rabbitmq" "PASS" "2s" "aws" "acct-a" "2026-07-28"

header_count=$(grep -c '^| cell | result | duration | provider | account | date |$' "$ACCEPTANCE_EVIDENCE" || true)
if [[ "$header_count" -ne 1 ]]; then
	printf 'expected exactly one header row, got %s\n' "$header_count" >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi

data_rows=$(grep -E '^\| queueMode:' "$ACCEPTANCE_EVIDENCE" | wc -l | tr -d ' ')
if [[ "$data_rows" -ne 2 ]]; then
	printf 'expected two data rows, got %s\n' "$data_rows" >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi

row1=$(grep 'queueMode:db' "$ACCEPTANCE_EVIDENCE")
# Markdown table row: "| c1 | c2 | c3 | c4 | c5 | c6 |" → awk NF=8
nf=$(printf '%s\n' "$row1" | awk -F'|' '{print NF}')
if [[ "$nf" -ne 8 ]]; then
	printf 'expected 6 columns (awk NF=8), got NF=%s row=%s\n' "$nf" "$row1" >&2
	exit 1
fi
printf '%s\n' "$row1" | grep -q '| queueMode:db | PASS | 1s | aws | acct-a | 2026-07-28 |' || {
	printf 'row missing required six-column values: %s\n' "$row1" >&2
	exit 1
}

printf 'evidence_append columns OK\n'

# --- Dry-run ≥3 cells: one create-once, ≥2 cell-update, zero destroy-between-cells ---
rm -f "$ACCEPTANCE_CHECKPOINT" "$ACCEPTANCE_EVIDENCE"
LOG="$TMP/multi-cell.log"
set +e
bash "$ROOT/scripts/aws-acceptance-local.sh" >"$LOG" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
	printf 'multi-cell dry-run exited %s\n' "$rc" >&2
	cat "$LOG" >&2
	exit 1
fi

create_once=$(grep -c 'acceptance create-once' "$LOG" || true)
updates=$(grep -c 'acceptance cell-update' "$LOG" || true)
destroys=$(grep -cE 'acceptance destroy-between|magelift destroy|\+ magelift destroy' "$LOG" || true)

if [[ "$create_once" -ne 1 ]]; then
	printf 'expected exactly one create-once, got %s\n' "$create_once" >&2
	cat "$LOG" >&2
	exit 1
fi
if [[ "$updates" -lt 2 ]]; then
	printf 'expected ≥2 cell-update markers, got %s\n' "$updates" >&2
	cat "$LOG" >&2
	exit 1
fi
if [[ "$destroys" -ne 0 ]]; then
	printf 'destroy-between-cells must be zero, got %s\n' "$destroys" >&2
	cat "$LOG" >&2
	exit 1
fi

cell_count=$(grep -v '^[[:space:]]*#' "$MAGELIFT_ACCEPTANCE_CELL_CATALOG" | grep -cv '^[[:space:]]*$' || true)
if [[ "$cell_count" -lt 3 ]]; then
	printf 'catalog must list ≥3 cells, got %s\n' "$cell_count" >&2
	exit 1
fi
if [[ "$updates" -ne "$cell_count" ]]; then
	printf 'expected cell-update count=%s matching catalog, got %s\n' "$cell_count" "$updates" >&2
	cat "$LOG" >&2
	exit 1
fi

printf 'evidence_append_test OK create-once=%s updates=%s\n' "$create_once" "$updates"

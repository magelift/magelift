#!/usr/bin/env bash
# Offline ACCEPT-05: GCP harness shares checkpoint/evidence; expanded catalog;
# live_cell_loop + force_clean/assert_clean shape present; dry-run never ups.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-acceptance-local.sh"
CATALOG="$ROOT/scripts/acceptance/cells-gcp-preview.txt"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/gcp-checkpoint.json"
export ACCEPTANCE_EVIDENCE="$TMP/gcp-matrix-results.md"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$CATALOG"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_PROVIDER=gcp
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-gcp

REQUIRED_CELLS=(
	bootstrap:wif
	composer:sm-write
	composer:sm-read
	day2:secrets
	day2:state
	day2:logs
	day2:exec
	day2:health
	deploy:candidate
	migrate:dump
	cost:estimate
	cutover:dns
)

for cell in "${REQUIRED_CELLS[@]}"; do
	if ! grep -qxF "$cell" "$CATALOG"; then
		printf 'catalog missing required cell: %s\n' "$cell" >&2
		exit 1
	fi
done

# Structural: live EXIT contract + live_cell_loop symbols
for sym in force_clean_orphans assert_clean live_cell_loop run_gcp_cell prove_wif_token_exchange; do
	if ! grep -Eq "^${sym}\\(\\)" "$SCRIPT"; then
		printf 'missing harness symbol: %s\n' "$sym" >&2
		exit 1
	fi
done
if ! grep -q 'PSA\|psa_soak\|PSA_SOAK\|soak' "$SCRIPT"; then
	printf 'missing PSA soak messaging in gcp harness\n' >&2
	exit 1
fi
if ! grep -q 'lib-checkpoint.sh' "$SCRIPT" || ! grep -q 'lib-evidence.sh' "$SCRIPT"; then
	printf 'gcp script must source shared checkpoint/evidence libs\n' >&2
	exit 1
fi
if ! grep -q 'append_row' "$SCRIPT"; then
	printf 'gcp script must call append_row for evidence\n' >&2
	exit 1
fi
# bootstrap:wif must refuse Ensure-only
if ! grep -q 'ENSURE_ONLY\|Ensure-only\|not Ensure-only\|Act/STS\|token exchange' "$SCRIPT"; then
	printf 'bootstrap:wif must require Act/STS proof (not Ensure-only)\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_DIGEST' "$SCRIPT"; then
	printf 'DIGEST pullable documentation gate missing\n' >&2
	exit 1
fi
# cleanup path: destroy then force_clean then assert_clean (live EXIT contract)
cleanup_block=$(awk '/^cleanup\(\)/,/^}/' "$SCRIPT")
if ! printf '%s\n' "$cleanup_block" | grep -q 'destroy'; then
	printf 'EXIT cleanup must call destroy when created\n' >&2
	exit 1
fi
if ! printf '%s\n' "$cleanup_block" | grep -q 'force_clean_orphans'; then
	printf 'EXIT cleanup must call force_clean_orphans\n' >&2
	exit 1
fi
if ! printf '%s\n' "$cleanup_block" | grep -q 'assert_clean'; then
	printf 'EXIT cleanup must call assert_clean\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_KEEP' "$SCRIPT"; then
	printf 'KEEP gate missing from gcp harness\n' >&2
	exit 1
fi
# Refuse live without gate (T-07-11)
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE' "$SCRIPT"; then
	printf 'MAGELIFT_GCP_ACCEPTANCE refuse gate missing\n' >&2
	exit 1
fi

LOG="$TMP/gcp-dry-run.log"
set +e
bash "$SCRIPT" >"$LOG" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
	printf 'gcp dry-run exited %s\n' "$rc" >&2
	cat "$LOG" >&2
	exit 1
fi

if ! grep -q 'gcp acceptance dry-run ok' "$LOG"; then
	printf 'expected dry-run ok marker\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q 'created=0' "$LOG"; then
	printf 'dry-run must report created=0\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q 'live_cell_loop' "$LOG"; then
	printf 'dry-run should mention live_cell_loop shape marker\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if grep -Eq '\+ magelift (up|destroy)|gcloud .+ create|created=1' "$LOG"; then
	printf 'dry-run must not invoke spending mutate or set created=1\n' >&2
	cat "$LOG" >&2
	exit 1
fi

if [[ ! -f "$ACCEPTANCE_CHECKPOINT" ]]; then
	printf 'checkpoint not written: %s\n' "$ACCEPTANCE_CHECKPOINT" >&2
	exit 1
fi
for cell in "${REQUIRED_CELLS[@]}"; do
	if ! jq -e --arg c "$cell" '.cells[$c]' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
		printf 'checkpoint missing cell: %s\n' "$cell" >&2
		cat "$ACCEPTANCE_CHECKPOINT" >&2
		exit 1
	fi
done
if [[ ! -f "$ACCEPTANCE_EVIDENCE" ]]; then
	printf 'evidence not written\n' >&2
	exit 1
fi
for cell in bootstrap:wif deploy:candidate migrate:dump cutover:dns; do
	if ! grep -q "| ${cell} | PASS |" "$ACCEPTANCE_EVIDENCE"; then
		printf 'evidence row missing for %s\n' "$cell" >&2
		cat "$ACCEPTANCE_EVIDENCE" >&2
		exit 1
	fi
done
if ! grep -q '| gcp |' "$ACCEPTANCE_EVIDENCE"; then
	printf 'evidence provider must be gcp\n' >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi

# Resume: second dry-run must skip completed cells
LOG2="$TMP/gcp-dry-run-resume.log"
set +e
bash "$SCRIPT" >"$LOG2" 2>&1
rc2=$?
set -e
if [[ "$rc2" -ne 0 ]]; then
	printf 'gcp dry-run resume exited %s\n' "$rc2" >&2
	cat "$LOG2" >&2
	exit 1
fi
if ! grep -q 'acceptance skip cell=bootstrap:wif (checkpoint)' "$LOG2"; then
	printf 'resume must skip first complete cell via checkpoint\n' >&2
	cat "$LOG2" >&2
	exit 1
fi

printf 'gcp_harness_shape_test OK\n'

#!/usr/bin/env bash
# Offline ACCEPT-05: GCP harness shares checkpoint/evidence; force_clean/assert_clean shape present; no up.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-acceptance-local.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/gcp-checkpoint.json"
export ACCEPTANCE_EVIDENCE="$TMP/gcp-matrix-results.md"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$ROOT/scripts/acceptance/cells-gcp-preview.txt"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_PROVIDER=gcp
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-gcp

# Structural: live EXIT contract symbols still defined in script body
for sym in force_clean_orphans assert_clean; do
	if ! grep -Eq "^${sym}\\(\\)|^${sym}\\s*\\(\\)" "$SCRIPT" && ! grep -Eq "^${sym}\\(\\) {" "$SCRIPT"; then
		# bash functions: name() {
		if ! grep -Eq "^${sym}\\(\\)" "$SCRIPT"; then
			printf 'missing harness symbol: %s\n' "$sym" >&2
			exit 1
		fi
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
if grep -Eq '\+ magelift (up|destroy)|gcloud .+ create|created=1' "$LOG"; then
	printf 'dry-run must not invoke spending mutate or set created=1\n' >&2
	cat "$LOG" >&2
	exit 1
fi

if [[ ! -f "$ACCEPTANCE_CHECKPOINT" ]]; then
	printf 'checkpoint not written: %s\n' "$ACCEPTANCE_CHECKPOINT" >&2
	exit 1
fi
if ! jq -e '.cells["preset:preview"]' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
	printf 'checkpoint missing preset:preview cell\n' >&2
	cat "$ACCEPTANCE_CHECKPOINT" >&2
	exit 1
fi
if [[ ! -f "$ACCEPTANCE_EVIDENCE" ]]; then
	printf 'evidence not written\n' >&2
	exit 1
fi
if ! grep -q '| preset:preview | PASS |' "$ACCEPTANCE_EVIDENCE"; then
	printf 'evidence row missing\n' >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi
if ! grep -q '| gcp |' "$ACCEPTANCE_EVIDENCE"; then
	printf 'evidence provider must be gcp\n' >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi

printf 'gcp_harness_shape_test OK\n'

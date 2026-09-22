#!/usr/bin/env bash
# Runs the distribution, artifact, and GCP product gates independently.
# One gate FAIL does not stop the others or copy its status onto them.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are explicit
# maintainer steps; this runner never creates, pushes, undrafts, or deletes
# the canary ref.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"

SHA=""
GATE_FILE="${MAGELIFT_DEVLOOP_GATE_FILE:-${MAGELIFT_DEVLOOP_GATES:-.magelift/development-loop/gates.jsonl}}"
EVIDENCE_DIR="${MAGELIFT_DEVLOOP_EVIDENCE_DIR:-.magelift/development-loop}"
FIXTURE="${MAGELIFT_DEVLOOP_FIXTURE:-0}"

usage() {
	sed -n '2,9p' "$0"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--sha)
		SHA="${2:?--sha needs a value}"
		shift 2
		;;
	--gate-file)
		GATE_FILE="${2:?--gate-file needs a value}"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'run-gates: unknown argument %s\n' >&2
		exit 2
		;;
	esac
done

if [[ -z "$SHA" ]]; then
	printf 'run-gates: --sha is required\n' >&2
	exit 2
fi

TAG="$(canary_ref_for_sha "$SHA")" || {
	printf 'run-gates: commit SHA must be 40 lowercase hexadecimal characters\n' >&2
	exit 2
}

mkdir -p "$(dirname "$GATE_FILE")" "$EVIDENCE_DIR"
: >"$GATE_FILE"
export MAGELIFT_DEVLOOP_GATE_FILE="$GATE_FILE"
export MAGELIFT_DEVLOOP_FIXTURE="$FIXTURE"
export MAGELIFT_ACCEPTANCE_DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-1}"
export ACCEPTANCE_SHARED_EVIDENCE="${MAGELIFT_DEVLOOP_SHARED_EVIDENCE:-$EVIDENCE_DIR/acceptance-evidence.jsonl}"
export MAGELIFT_ACCEPTANCE_SOURCE="development-loop"
export MAGELIFT_ACCEPTANCE_RUN_ID="${MAGELIFT_DEVLOOP_RUN_ID:-devloop-${SHA:0:12}-$$}"
export MAGELIFT_ACCEPTANCE_GENERATED_AT="${MAGELIFT_DEVLOOP_GENERATED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

run_one_gate() {
	local gate="$1" script="$2"
	local rc=0
	set +e
	bash "$script" --sha "$SHA" --gate-file "$GATE_FILE"
	rc=$?
	set -e
	return "$rc"
}

failures=0
run_one_gate distribution "$ROOT/scripts/development-loop/distribution-gate.sh" || failures=$((failures + 1))
run_one_gate artifact "$ROOT/scripts/development-loop/artifact-gate.sh" || failures=$((failures + 1))
run_one_gate gcp-product "$ROOT/scripts/development-loop/gcp-product-gate.sh" || failures=$((failures + 1))

write_gate_evidence() {
	local gate="$1" cell="$2"
	local status detail
	status="$(jq -r --arg gate "$gate" 'select(.gate == $gate) | .status' "$GATE_FILE" | tail -1)"
	detail="$(jq -r --arg gate "$gate" 'select(.gate == $gate) | .detail' "$GATE_FILE" | tail -1)"
	case "$status" in
	PASS) append_shared_row "$cell" "PASS" "0s" "gcp" "development-loop" "$(date -u +%Y-%m-%d)" ;;
	FAIL)
		MAGELIFT_ACCEPTANCE_REASON="$detail" append_shared_row "$cell" "FAIL" "0s" "gcp" "development-loop" "$(date -u +%Y-%m-%d)"
		;;
	SKIP)
		MAGELIFT_ACCEPTANCE_REASON="$detail" MAGELIFT_ACCEPTANCE_DRY_RUN=1 append_shared_row "$cell" "PASS" "0s" "gcp" "development-loop" "$(date -u +%Y-%m-%d)"
		;;
	*)
		MAGELIFT_ACCEPTANCE_REASON="missing gate record for ${gate}" append_shared_row "$cell" "FAIL" "0s" "gcp" "development-loop" "$(date -u +%Y-%m-%d)"
		;;
	esac
	unset MAGELIFT_ACCEPTANCE_REASON
}

write_gate_evidence distribution "devloop:distribution:${TAG}"
write_gate_evidence artifact "devloop:artifact:${TAG}"
write_gate_evidence gcp-product "devloop:gcp-product:${TAG}"

if [[ "$failures" -gt 0 ]]; then
	printf 'run-gates: %s gate(s) failed for %s\n' "$failures" "$TAG" >&2
	exit 1
fi
printf 'run-gates: all gates recorded for %s\n' "$TAG"
exit 0

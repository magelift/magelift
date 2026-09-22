#!/usr/bin/env bash
# GCP product gate: delegates to providers/gcp/scripts/gcp-acceptance-local.sh.
# Dry-run never creates cloud resources. When required harness env is missing,
# the gate records SKIP instead of mutating the acceptance project.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are not performed
# by this script.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"

HARNESS="$ROOT/providers/gcp/scripts/gcp-acceptance-local.sh"
SHA=""
GATE_FILE="${MAGELIFT_DEVLOOP_GATE_FILE:-${MAGELIFT_DEVLOOP_GATES:-.magelift/development-loop/gates.jsonl}}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-1}"

usage() {
	sed -n '2,8p' "$0"
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
		printf 'gcp-product-gate: unknown argument %s\n' >&2
		exit 2
		;;
	esac
done

if [[ -z "$SHA" ]]; then
	printf 'gcp-product-gate: --sha is required\n' >&2
	exit 2
fi

if ! canary_ref_for_sha "$SHA" >/dev/null; then
	devloop_append_gate "$GATE_FILE" gcp-product FAIL 'commit SHA must be 40 lowercase hexadecimal characters'
	exit 1
fi

if [[ "$DRY_RUN" != 1 && "$DRY_RUN" != true ]]; then
	if [[ "${MAGELIFT_GCP_ACCEPTANCE:-}" != 1 ]]; then
		devloop_append_gate "$GATE_FILE" gcp-product SKIP 'MAGELIFT_GCP_ACCEPTANCE=1 is required for live product gates'
		exit 0
	fi
	if [[ -z "${MAGELIFT_GCP_PROJECT:-}" ]]; then
		devloop_append_gate "$GATE_FILE" gcp-product SKIP 'MAGELIFT_GCP_PROJECT is required for live product gates'
		exit 0
	fi
fi

missing_tools=()
for cmd in gcloud docker pulumi go kubectl gke-gcloud-auth-plugin curl openssl shasum; do
	if ! command -v "$cmd" >/dev/null 2>&1; then
		missing_tools+=("$cmd")
	fi
done
if [[ "${#missing_tools[@]}" -gt 0 ]]; then
	devloop_append_gate "$GATE_FILE" gcp-product SKIP "missing prerequisite: ${missing_tools[*]}"
	exit 0
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-devloop-gcp.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT INT TERM

export MAGELIFT_ACCEPTANCE_DRY_RUN="$DRY_RUN"
export MAGELIFT_ACCEPTANCE_PROVIDER=gcp
export MAGELIFT_ACCEPTANCE_ACCOUNT="${MAGELIFT_ACCEPTANCE_ACCOUNT:-devloop-gcp}"
export MAGELIFT_GCP_ACCEPTANCE_NAME="${MAGELIFT_GCP_ACCEPTANCE_NAME:-devloop-${SHA:0:12}}"
export ACCEPTANCE_CHECKPOINT="$WORK/gcp-checkpoint.json"
export ACCEPTANCE_EVIDENCE="$WORK/gcp-matrix-results.md"
export ACCEPTANCE_SHARED_EVIDENCE="$WORK/acceptance-evidence.jsonl"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}"
export MAGELIFT_GCP_ACCEPTANCE_DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_DEVLOOP_ARTIFACT_DIGEST:-registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}}"

LOG="$WORK/gcp-product.log"
set +e
bash "$HARNESS" >"$LOG" 2>&1
rc=$?
set -e

if [[ "$rc" -ne 0 ]]; then
	devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp-acceptance-local.sh dry-run failed'
	exit 1
fi
if ! grep -q 'gcp acceptance dry-run ok' "$LOG"; then
	devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp dry-run marker missing'
	exit 1
fi
if grep -Eq '\+ magelift (up|destroy)|gcloud .+ create|created=1' "$LOG"; then
	devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp dry-run performed a mutating operation'
	exit 1
fi
devloop_append_gate "$GATE_FILE" gcp-product PASS ''
exit 0

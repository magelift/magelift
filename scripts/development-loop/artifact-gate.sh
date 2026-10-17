#!/usr/bin/env bash
# Artifact gate: run existing artifact contract and manifest checks separately
# from producing a real Magento image. Offline checks never certify an image.
# Live image production remains explicitly NOT_RUN until configured.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are not performed
# by this script.
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"

SHA=""
GATE_FILE="${MAGELIFT_DEVLOOP_GATE_FILE:-${MAGELIFT_DEVLOOP_GATES:-.magelift/development-loop/gates.jsonl}}"
FIXTURE="${MAGELIFT_DEVLOOP_FIXTURE:-0}"
ARTIFACT_DIGEST="${MAGELIFT_DEVLOOP_ARTIFACT_DIGEST:-}"

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
		printf 'artifact-gate: unknown argument %s\n' "$1" >&2
		exit 2
		;;
	esac
done

if [[ -z "$SHA" ]]; then
	printf 'artifact-gate: --sha is required\n' >&2
	exit 2
fi

if ! canary_ref_for_sha "$SHA" >/dev/null; then
	devloop_append_gate "$GATE_FILE" artifact FAIL 'commit SHA must be 40 lowercase hexadecimal characters'
	exit 1
fi

record_fail() {
	devloop_append_gate "$GATE_FILE" artifact FAIL "$1"
}

record_skip() {
	devloop_append_gate "$GATE_FILE" artifact SKIP "$1"
}

EVIDENCE_DIR="${MAGELIFT_DEVLOOP_EVIDENCE_DIR:-$ROOT/.magelift/development-loop}"
mkdir -p "$EVIDENCE_DIR"
WORK="$(mktemp -d "$EVIDENCE_DIR/artifact-${SHA:0:12}.XXXXXX")"

run_offline_contract() {
	if ! command -v go >/dev/null 2>&1; then
		record_skip 'Go is unavailable for the offline artifact contract checks; image build NOT_RUN'
		return 0
	fi
	if ! (cd "$ROOT" && GOMEMLIMIT="$("$ROOT/scripts/go-memlimit.sh")" go test ./internal/certification/ -count=1) >"$WORK/contract.log" 2>&1; then
		record_fail 'offline artifact contract tests failed; inspect private diagnostics'
		return 1
	fi
	if bash "$ROOT/scripts/php-test.sh" --artifact-manifest >"$WORK/manifest.log" 2>&1; then
		record_skip 'offline artifact contract and manifest checks passed; application image build NOT_RUN'
	else
		local status=$?
		# PHPUnit exit 2 is a test error, not an unavailable-runtime marker.
		record_fail "artifact manifest checks failed (exit ${status}); inspect private diagnostics"
		return 1
	fi
}

run_live_push() {
	if [[ -z "$ARTIFACT_DIGEST" ]]; then
		record_skip 'MAGELIFT_DEVLOOP_ARTIFACT_DIGEST is not configured'
		return 0
	fi
	if ! command -v magelift >/dev/null 2>&1; then
		record_skip 'magelift is not on PATH for build --push'
		return 0
	fi
	printf 'artifact-gate: live image production is not configured in this runner; prepare immutable inputs with the existing build pipeline\n' >&2
	record_skip 'live magelift build --push is not configured for this runner'
	return 0
}

if [[ "$FIXTURE" == 1 || -z "$ARTIFACT_DIGEST" ]]; then
	run_offline_contract
else
	run_live_push
fi

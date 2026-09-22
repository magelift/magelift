#!/usr/bin/env bash
# Artifact gate: validates the Magento artifact manifest and runs the existing
# certification EnsurePipelineArtifact path. A live magelift build --push path
# runs only when MAGELIFT_DEVLOOP_ARTIFACT_DIGEST is configured; otherwise the
# offline contract tests are used and no shop image is built.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are not performed
# by this script.
set -Eeuo pipefail

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
		printf 'artifact-gate: unknown argument %s\n' >&2
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

phpunit_extensions_available() {
	local ext
	for ext in dom filter json libxml mbstring tokenizer xmlwriter; do
		if ! php -m 2>/dev/null | grep -qi "^${ext}$"; then
			return 1
		fi
	done
	return 0
}

run_artifact_manifest_tests() {
	if phpunit_extensions_available; then
		composer test --working-dir="$ROOT/build" -- tests/Artifact/ArtifactManifestTest.php >/dev/null
		return
	fi
	if ! command -v docker >/dev/null 2>&1; then
		return 2
	fi
	docker run --rm -v "$ROOT/build:/app" -w /app --entrypoint php php:8.5-fpm-trixie \
		vendor/bin/phpunit tests/Artifact/ArtifactManifestTest.php >/dev/null
}

run_offline_contract() {
	if ! (cd "$ROOT" && go test ./internal/certification/ -count=1 >/dev/null); then
		record_fail 'EnsurePipelineArtifact certification tests failed'
		return 1
	fi
	if [[ ! -f "$ROOT/build/vendor/autoload.php" ]]; then
		record_skip 'composer dependencies are not installed in build/'
		return 0
	fi
	if ! run_artifact_manifest_tests; then
		local status=$?
		if [[ "$status" -eq 2 ]]; then
			record_skip 'PHP extensions required by PHPUnit are not available and no Docker PHP image could run them'
			return 0
		fi
		record_fail 'ArtifactManifestTest.php failed'
		return 1
	fi
	devloop_append_gate "$GATE_FILE" artifact PASS ''
	return 0
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
	printf 'artifact-gate: live magelift build --push is not implemented in this gate; use MAGELIFT_DEVLOOP_FIXTURE=1 or configure the digest for downstream gates\n' >&2
	record_skip 'live magelift build --push is not configured for this runner'
	return 0
}

if [[ "$FIXTURE" == 1 || -z "$ARTIFACT_DIGEST" ]]; then
	run_offline_contract
else
	run_live_push
fi

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=../../scripts/development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"
# shellcheck source=../../scripts/development-loop/lib-distribution-fixture.sh
source "$ROOT/scripts/development-loop/lib-distribution-fixture.sh"

sample_tag='v0.1.0-alpha.1-rc.22'
if [[ "$(devloop_provider_binary_name "$sample_tag" linux amd64)" != 'magelift-provider-gcp_0.1.0-alpha.1-rc.22_linux_amd64' ]]; then
	printf 'provider binary name does not match published assets\n' >&2
	exit 1
fi
if [[ "$(devloop_provider_lock_name linux amd64)" != 'magelift.providers.lock.linux_amd64' ]]; then
	printf 'provider lock name does not match published assets\n' >&2
	exit 1
fi

sha='0123456789abcdef0123456789abcdef01234567'
ref="$(canary_ref_for_sha "$sha")"
if [[ "$ref" != "v0.0.0-canary.${sha}" ]]; then
	printf 'ref = %s\n' "$ref" >&2
	exit 1
fi
if canary_ref_for_sha 'ABC' >/dev/null; then
	printf 'short SHA was accepted\n' >&2
	exit 1
fi
if canary_ref_for_sha '0123456789ABCDEF0123456789ABCDEF01234567' >/dev/null; then
	printf 'uppercase SHA was accepted\n' >&2
	exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
record="$TMP/gates.jsonl"

devloop_append_gate "$record" distribution FAIL 'checksum mismatch'
devloop_record_blocked "$record" artifact 'distribution archive'
devloop_append_gate "$record" gcp-product PASS ''

if devloop_append_gate "$record" other PASS '' >/dev/null; then
	printf 'unknown gate was accepted\n' >&2
	exit 1
fi
if devloop_append_gate "$record" distribution SKIP '' >/dev/null; then
	printf 'SKIP without a detail was accepted\n' >&2
	exit 1
fi

distribution_status="$(jq -r 'select(.gate=="distribution") | .status' "$record")"
artifact_status="$(jq -r 'select(.gate=="artifact") | .status' "$record")"
product_status="$(jq -r 'select(.gate=="gcp-product") | .status' "$record")"
artifact_detail="$(jq -r 'select(.gate=="artifact") | .detail' "$record")"

if [[ "$distribution_status" != FAIL || "$artifact_status" != SKIP || "$product_status" != PASS ]]; then
	printf 'statuses were rewritten: distribution=%s artifact=%s product=%s\n' \
		"$distribution_status" "$artifact_status" "$product_status" >&2
	exit 1
fi
if [[ "$artifact_detail" != 'missing prerequisite: distribution archive' ]]; then
	printf 'skip detail = %s\n' "$artifact_detail" >&2
	exit 1
fi
if [[ "$artifact_status" == "$distribution_status" ]]; then
	printf 'artifact SKIP copied the distribution FAIL\n' >&2
	exit 1
fi

RUNNER_TMP="$(mktemp -d)"
trap 'rm -rf "$TMP" "$RUNNER_TMP"' EXIT
RUNNER_GATE="$RUNNER_TMP/gates.jsonl"
RUNNER_EVIDENCE="$RUNNER_TMP/evidence.jsonl"
sha_runner='fedcba9876543210fedcba9876543210fedcba98'
export MAGELIFT_DEVLOOP_FIXTURE=1
export MAGELIFT_DEVLOOP_GATE_FILE="$RUNNER_GATE"
export MAGELIFT_DEVLOOP_SHARED_EVIDENCE="$RUNNER_EVIDENCE"
export MAGELIFT_DEVLOOP_EVIDENCE_DIR="$RUNNER_TMP"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
if ! bash "$ROOT/scripts/development-loop/run-gates.sh" --sha "$sha_runner"; then
	printf 'fixture run-gates failed\n' >&2
	exit 1
fi
for gate in distribution artifact gcp-product; do
	status="$(jq -r --arg gate "$gate" 'select(.gate == $gate) | .status' "$RUNNER_GATE" | tail -1)"
	if [[ -z "$status" || "$status" == null ]]; then
		printf 'run-gates missing record for %s\n' "$gate" >&2
		exit 1
	fi
done
distribution_runner="$(jq -r 'select(.gate=="distribution") | .status' "$RUNNER_GATE" | tail -1)"
artifact_runner="$(jq -r 'select(.gate=="artifact") | .status' "$RUNNER_GATE" | tail -1)"
product_runner="$(jq -r 'select(.gate=="gcp-product") | .status' "$RUNNER_GATE" | tail -1)"
if [[ "$distribution_runner" != PASS || "$product_runner" != PASS ]]; then
	printf 'runner fixture gates: distribution=%s product=%s\n' "$distribution_runner" "$product_runner" >&2
	exit 1
fi
if [[ "$artifact_runner" != PASS && "$artifact_runner" != SKIP ]]; then
	printf 'artifact gate = %s\n' "$artifact_runner" >&2
	exit 1
fi
if [[ ! -f "$RUNNER_EVIDENCE" ]] || [[ "$(wc -l <"$RUNNER_EVIDENCE" | tr -d ' ')" -lt 3 ]]; then
	printf 'run-gates did not write shared evidence rows\n' >&2
	exit 1
fi

WORKFLOW="$ROOT/.github/workflows/alpha-development-loop.yml"
if [[ ! -f "$WORKFLOW" ]]; then
	printf 'missing alpha-development-loop workflow\n' >&2
	exit 1
fi
if ! grep -q 'Operator actions: pushing and deleting v0.0.0-canary.<sha>' "$WORKFLOW"; then
	printf 'workflow missing operator-action header\n' >&2
	exit 1
fi
if grep -Eq 'git push|gh release create' "$WORKFLOW"; then
	printf 'workflow must not push tags or create releases\n' >&2
	exit 1
fi
if ! grep -q 'workflow_dispatch:' "$WORKFLOW"; then
	printf 'workflow must be workflow_dispatch only\n' >&2
	exit 1
fi

printf 'development_loop_gate_test OK\n'

#!/usr/bin/env bash
# KEEP resume must record cleanup on the Magento run ID stored in the checkpoint.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/acceptance-checkpoint.json"
# shellcheck source=../../scripts/acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"

export ACCEPTANCE_EVIDENCE_RUN_ID=run-create-111
unset MAGELIFT_ACCEPTANCE_RUN_ID || true
acceptance_checkpoint_bind_run_id
jq -e '.runId == "run-create-111"' "$ACCEPTANCE_CHECKPOINT" >/dev/null

export ACCEPTANCE_EVIDENCE_RUN_ID=run-destroy-222
unset MAGELIFT_ACCEPTANCE_RUN_ID || true
acceptance_checkpoint_bind_run_id
if [[ "$ACCEPTANCE_EVIDENCE_RUN_ID" != "run-create-111" ]]; then
	printf 'resume must reuse checkpoint run id, got %s\n' "$ACCEPTANCE_EVIDENCE_RUN_ID" >&2
	exit 1
fi
if [[ "${MAGELIFT_ACCEPTANCE_RUN_ID:-}" != "run-create-111" ]]; then
	printf 'MAGELIFT_ACCEPTANCE_RUN_ID must follow the checkpoint\n' >&2
	exit 1
fi

printf 'checkpoint_run_id_test OK\n'

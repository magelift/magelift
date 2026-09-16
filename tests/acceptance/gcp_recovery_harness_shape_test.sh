#!/usr/bin/env bash
# Offline guard for the disposable Cloud Storage recovery cell destination
# and durable-class flags. Isolated media remains the default.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/providers/gcp/scripts/gcp-recovery-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_RECOVERY_DESTINATION' "$SCRIPT" || {
	printf 'Cloud Storage harness must honor MAGELIFT_GCP_RECOVERY_DESTINATION\n' >&2
	exit 1
}
grep -Fq 'MAGELIFT_GCP_RECOVERY_DATA_CLASS' "$SCRIPT" || {
	printf 'Cloud Storage harness must honor MAGELIFT_GCP_RECOVERY_DATA_CLASS\n' >&2
	exit 1
}
grep -Fq -- '--destination "$destination"' "$SCRIPT" || {
	printf 'Cloud Storage harness must pass isolated or in-place destination to the recovery command\n' >&2
	exit 1
}
grep -Fq -- '--data-class "$data_class"' "$SCRIPT" || {
	printf 'Cloud Storage harness must pass the durable object class to the recovery command\n' >&2
	exit 1
}

printf 'gcp_recovery_harness_shape_test OK\n'

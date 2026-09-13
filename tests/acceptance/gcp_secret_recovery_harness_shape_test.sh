#!/usr/bin/env bash
# Offline guard for the disposable Secret Manager recovery cell destination
# flag. Isolated remains the default; in-place is opt-in.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-secret-recovery-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_SECRET_RECOVERY_DESTINATION' "$SCRIPT" || {
	printf 'Secret Manager harness must honor MAGELIFT_GCP_SECRET_RECOVERY_DESTINATION\n' >&2
	exit 1
}
grep -Fq -- '--destination "$destination"' "$SCRIPT" || {
	printf 'Secret Manager harness must pass isolated or in-place destination to the recovery command\n' >&2
	exit 1
}

printf 'gcp_secret_recovery_harness_shape_test OK\n'

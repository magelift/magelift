#!/usr/bin/env bash
# Offline guard for the disposable Cloud SQL recovery cell. The source
# instance name must be claimed before the create request so a provider-side
# accepted-but-erroring request cannot escape the cleanup trap.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/providers/gcp/scripts/gcp-cloudsql-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT" || {
	printf 'Cloud SQL harness must source the shared dependency preflight\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_jq' "$SCRIPT" || {
	printf 'Cloud SQL harness must preflight jq before provider mutation\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_commands gcloud curl go shasum openssl' "$SCRIPT" || {
	printf 'Cloud SQL harness must preflight all local commands before provider mutation\n' >&2
	exit 1
}
grep -Fq -- '--destination="${destination}"' "$SCRIPT" || {
	printf 'Cloud SQL harness must pass isolated or in-place destination to the recovery command\n' >&2
	exit 1
}
grep -Fq 'MAGELIFT_GCP_CLOUDSQL_DESTINATION' "$SCRIPT" || {
	printf 'Cloud SQL harness must honor MAGELIFT_GCP_CLOUDSQL_DESTINATION\n' >&2
	exit 1
}
grep -Fq 'MAGELIFT_GCP_CLOUDSQL_ROOT_PASSWORD=' "$SCRIPT" || {
	printf 'Cloud SQL harness must pass the generated fixture credential through the environment\n' >&2
	exit 1
}
grep -Fq -- '--authorized-networks="${caller_ipv4}/32"' "$SCRIPT" || {
	printf 'Cloud SQL harness must scope public access to the runner address\n' >&2
	exit 1
}
grep -Fq 'existing_instance_probe=' "$SCRIPT" || {
	printf 'Cloud SQL harness must prove its generated instance name is unused\n' >&2
	exit 1
}
grep -Fq 'could not prove generated Cloud SQL instance name is unused' "$SCRIPT" || {
	printf 'Cloud SQL harness must fail closed on an inconclusive name probe\n' >&2
	exit 1
}
grep -Fq 'instance_claimed=1' "$SCRIPT" || {
	printf 'Cloud SQL harness must claim the source name before create\n' >&2
	exit 1
}
grep -Fq 'gcloud sql instances describe "${instance}"' "$SCRIPT" || {
	printf 'Cloud SQL cleanup must inspect the exact claimed source instance\n' >&2
	exit 1
}
grep -Fq 'gcloud sql instances delete "${instance}"' "$SCRIPT" || {
	printf 'Cloud SQL cleanup must delete the exact claimed source instance\n' >&2
	exit 1
}
grep -Fq 'patch_response=' "$SCRIPT" || {
	printf 'Cloud SQL label patch response must be retained for operation readiness\n' >&2
	exit 1
}
grep -Fq 'wait_for_cloud_sql_operation "${patch_operation}"' "$SCRIPT" || {
	printf 'Cloud SQL harness must wait for the label patch operation before backup\n' >&2
	exit 1
}
grep -Fq 'operations/${operation_id}' "$SCRIPT" || {
	printf 'Cloud SQL harness must query the owning operations API\n' >&2
	exit 1
}
grep -Fq 'Cloud SQL operation %s completed with an error' "$SCRIPT" || {
	printf 'Cloud SQL harness must fail on an operation error\n' >&2
	exit 1
}

claim_line="$(rg -n '^instance_claimed=1$' "$SCRIPT" | tail -1 | cut -d: -f1)"
create_line="$(rg -n 'gcloud sql instances create' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$claim_line" || -z "$create_line" || "$claim_line" -ge "$create_line" ]]; then
	printf 'Cloud SQL source name is not claimed before create (claim=%s create=%s)\n' "$claim_line" "$create_line" >&2
	exit 1
fi

patch_line="$(rg -n 'patch_response=' "$SCRIPT" | head -1 | cut -d: -f1)"
wait_line="$(rg -nF 'wait_for_cloud_sql_operation "${patch_operation}"' "$SCRIPT" | head -1 | cut -d: -f1)"
run_line="$(rg -n '\(cd "\$\{ROOT\}" && .*go run ./providers/gcp/cmd/gcp-cloudsql-acceptance' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$patch_line" || -z "$wait_line" || -z "$run_line" || "$patch_line" -ge "$wait_line" || "$wait_line" -ge "$run_line" ]]; then
	printf 'Cloud SQL operation readiness is not ordered before the backup command (patch=%s wait=%s run=%s)\n' "$patch_line" "$wait_line" "$run_line" >&2
	exit 1
fi

printf 'gcp_cloudsql_harness_shape_test OK\n'

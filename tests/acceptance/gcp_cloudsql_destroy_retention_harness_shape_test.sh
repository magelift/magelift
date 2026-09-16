#!/usr/bin/env bash
# Offline guard for the disposable Cloud SQL destroy-retention cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/providers/gcp/scripts/gcp-cloudsql-destroy-retention-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT" || {
	printf 'destroy-retention harness must source the shared dependency preflight\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_jq' "$SCRIPT" || {
	printf 'destroy-retention harness must preflight jq before provider mutation\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_commands gcloud curl go openssl' "$SCRIPT" || {
	printf 'destroy-retention harness must preflight local commands before provider mutation\n' >&2
	exit 1
}
grep -Fq -- '--final-backup' "$SCRIPT" || {
	printf 'destroy-retention harness must enable Cloud SQL final backups\n' >&2
	exit 1
}
grep -Fq -- '--final-backup-retention-days=1' "$SCRIPT" || {
	printf 'destroy-retention harness must cap final backup retention at one day\n' >&2
	exit 1
}
grep -Fq -- '--no-retain-backups-on-delete' "$SCRIPT" || {
	printf 'destroy-retention harness must isolate leftover proof to the final backup\n' >&2
	exit 1
}
grep -Fq 'gcloud sql instances delete "${instance}"' "$SCRIPT" || {
	printf 'destroy-retention harness must delete the claimed instance before leftover cleanup\n' >&2
	exit 1
}
grep -Fq 'go run ./providers/gcp/cmd/gcp-cloudsql-destroy-retention-acceptance' "$SCRIPT" || {
	printf 'destroy-retention harness must invoke the MageLift leftover deleter\n' >&2
	exit 1
}
grep -Fq 'existing_instance_probe=' "$SCRIPT" || {
	printf 'destroy-retention harness must prove its generated instance name is unused\n' >&2
	exit 1
}
grep -Fq 'instance_claimed=1' "$SCRIPT" || {
	printf 'destroy-retention harness must claim the source name before create\n' >&2
	exit 1
}
grep -Fq 'delete_leftover_backups_for_instance' "$SCRIPT" || {
	printf 'destroy-retention cleanup must delete leftovers for the claimed instance\n' >&2
	exit 1
}

claim_line="$(grep -n '^instance_claimed=1$' "$SCRIPT" | tail -1 | cut -d: -f1)"
create_line="$(grep -n 'gcloud sql instances create' "$SCRIPT" | head -1 | cut -d: -f1)"
delete_line="$(grep -nF 'gcloud sql instances delete "${instance}" --project="${project}" --quiet' "$SCRIPT" | head -1 | cut -d: -f1)"
run_line="$(grep -nF 'go run ./providers/gcp/cmd/gcp-cloudsql-destroy-retention-acceptance' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$claim_line" || -z "$create_line" || "$claim_line" -ge "$create_line" ]]; then
	printf 'destroy-retention source name is not claimed before create (claim=%s create=%s)\n' "$claim_line" "$create_line" >&2
	exit 1
fi
if [[ -z "$delete_line" || -z "$run_line" || "$delete_line" -ge "$run_line" ]]; then
	printf 'destroy-retention must delete the instance before leftover cleanup (delete=%s run=%s)\n' "$delete_line" "$run_line" >&2
	exit 1
fi

printf 'gcp_cloudsql_destroy_retention_harness_shape_test OK\n'

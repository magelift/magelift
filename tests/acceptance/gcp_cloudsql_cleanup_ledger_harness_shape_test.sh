#!/usr/bin/env bash
# Offline guard for the disposable Cloud SQL cleanup-ledger reconcile cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-cloudsql-cleanup-ledger-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT" || {
	printf 'cleanup-ledger harness must source the shared dependency preflight\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_jq' "$SCRIPT" || {
	printf 'cleanup-ledger harness must preflight jq before provider mutation\n' >&2
	exit 1
}
grep -Fq 'acceptance_require_commands gcloud curl go openssl' "$SCRIPT" || {
	printf 'cleanup-ledger harness must preflight local commands before provider mutation\n' >&2
	exit 1
}
grep -Fq 'go run ./cmd/magelift --output json cleanup claim' "$SCRIPT" || {
	printf 'cleanup-ledger harness must claim through magelift before create\n' >&2
	exit 1
}
grep -Fq 'go run ./cmd/magelift --output json cleanup record' "$SCRIPT" || {
	printf 'cleanup-ledger harness must record the provider identity after create\n' >&2
	exit 1
}
grep -Fq 'go run ./cmd/magelift --yes --output json cleanup reconcile' "$SCRIPT" || {
	printf 'cleanup-ledger harness must delete through magelift cleanup reconcile\n' >&2
	exit 1
}
grep -Fq -- '--no-backup' "$SCRIPT" || {
	printf 'cleanup-ledger harness must disable automated backups\n' >&2
	exit 1
}
grep -Fq -- '--no-deletion-protection' "$SCRIPT" || {
	printf 'cleanup-ledger harness must create an unprotected instance\n' >&2
	exit 1
}
grep -Fq -- '--kind cloudsql-instance' "$SCRIPT" || {
	printf 'cleanup-ledger harness must claim kind cloudsql-instance\n' >&2
	exit 1
}
grep -Fq 'existing_instance_probe=' "$SCRIPT" || {
	printf 'cleanup-ledger harness must prove its generated instance name is unused\n' >&2
	exit 1
}
grep -Fq 'instance_claimed=1' "$SCRIPT" || {
	printf 'cleanup-ledger harness must claim the source name before create\n' >&2
	exit 1
}
grep -Fq 'cleanup-ledger-${run_id}.json' "$SCRIPT" || {
	printf 'cleanup-ledger harness must not pre-create an empty JSON file with mktemp\n' >&2
	exit 1
}

claim_line="$(grep -n 'cleanup claim' "$SCRIPT" | head -1 | cut -d: -f1)"
create_line="$(grep -n 'gcloud sql instances create' "$SCRIPT" | head -1 | cut -d: -f1)"
record_line="$(grep -n 'cleanup record' "$SCRIPT" | head -1 | cut -d: -f1)"
reconcile_line="$(grep -n 'cleanup reconcile' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$claim_line" || -z "$create_line" || "$claim_line" -ge "$create_line" ]]; then
	printf 'cleanup-ledger claim is not before create (claim=%s create=%s)\n' "$claim_line" "$create_line" >&2
	exit 1
fi
if [[ -z "$record_line" || -z "$reconcile_line" || "$record_line" -ge "$reconcile_line" ]]; then
	printf 'cleanup-ledger record is not before reconcile (record=%s reconcile=%s)\n' "$record_line" "$reconcile_line" >&2
	exit 1
fi
if [[ "$create_line" -ge "$record_line" ]]; then
	printf 'cleanup-ledger create is not before record (create=%s record=%s)\n' "$create_line" "$record_line" >&2
	exit 1
fi

happy_path_delete="$(awk '/^cleanup\(\)/{in_cleanup=1} in_cleanup && /^}/{in_cleanup=0} in_cleanup==0 && /gcloud sql instances delete/{print}' "$SCRIPT" || true)"
if [[ -n "$happy_path_delete" ]]; then
	printf 'cleanup-ledger happy path must not gcloud-delete the instance; Magelift reconcile owns deletion\n' >&2
	exit 1
fi

printf 'gcp_cloudsql_cleanup_ledger_harness_shape_test OK\n'

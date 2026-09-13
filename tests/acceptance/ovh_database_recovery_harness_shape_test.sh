#!/usr/bin/env bash
# Offline contract for the disposable OVHcloud Public Cloud Database recovery cell.
# This test checks the dependency gate, exact ownership cleanup, and dry-run
# boundary without creating an OVH resource.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/ovh-database-recovery-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'acceptance_require_jq' "$SCRIPT"
grep -Fq 'acceptance_require_commands ovhcloud curl go' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_claim' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_record' "$SCRIPT"
grep -Fq 'acceptance_cleanup_reconcile_pending' "$SCRIPT"
grep -Fq 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT"
grep -Fq 'refusing to adopt it' "$SCRIPT"
grep -Fq 'source_claimed=1' "$SCRIPT"
grep -Fq 'delete_owned_outputs' "$SCRIPT"
grep -Fq 'delete_source' "$SCRIPT"
grep -Fq 'redact_provider_output' "$SCRIPT"
grep -Fq 'did not return a parseable service ID' "$SCRIPT"
grep -Fq 'extract_generated_password' "$SCRIPT"
grep -Fq 'credentials-reset' "$SCRIPT"
grep -Fq -- '--ip-restrictions' "$SCRIPT"
grep -Fq 'application-fixture backup/restore cell' "$SCRIPT"
grep -Fq 'provider-managed PITR point' "$SCRIPT"
grep -Fq 'protection=provider-optional' "$ROOT/cmd/ovh-database-recovery-acceptance/main.go"
grep -Fq 'applicationFixture=verified' "$ROOT/cmd/ovh-database-recovery-acceptance/main.go"
grep -Fq '+ ovh-database: creating encrypted source' "$SCRIPT"

output="$(
	MAGELIFT_OVH_DATABASE_RECOVERY_PROJECT=8728028545db487baeee2e472e7e96dd \
	MAGELIFT_OVH_DATABASE_RECOVERY_RUN_ID=shape-test \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	bash "$SCRIPT"
)"
grep -Fq 'no OVH mutation invoked' <<<"$output"
grep -Fq 'sourceDescription=magelift-database-shape-test' <<<"$output"
grep -Fq 'flavor=db1-4' <<<"$output"
grep -Fq 'plan=essential' <<<"$output"
grep -Fq 'region=GRA' <<<"$output"

printf 'ovh_database_recovery_harness_shape_test OK\n'

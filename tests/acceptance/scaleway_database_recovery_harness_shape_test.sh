#!/usr/bin/env bash
# Offline contract for the disposable Scaleway Managed Database recovery cell.
# This test checks the dependency gate, exact ownership cleanup, and dry-run
# boundary without creating a Scaleway resource.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/scaleway-database-recovery-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'acceptance_require_jq' "$SCRIPT"
grep -Fq 'acceptance_require_commands scw go' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_claim' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_record' "$SCRIPT"
grep -Fq 'acceptance_cleanup_reconcile_pending' "$SCRIPT"
grep -Fq 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT"
grep -Fq 'refusing to adopt it' "$SCRIPT"
grep -Fq 'magelift.io/ownership=' "$SCRIPT"
grep -Fq 'source_claimed=1' "$SCRIPT"
grep -Fq 'delete_owned_outputs' "$SCRIPT"
grep -Fq 'delete_source' "$SCRIPT"
grep -Fq 'encryption.enabled=true' "$SCRIPT"
grep -Fq 'volume-size=30G' "$SCRIPT"
grep -Fq 'engine=MySQL-8' "$SCRIPT"
grep -Fq 'redact_provider_output' "$SCRIPT"
grep -Fq 'sed -n '\''/^{/,$p'\''' "$SCRIPT"
grep -Fq 'did not return a parseable instance ID' "$SCRIPT"
grep -Fq 'protection=provider-optional' "$ROOT/cmd/scaleway-database-recovery-acceptance/main.go"
grep -Fq 'NewDatabaseRecoveryCell' "$ROOT/cmd/scaleway-database-recovery-acceptance/main.go"
grep -Fq 'cell.Run' "$ROOT/cmd/scaleway-database-recovery-acceptance/main.go"
grep -Fq '+ scaleway-rdb: %s status=%s operation=%s' "$ROOT/cmd/scaleway-database-recovery-acceptance/main.go"
grep -Fq '+ scaleway-rdb: creating encrypted source' "$SCRIPT"

output="$(
	MAGELIFT_SCALEWAY_DATABASE_RECOVERY_PROJECT=2be47b4e-0d39-444b-9d25-b2026bfa7e10 \
	MAGELIFT_SCALEWAY_DATABASE_RECOVERY_RUN_ID=shape-test \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	bash "$SCRIPT"
)"
grep -Fq 'no Scaleway mutation invoked' <<<"$output"
grep -Fq 'sourceName=magelift-rdb-shape-test' <<<"$output"
grep -Fq 'nodeType=DB-DEV-S' <<<"$output"

# Makefile dry-run sets only MAGELIFT_ACCEPTANCE_DRY_RUN=1.
unenv_output="$(
	env -u MAGELIFT_SCALEWAY_DATABASE_RECOVERY_PROJECT -u SCW_PROJECT_ID \
	MAGELIFT_SCALEWAY_DATABASE_RECOVERY_RUN_ID=shape-unenv \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	bash "$SCRIPT"
)"
grep -Fq 'no Scaleway mutation invoked' <<<"$unenv_output"
grep -Fq 'sourceName=magelift-rdb-shape-unenv' <<<"$unenv_output"

# Dry-run is an offline contract: it must not require the Scaleway CLI.
PATH=/usr/bin:/bin MAGELIFT_SCALEWAY_DATABASE_RECOVERY_RUN_ID=shape-noscw \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT" | grep -Fq 'no Scaleway mutation invoked'

printf 'scaleway_database_recovery_harness_shape_test OK\n'

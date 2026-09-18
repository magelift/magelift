#!/usr/bin/env bash
# Offline contract for the disposable AWS RDS/Aurora recovery cell. This test
# checks ownership and deletion guards without creating an AWS resource.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/aws-database-recovery-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'acceptance_require_json_yaml_tools' "$SCRIPT"
grep -Fq 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT"
grep -Fq 'refusing to adopt it' "$SCRIPT"
grep -Fq 'list-tags-for-resource' "$SCRIPT"
grep -Fq -- '--storage-encrypted' "$SCRIPT"
grep -Fq -- '--deletion-protection' "$SCRIPT"
grep -Fq -- '--skip-final-snapshot' "$SCRIPT"
grep -Fq -- '--delete-automated-backups' "$SCRIPT"
grep -Fq 'delete_owned_instances' "$SCRIPT"
grep -Fq 'delete_owned_snapshots' "$SCRIPT"
grep -Fq 'source_claimed=1' "$SCRIPT"
grep -Fq 'subnet_group_claimed=1' "$SCRIPT"

modify_line="$(grep -n -- '--no-deletion-protection' "$SCRIPT" | head -n 1 | cut -d: -f1)"
delete_line="$(grep -n -- 'delete-db-instance' "$SCRIPT" | head -n 1 | cut -d: -f1)"
if [[ -z "$modify_line" || -z "$delete_line" || "$modify_line" -ge "$delete_line" ]]; then
	printf 'owned RDS instance cleanup must disable deletion protection before delete\n' >&2
	exit 1
fi

output="$(MAGELIFT_AWS_RDS_RECOVERY_REGION=eu-north-1 MAGELIFT_AWS_RDS_RECOVERY_RUN_ID=shape-test MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT")"
grep -Fq 'no AWS mutation invoked' <<<"$output"
grep -Fq 'source=magelift-rds-shape-test' <<<"$output"
n="$(grep -F 'source=magelift-rds-shape-test' <<<"$output" | wc -l | tr -d ' ')"
[[ "$n" == 1 ]]

aurora_output="$(MAGELIFT_AWS_RDS_RECOVERY_REGION=eu-north-1 MAGELIFT_AWS_RDS_RECOVERY_RUN_ID=shape-aurora MAGELIFT_AWS_RDS_RECOVERY_SOURCE_KIND=cluster MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT")"
grep -Fq 'sourceKind=cluster' <<<"$aurora_output"
grep -Fq 'source=magelift-aurora-shape-aurora' <<<"$aurora_output"

printf 'aws_database_recovery_harness_shape_test OK\n'

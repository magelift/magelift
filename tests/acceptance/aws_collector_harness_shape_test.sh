#!/usr/bin/env bash
# Offline shape checks for the bounded ECS collector delivery cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/aws-collector-acceptance-local.sh"

[[ -x "$SCRIPT" ]] || { printf 'AWS collector acceptance script is not executable\n' >&2; exit 1; }
bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'acceptance_require_commands aws newrelic go mktemp' "$SCRIPT"
grep -Fq 'MAGELIFT_AWS_COLLECTOR_ACCEPTANCE' "$SCRIPT"
grep -Fq 'apiAccessCreateKeys' "$SCRIPT"
grep -Fq 'apiAccessDeleteKeys' "$SCRIPT"
grep -Fq 'secretsmanager create-secret' "$SCRIPT"
grep -Fq 'secretsmanager delete-secret' "$SCRIPT"
grep -Fq 'iam create-role' "$SCRIPT"
grep -Fq 'iam delete-role' "$SCRIPT"
grep -Fq 'ecs create-cluster' "$SCRIPT"
grep -Fq 'ecs delete-cluster' "$SCRIPT"
grep -Fq 'ecs create-service' "$SCRIPT"
grep -Fq 'ecs delete-service' "$SCRIPT"
grep -Fq 'go run ./cmd/aws-collector-acceptance' "$SCRIPT"
grep -Fq 'acceptance_start_ttl_watchdog' "$SCRIPT"

trap_line="$(rg -n '^trap cleanup EXIT$' "$SCRIPT" | head -1 | cut -d: -f1)"
watchdog_line="$(rg -n 'acceptance_start_ttl_watchdog' "$SCRIPT" | tail -1 | cut -d: -f1)"
if [[ -z "$trap_line" || -z "$watchdog_line" || "$watchdog_line" -le "$trap_line" ]]; then
	printf 'AWS ECS collector acceptance must install cleanup before the TTL watchdog\n' >&2
	exit 1
fi

output="$(
	MAGELIFT_AWS_COLLECTOR_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_ACCEPTANCE_TTL_SECONDS=60 \
	MAGELIFT_AWS_PROFILE=default \
	MAGELIFT_AWS_REGION=eu-west-3 \
	MAGELIFT_NEWRELIC_ACCOUNT_ID=8368691 \
	MAGELIFT_AWS_COLLECTOR_RUN_ID=offline-shape \
	bash "$SCRIPT" 2>/dev/null
)"
[[ "$output" == *"aws ECS collector acceptance dry-run ok"* ]]

printf 'aws_collector_harness_shape_test OK\n'

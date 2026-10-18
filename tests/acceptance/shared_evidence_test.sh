#!/usr/bin/env bash
# Offline check for the sealed JSONL evidence writer.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_SHARED_EVIDENCE="$TMP/evidence.jsonl"
export ACCEPTANCE_EVIDENCE_RUN_ID="run-shared-evidence"
export MAGELIFT_ACCEPTANCE_RUN_ID="run-shared-evidence"
export MAGELIFT_ACCEPTANCE_DIGEST="registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export MAGELIFT_ACCEPTANCE_RELEASE="2.4.9"
export MAGELIFT_ACCEPTANCE_PHP_VERSION="8.5"
export MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS="intl,pdo_mysql"
export MAGELIFT_ACCEPTANCE_COMPOSER_VERSION="2.10"
export MAGELIFT_ACCEPTANCE_DATABASE="rds-mysql"
export MAGELIFT_ACCEPTANCE_SEARCH="disabled"
export MAGELIFT_ACCEPTANCE_QUEUE="database"
export MAGELIFT_ACCEPTANCE_CACHE="valkey"
export MAGELIFT_ACCEPTANCE_WEB_CACHE="varnish"
export MAGELIFT_ACCEPTANCE_EDGE="none"
export MAGELIFT_ACCEPTANCE_FIXTURE_ID="fixture-shared-evidence"
export MAGELIFT_ACCEPTANCE_BACKUP_SET="backup-shared-evidence"
export MAGELIFT_ACCEPTANCE_OBSERVABILITY_SETUP="cloudwatch"
export MAGELIFT_ACCEPTANCE_EDGE_SETUP="none"
export MAGELIFT_ACCEPTANCE_SCHEMA_FINGERPRINT="schema-shared-evidence"
export MAGELIFT_ACCEPTANCE_MIGRATION_FINGERPRINT="migration-shared-evidence"
export MAGELIFT_ACCEPTANCE_STATE_BACKEND="state://shared-evidence"
export MAGELIFT_ACCEPTANCE_SCENARIO="architecture"
export MAGELIFT_ACCEPTANCE_STACK_ID="magelift/shared-evidence"
export MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

# shellcheck source=../../scripts/acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
append_shared_row "queueMode:db" "PASS" "4s" "aws" "test-account" "2026-08-04"
append_shared_cleanup "PASS" "ml-rc1-preview" "magelift-run=run-shared-evidence" ""

if [[ "$(wc -l <"$ACCEPTANCE_SHARED_EVIDENCE" | tr -d ' ')" -ne 2 ]]; then
	printf 'expected one cell row and one cleanup row\n' >&2
	exit 1
fi
jq -e 'select(.type == "cell" and .status == "PASS" and (.cellId | endswith("/architecture")) and (has("recordDigest") | not) and .dimensions.computeMode == "fargate" and .dimensions.kubernetesMode == "none")' "$ACCEPTANCE_SHARED_EVIDENCE" >/dev/null
jq -e 'select(.type == "cleanup" and .cleanup.status == "PASS" and (.cleanup.remaining | length == 0) and (has("recordDigest") | not))' "$ACCEPTANCE_SHARED_EVIDENCE" >/dev/null
if [[ "$(jq -r 'select(.type == "cell") | .cellId' "$ACCEPTANCE_SHARED_EVIDENCE" | sort -u | wc -l | tr -d ' ')" -ne 1 ]]; then
	printf 'expected one stable cell ID\n' >&2
	exit 1
fi

DRY_RUN_EVIDENCE="$TMP/dry-run.jsonl"
ACCEPTANCE_SHARED_EVIDENCE="$DRY_RUN_EVIDENCE" MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	append_shared_row "queueMode:db" "PASS" "4s" "aws" "test-account" "2026-08-04"
if ! jq -e 'select(.type == "cell" and .status == "SKIP" and (.reason | test("dry-run")))' "$DRY_RUN_EVIDENCE" >/dev/null; then
	printf 'dry-run PASS must record SKIP JSONL\n' >&2
	cat "$DRY_RUN_EVIDENCE" >&2
	exit 1
fi

printf 'shared_evidence_test OK\n'

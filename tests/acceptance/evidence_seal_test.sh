#!/usr/bin/env bash
# Offline check that the shell append log is sealed by the Go certification core.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
printf '+ evidence_seal_test: start\n'
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_SHARED_EVIDENCE="$TMP/candidates.jsonl"
export MAGELIFT_ACCEPTANCE_RUN_ID="run-evidence-seal"
export MAGELIFT_ACCEPTANCE_GENERATED_AT="2026-08-12T12:00:00Z"
export MAGELIFT_ACCEPTANCE_DIGEST="registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export MAGELIFT_ACCEPTANCE_RELEASE="2.4.9"
export MAGELIFT_ACCEPTANCE_RUNTIME="ecs-fargate"
export MAGELIFT_ACCEPTANCE_COMPUTE_MODE="fargate"
export MAGELIFT_ACCEPTANCE_KUBERNETES_MODE="none"
export MAGELIFT_ACCEPTANCE_PHP_VERSION="8.5.4"
export MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS="intl,pdo_mysql"
export MAGELIFT_ACCEPTANCE_COMPOSER_VERSION="2.10.2"
export MAGELIFT_ACCEPTANCE_DATABASE="rds-mysql"
export MAGELIFT_ACCEPTANCE_SEARCH="disabled"
export MAGELIFT_ACCEPTANCE_QUEUE="database"
export MAGELIFT_ACCEPTANCE_CACHE="valkey"
export MAGELIFT_ACCEPTANCE_WEB_CACHE="varnish"
export MAGELIFT_ACCEPTANCE_EDGE="none"
export MAGELIFT_ACCEPTANCE_FIXTURE_ID="fixture-evidence-seal"
export MAGELIFT_ACCEPTANCE_BACKUP_SET="backup-evidence-seal"
export MAGELIFT_ACCEPTANCE_OBSERVABILITY_SETUP="cloudwatch+newrelic"
export MAGELIFT_ACCEPTANCE_EDGE_SETUP="none"
export MAGELIFT_ACCEPTANCE_SCHEMA_FINGERPRINT="schema-evidence-seal"
export MAGELIFT_ACCEPTANCE_MIGRATION_FINGERPRINT="migration-evidence-seal"
export MAGELIFT_ACCEPTANCE_STATE_BACKEND="state://evidence-seal"
export MAGELIFT_ACCEPTANCE_STACK_ID="magelift/evidence-seal"
export MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

# shellcheck source=../../scripts/acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
append_shared_row "queueMode:db" "PASS" "4s" "aws" "test-account" "2026-08-12"
append_shared_cleanup "PASS" "ml-evidence-seal" "magelift-run=run-evidence-seal" ""

SEALED="$TMP/sealed.jsonl"
CERTIFICATION_BIN="$TMP/magelift-certification"
printf '+ evidence_seal_test: building certification CLI (cold cache can take several minutes)\n'
# Parallel build: the Makefile serializes Go for IDE safety, but this one
# bounded compile needs all cores; serial cold builds exceed 25 minutes.
SEAL_PROCS="$( (nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4) | head -n 1)"
(cd "$ROOT" && GOMAXPROCS="$SEAL_PROCS" GOFLAGS="${GOFLAGS:-} -p=$SEAL_PROCS" GOMEMLIMIT=4GiB go build -o "$CERTIFICATION_BIN" ./cmd/magelift)
printf '+ evidence_seal_test: sealing evidence\n'
"$CERTIFICATION_BIN" --output json certification seal --file "$ACCEPTANCE_SHARED_EVIDENCE" --output-file "$SEALED" >/dev/null

jq -e 'select(.type == "cell" and .status == "PASS" and (.recordDigest | length == 64))' "$SEALED" >/dev/null
jq -e 'select(.type == "cleanup" and .cleanup.status == "PASS" and (.recordDigest | length == 64))' "$SEALED" >/dev/null

CELL_ID="$(jq -r 'select(.type == "cell") | .cellId' "$SEALED")"
VERIFY_OUTPUT="$("$CERTIFICATION_BIN" --output json certification verify --file "$SEALED" --required-cell "$CELL_ID")"
printf '%s\n' "$VERIFY_OUTPUT" | jq -e '.passedCells == 1 and .cleanupVerified == true' >/dev/null

printf 'evidence_seal_test OK\n'

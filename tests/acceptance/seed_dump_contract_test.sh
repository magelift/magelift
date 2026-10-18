#!/usr/bin/env bash
# Offline tests for the live Magento seed-dump preflight.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

if acceptance_validate_magento_seed_dump "$ROOT/testdata/fixtures/migrate/tiny.sql" >/dev/null 2>&1; then
	printf 'the synthetic tiny migration fixture must not satisfy the live seed contract\n' >&2
	exit 1
fi

cat >"$TMP/valid.sql" <<'SQL'
CREATE TABLE `m2_flag` (id int);
CREATE TABLE IF NOT EXISTS `m2_setup_module` (module varchar(255));
CREATE TABLE `m2_core_config_data` (path varchar(255));
SQL

acceptance_validate_magento_seed_dump "$TMP/valid.sql" >/dev/null
gzip -c -- "$TMP/valid.sql" >"$TMP/valid.sql.gz"
acceptance_validate_magento_seed_dump "$TMP/valid.sql.gz" >/dev/null

printf 'not a gzip stream\n' >"$TMP/invalid.sql.gz"
if acceptance_validate_magento_seed_dump "$TMP/invalid.sql.gz" >/dev/null 2>&1; then
	printf 'malformed gzip seed dump must be rejected\n' >&2
	exit 1
fi

printf 'CREATE TABLE `flag` (id int);\n' >"$TMP/incomplete.sql"
if acceptance_validate_magento_seed_dump "$TMP/incomplete.sql" >/dev/null 2>&1; then
	printf 'incomplete Magento schema must be rejected\n' >&2
	exit 1
fi

printf 'seed_dump_contract_test OK\n'

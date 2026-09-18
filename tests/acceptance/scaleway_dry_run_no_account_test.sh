#!/usr/bin/env bash
# Makefile dry-run sets only MAGELIFT_ACCEPTANCE_DRY_RUN=1. These wrappers
# must print the contract without the Scaleway CLI.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
stub="$(mktemp -d)"
cleanup() { rm -rf "$stub"; }
trap cleanup EXIT
printf '#!/bin/sh\nprintf "scw must not run on dry-run\\n" >&2\nexit 1\n' >"$stub/scw"
chmod +x "$stub/scw"

run_dry() {
	local script="$1"
	local needle="$2"
	local output
	output="$(
		env -u SCW_ACCESS_KEY -u SCW_SECRET_KEY -u SCW_DEFAULT_PROJECT_ID \
			-u MAGELIFT_SCALEWAY_SECRET_RECOVERY_PROJECT \
			PATH="$stub:$PATH" \
			MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
			bash "$ROOT/$script"
	)"
	grep -Fq 'no Scaleway mutation invoked' <<<"$output"
	grep -Fq "$needle" <<<"$output"
}

run_dry scripts/scaleway-recovery-acceptance-local.sh 'scaleway recovery acceptance dry-run'
run_dry scripts/scaleway-secret-recovery-acceptance-local.sh 'Scaleway Secret Manager recovery acceptance dry-run ok'
run_dry scripts/scaleway-observability-acceptance-local.sh 'scaleway observability acceptance dry-run'
run_dry scripts/scaleway-database-recovery-acceptance-local.sh 'Scaleway Managed Database recovery acceptance dry-run ok'

printf 'scaleway_dry_run_no_account_test OK\n'

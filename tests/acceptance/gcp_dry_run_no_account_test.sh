#!/usr/bin/env bash
# Makefile dry-run sets only MAGELIFT_ACCEPTANCE_DRY_RUN=1. These wrappers
# must print the contract without a gcloud account or project config.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
stub="$(mktemp -d)"
cleanup() { rm -rf "$stub"; }
trap cleanup EXIT
printf '#!/bin/sh\nprintf "gcloud must not run on dry-run\\n" >&2\nexit 1\n' >"$stub/gcloud"
chmod +x "$stub/gcloud"

run_dry() {
	local script="$1"
	local needle="$2"
	local output
	output="$(
		env -u GOOGLE_CLOUD_PROJECT -u GCLOUD_PROJECT -u GCP_PROJECT \
			-u CLOUDSDK_CORE_PROJECT -u CLOUDSDK_CORE_ACCOUNT \
			PATH="$stub:$PATH" \
			MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
			bash "$ROOT/$script"
	)"
	grep -Fq "$needle" <<<"$output"
}

run_dry providers/gcp/scripts/gcp-observability-acceptance-local.sh 'no GCP mutation invoked'
run_dry providers/gcp/scripts/gcp-recovery-acceptance-local.sh 'no GCP mutation invoked'
run_dry providers/gcp/scripts/gcp-secret-recovery-acceptance-local.sh 'no mutation invoked'

printf 'gcp_dry_run_no_account_test OK\n'

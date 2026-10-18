#!/usr/bin/env bash
# Disposable Scaleway Secret Manager recovery acceptance. The wrapper owns
# exactly one generated Object Storage bucket and one generated source secret;
# the Go command owns the provider-neutral recovery cell. Secret deletion is
# intentionally scheduled by Scaleway (seven-day free retention) after the
# source is unprotected, while all active resources and the short-lived
# Object-Locked archive bucket are removed before the command exits.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_json_yaml_tools || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${SCW_PROFILE:-default}"
region="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_REGION:-fr-par}"
run_id="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
bucket="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_BUCKET:-magelift-secret-recovery-${run_id}}"
secret_name="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_SECRET_NAME:-magelift-acceptance-secret-${run_id}}"
marker="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_MARKER:-magelift/scaleway/secret/${run_id}}"
fixture="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_FIXTURE:-fixture-known-content-${run_id}}"
project="${MAGELIFT_SCALEWAY_SECRET_RECOVERY_PROJECT:-}"
bucket_created=0
ttl_marker="${TMPDIR:-/tmp}/magelift-scaleway-secret-recovery-${run_id}-ttl-expired-$$"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^(fr-par|nl-ams|pl-waw)$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$secret_name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid Scaleway profile, region, run ID, bucket, secret name, marker, or fixture\n' >&2
	exit 2
fi

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'Scaleway Secret Manager recovery acceptance dry-run ok profile=%s region=%s bucket=%s secret=%s marker=%s; no Scaleway mutation invoked\n' "$profile" "$region" "$bucket" "$secret_name" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands scw go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

if [[ -z "$project" ]]; then
	project="$(scw --profile "$profile" config get default-project-id 2>/dev/null || true)"
fi
if [[ -z "$project" ]]; then
	printf 'could not resolve the selected Scaleway profile default project ID\n' >&2
	exit 2
fi

acceptance_prepare_lifecycle

cleanup_source_secret() {
	local ids id
	ids="$(scw --profile "$profile" --output json secret secret list "project-id=$project" "region=$region" "name=$secret_name" 2>/dev/null | jq -r --arg ownership "magelift.io/ownership=$marker" '.[]? | select(any(.tags[]?; . == $ownership)) | .id' 2>/dev/null || true)"
	while IFS= read -r id; do
		[[ -n "$id" ]] || continue
		scw --profile "$profile" secret secret unprotect "$id" "region=$region" >/dev/null 2>&1 || true
		scw --profile "$profile" secret secret delete "$id" "region=$region" >/dev/null 2>&1 || true
	done <<<"$ids"
}

cleanup_bucket() {
	for _ in {1..45}; do
		if scw --profile "$profile" object bucket delete "$bucket" "region=$region" >/dev/null 2>&1; then
			local remaining
			remaining="$(scw --profile "$profile" --output json object bucket list "region=$region" 2>/dev/null | jq --arg name "$bucket" '[.[]? | select((.Name // .name // .bucket_name // "") == $name)] | length' 2>/dev/null || printf 'unknown')"
			if [[ "$remaining" == "0" ]]; then
				return 0
			fi
		fi
		sleep 2
	done
	return 1
}

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'Scaleway Secret Manager recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	cleanup_source_secret
	if (( bucket_created == 1 )) && ! cleanup_bucket; then
		printf 'Scaleway Secret Manager recovery archive bucket cleanup did not converge bucket=%s; inspect exact bucket ownership=%s\n' "$bucket" "$marker" >&2
		status=1
	fi
	if (( status == 0 )); then
		printf 'Scaleway Secret Manager recovery acceptance active resources cleaned; source secret is scheduled for free provider deletion retention name=%s\n' "$secret_name"
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

existing="$(scw --profile "$profile" --output json object bucket list "region=$region")"
if jq -e --arg name "$bucket" 'any(.[]?; (.Name // .name // .bucket_name // "") == $name)' <<<"$existing" >/dev/null; then
	printf 'generated Scaleway recovery bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
fi

scw --profile "$profile" --output json object bucket create "$bucket" "region=$region" "acl=private" "enable-versioning=true" >/dev/null
bucket_created=1

(cd "$ROOT" && go run ./cmd/scaleway-secret-recovery-acceptance \
	--profile "$profile" \
	--region "$region" \
	--bucket "$bucket" \
	--secret-name "$secret_name" \
	--marker "$marker" \
	--fixture "$fixture")

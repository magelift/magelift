#!/usr/bin/env bash
# Disposable Scaleway Object Storage recovery acceptance. The wrapper owns
# exactly one generated bucket; the Go cell owns the fixture, recovery
# operation, independent content verification, and marker-scoped cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${SCW_PROFILE:-default}"
region="${SCW_OBJECT_STORAGE_REGION:-fr-par}"
run_id="${MAGELIFT_SCALEWAY_RECOVERY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
bucket="${MAGELIFT_SCALEWAY_RECOVERY_BUCKET:-magelift-recovery-${run_id}}"
marker="${MAGELIFT_SCALEWAY_RECOVERY_MARKER:-magelift/scaleway/recovery/${run_id}}"
fixture="${MAGELIFT_SCALEWAY_RECOVERY_FIXTURE:-fixture-known-content-${run_id}}"
bucket_created=0
ttl_marker="${TMPDIR:-/tmp}/magelift-scaleway-recovery-${run_id}-ttl-expired-$$"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^(fr-par|nl-ams|pl-waw)$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid Scaleway profile, region, run ID, bucket, marker, or fixture\n' >&2
	exit 2
fi

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'Scaleway recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if (( bucket_created == 1 )); then
		scw --profile "$profile" --output json object bucket delete "$bucket" "region=$region" >/dev/null 2>&1
		local delete_status=$?
		if (( delete_status != 0 )); then
			printf 'Scaleway recovery bucket cleanup failed bucket=%s; inspect it before retrying\n' "$bucket" >&2
			status=1
		else
			local remaining
			remaining="$(scw --profile "$profile" --output json object bucket list "region=$region" 2>/dev/null | jq --arg name "$bucket" '[.[] | select((.Name // .name // .bucket_name // "") == $name)] | length' 2>/dev/null || printf 'unknown')"
			if [[ "$remaining" != "0" ]]; then
				printf 'Scaleway recovery bucket inventory is inconclusive after delete bucket=%s remaining=%s\n' "$bucket" "$remaining" >&2
				status=1
			fi
		fi
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

existing="$(scw --profile "$profile" --output json object bucket list "region=$region")"
if jq -e --arg name "$bucket" 'any(.[]; (.Name // .name // .bucket_name // "") == $name)' <<<"$existing" >/dev/null; then
	printf 'generated Scaleway recovery bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
fi

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'scaleway recovery acceptance dry-run profile=%s region=%s bucket=%s marker=%s\n' "$profile" "$region" "$bucket" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands scw go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

scw --profile "$profile" --output json object bucket create "$bucket" "region=$region" "acl=private" "enable-versioning=false" >/dev/null
bucket_created=1

(cd "$ROOT" && SCW_PROFILE="$profile" go run ./cmd/scaleway-recovery-acceptance \
	--profile "$profile" \
	--region "$region" \
	--bucket "$bucket" \
	--marker "$marker" \
	--fixture "$fixture")

printf 'Scaleway Object Storage recovery acceptance bucket deleted through exact generated ownership=%s\n' "$bucket"

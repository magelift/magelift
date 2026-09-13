#!/usr/bin/env bash
# Disposable OVHcloud Object Storage recovery acceptance. The wrapper owns
# exactly one generated bucket and, when no caller-owned S3 credentials are
# supplied, one generated ObjectStore user and credential. Every generated
# identity is revoked on exit; the Go cell owns the fixture, recovery
# operation, independent content verification, and marker-scoped cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_json_yaml_tools || dependency_status=1
acceptance_require_commands ovhcloud go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_OVH_RECOVERY_PROFILE:-${MAGELIFT_OVH_PROFILE:-default}}"
project="${MAGELIFT_OVH_RECOVERY_PROJECT:-${MAGELIFT_OVH_ACCEPTANCE_PROJECT:-dry-run}}"
region_cli="${MAGELIFT_OVH_RECOVERY_REGION:-GRA}"
region="$(printf '%s' "$region_cli" | tr '[:upper:]' '[:lower:]')"
endpoint="${MAGELIFT_OVH_RECOVERY_ENDPOINT:-https://s3.${region}.io.cloud.ovh.net}"
run_id="${MAGELIFT_OVH_RECOVERY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
bucket="${MAGELIFT_OVH_RECOVERY_BUCKET:-magelift-recovery-${run_id}}"
marker="${MAGELIFT_OVH_RECOVERY_MARKER:-magelift/ovh/recovery/${run_id}}"
fixture="${MAGELIFT_OVH_RECOVERY_FIXTURE:-fixture-known-content-${run_id}}"
ttl_marker="${TMPDIR:-/tmp}/magelift-ovh-recovery-${run_id}-ttl-expired-$$"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$project" =~ ^[A-Za-z0-9-]{8,64}$|^dry-run$ || ! "$region_cli" =~ ^[A-Za-z0-9-]{2,16}$ || ! "$region" =~ ^[a-z0-9-]{2,16}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'set valid OVHcloud profile, project, region, run ID, bucket, marker, and fixture values\n' >&2
	exit 2
fi

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'OVHcloud Object Storage recovery acceptance dry-run ok profile=%s project=%s region=%s bucket=%s marker=%s; no OVH mutation invoked\n' "$profile" "$project" "$region" "$bucket" "$marker"
	exit 0
fi
if [[ "$project" == dry-run ]]; then
	printf 'MAGELIFT_OVH_RECOVERY_PROJECT must be set to the exact OVHcloud project ID before live mutation\n' >&2
	exit 2
fi

acceptance_prepare_lifecycle

caller_access_key="${MAGELIFT_OVH_S3_ACCESS_KEY:-}"
caller_secret_key="${MAGELIFT_OVH_S3_SECRET_KEY:-}"
caller_owner_id="${MAGELIFT_OVH_S3_OWNER_ID:-}"
access_key="$caller_access_key"
secret_key="$caller_secret_key"
owner_id="$caller_owner_id"
temporary_user_id=""
temporary_credential_created=0
bucket_created=0

ovh_bucket_present() {
	local listing
	if ! listing="$(ovhcloud cloud storage object list --cloud-project "$project" --profile "$profile" --output json)"; then
		return 2
	fi
	if jq -e --arg bucket "$bucket" 'any((. // [])[]?; (.name // .containerName // .container_name // "") == $bucket)' <<<"$listing" >/dev/null 2>&1; then
		return 0
	fi
	return 1
}

wait_for_ovh_bucket_absent() {
	local attempt
	local probe_status
	for attempt in {1..30}; do
		if ovh_bucket_present; then
			sleep 2
		else
			probe_status=$?
			if (( probe_status == 1 )); then
				return 0
			fi
			return 1
		fi
	done
	return 1
}

cleanup() {
	local exit_status=$?
	local cleanup_status=0
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'OVHcloud recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if (( bucket_created == 1 )); then
		if ! ovhcloud cloud storage object delete "$bucket" --cloud-project "$project" --profile "$profile" >/dev/null 2>&1; then
			printf 'OVHcloud recovery bucket cleanup failed bucket=%s\n' "$bucket" >&2
			cleanup_status=1
		elif ! wait_for_ovh_bucket_absent; then
			printf 'OVHcloud recovery bucket inventory is inconclusive after deletion bucket=%s\n' "$bucket" >&2
			cleanup_status=1
		fi
	fi
	if (( temporary_credential_created == 1 )); then
		if ! ovhcloud cloud storage object credentials delete "$temporary_user_id" "$access_key" --cloud-project "$project" --profile "$profile" >/dev/null 2>&1; then
			printf 'OVHcloud temporary Object Storage credential cleanup failed user=%s\n' "$temporary_user_id" >&2
			cleanup_status=1
		fi
	fi
	if [[ -n "$temporary_user_id" ]]; then
		local user_status=""
		local user_count=""
		local attempt
		for attempt in {1..30}; do
			user_status="$(ovhcloud cloud user get "$temporary_user_id" --cloud-project "$project" --profile "$profile" --output json 2>/dev/null | jq -r '.status // empty' 2>/dev/null || true)"
			if [[ "$user_status" == "ok" ]]; then
				break
			fi
			sleep 2
		done
		if [[ "$user_status" != "ok" ]] || ! ovhcloud cloud user delete "$temporary_user_id" --cloud-project "$project" --profile "$profile" >/dev/null 2>&1; then
			printf 'OVHcloud temporary Object Storage user cleanup failed user=%s status=%s\n' "$temporary_user_id" "$user_status" >&2
			cleanup_status=1
		else
			for attempt in {1..30}; do
				user_count="$(ovhcloud cloud user list --cloud-project "$project" --profile "$profile" --output json 2>/dev/null | jq --arg description "$description" '[(. // [])[] | select(.description == $description)] | length' 2>/dev/null || true)"
				if [[ "$user_count" == "0" ]]; then
					break
				fi
				sleep 2
			done
			if [[ "$user_count" != "0" ]]; then
				printf 'OVHcloud temporary Object Storage user remains after deletion user=%s count=%s\n' "$temporary_user_id" "$user_count" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( exit_status == 0 && cleanup_status == 0 )); then
		printf 'OVHcloud Object Storage recovery acceptance bucket deleted through exact generated ownership=%s\n' "$bucket"
	else
		printf 'OVHcloud Object Storage recovery acceptance failed; exact bucket/user cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	if (( cleanup_status != 0 )); then
		exit_status=1
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

if [[ -z "$access_key" || -z "$secret_key" ]]; then
	if [[ -n "$access_key" || -n "$secret_key" ]]; then
		printf 'set both MAGELIFT_OVH_S3_ACCESS_KEY and MAGELIFT_OVH_S3_SECRET_KEY, or neither\n' >&2
		exit 2
	fi
	description="MageLift disposable recovery user ${run_id}"
	ovhcloud cloud user create --cloud-project "$project" --profile "$profile" --description "$description" --roles objectstore_operator --output json >/dev/null 2>&1
	user_ids="$(ovhcloud cloud user list --cloud-project "$project" --profile "$profile" --output json | jq -r --arg description "$description" '.[] | select(.description == $description) | .id')"
	if [[ "$(printf '%s\n' "$user_ids" | sed '/^$/d' | wc -l | tr -d ' ')" != 1 ]]; then
		printf 'could not resolve exactly one temporary OVHcloud Object Storage user\n' >&2
		exit 1
	fi
	temporary_user_id="$(printf '%s\n' "$user_ids" | sed '/^$/d')"
	user_status=""
	for attempt in {1..30}; do
		user_status="$(ovhcloud cloud user get "$temporary_user_id" --cloud-project "$project" --profile "$profile" --output json | jq -r '.status // empty' 2>/dev/null || true)"
		if [[ "$user_status" == "ok" ]]; then
			break
		fi
		sleep 2
	done
	if [[ "$user_status" != "ok" ]]; then
		printf 'temporary OVHcloud Object Storage user did not become ready: %s\n' "$user_status" >&2
		exit 1
	fi
	raw_credentials="$(ovhcloud cloud storage object credentials create "$temporary_user_id" --cloud-project "$project" --profile "$profile" --output json 2>&1)"
	credentials_json="$(printf '%s\n' "$raw_credentials" | sed -n '/^{/,$p' | jq -s '.[-1]')"
	access_key="$(printf '%s' "$credentials_json" | jq -r '.access // empty')"
	secret_key="$(printf '%s' "$credentials_json" | jq -r '.secret // empty')"
	if [[ -z "$access_key" || -z "$secret_key" ]]; then
		printf 'OVHcloud temporary Object Storage credential response did not contain access and secret fields\n' >&2
		exit 1
	fi
	owner_id="$temporary_user_id"
	temporary_credential_created=1
elif [[ ! "$owner_id" =~ ^[0-9]+$ ]]; then
	printf 'MAGELIFT_OVH_S3_OWNER_ID must be the numeric OVHcloud Object Storage owner ID when caller credentials are supplied\n' >&2
	exit 2
fi

if ovh_bucket_present; then
	printf 'generated OVHcloud recovery bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
else
	probe_status=$?
	if (( probe_status != 1 )); then
		printf 'could not prove generated OVHcloud recovery bucket is absent: %s\n' "$bucket" >&2
		exit 2
	fi
fi

ovhcloud cloud storage object create "$region_cli" --name "$bucket" --owner-id "$owner_id" --encryption-sse-algorithm AES256 --versioning-status disabled --cloud-project "$project" --profile "$profile" --output json >/dev/null
bucket_created=1
export MAGELIFT_OVH_S3_ACCESS_KEY="$access_key"
export MAGELIFT_OVH_S3_SECRET_KEY="$secret_key"

(cd "$ROOT" && go run ./cmd/ovh-recovery-acceptance \
	--region "$region" \
	--endpoint "$endpoint" \
	--bucket "$bucket" \
	--marker "$marker" \
	--fixture "$fixture")

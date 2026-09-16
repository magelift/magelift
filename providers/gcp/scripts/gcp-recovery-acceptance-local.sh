#!/usr/bin/env bash
# Disposable Google Cloud Storage recovery acceptance. The wrapper owns one
# generated globally-unique bucket; the Go cell owns known content, normalized
# recovery, independent verification, and marker-scoped cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_json_yaml_tools || dependency_status=1
acceptance_require_commands gcloud go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

project="${GOOGLE_CLOUD_PROJECT:-${GCLOUD_PROJECT:-}}"
if [[ -z "$project" ]]; then
	project="$(gcloud config get-value project 2>/dev/null)"
fi
location="${MAGELIFT_GCP_OBJECT_LOCATION:-EUROPE-WEST9}"
run_id="${MAGELIFT_GCP_RECOVERY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
bucket="${MAGELIFT_GCP_RECOVERY_BUCKET:-magelift-recovery-${run_id}}"
marker="${MAGELIFT_GCP_RECOVERY_MARKER:-magelift/gcp/recovery/${run_id}}"
fixture="${MAGELIFT_GCP_RECOVERY_FIXTURE:-fixture-known-content-${run_id}}"
destination="${MAGELIFT_GCP_RECOVERY_DESTINATION:-isolated}"
data_class="${MAGELIFT_GCP_RECOVERY_DATA_CLASS:-media}"
case "${destination}" in
isolated|in-place) ;;
*)
	printf 'MAGELIFT_GCP_RECOVERY_DESTINATION must be isolated or in-place, got %s\n' "${destination}" >&2
	exit 2
	;;
esac
case "${data_class}" in
media|infrastructure-state|audit-evidence) ;;
*)
	printf 'MAGELIFT_GCP_RECOVERY_DATA_CLASS must be media, infrastructure-state, or audit-evidence, got %s\n' "${data_class}" >&2
	exit 2
	;;
esac
bucket_created=0
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-recovery-${run_id}-ttl-expired-$$"

if [[ ! "$project" =~ ^[A-Za-z0-9][A-Za-z0-9.-]{4,61}[A-Za-z0-9]$ || ! "$location" =~ ^[A-Za-z0-9-]{2,32}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid GCP project, location, run ID, bucket, marker, or fixture\n' >&2
	exit 2
fi

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if (( bucket_created == 1 )); then
		gcloud storage rm --quiet --recursive "gs://${bucket}/" --project="$project" >/dev/null 2>&1
		local delete_status=$?
		if (( delete_status != 0 )); then
			printf 'GCP recovery bucket cleanup failed bucket=%s; inspect it before retrying\n' "$bucket" >&2
			status=1
		else
			if gcloud storage buckets describe "gs://${bucket}" --project="$project" >/dev/null 2>&1; then
				printf 'GCP recovery bucket still exists after recursive delete bucket=%s\n' "$bucket" >&2
				status=1
			fi
		fi
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM


probe_output=""
probe_status=0
probe_output="$(gcloud storage buckets describe "gs://${bucket}" --project="$project" --format=json 2>&1)" || probe_status=$?
if (( probe_status == 0 )); then
	printf 'generated GCP recovery bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
fi
if ! grep -Eqi '(not found|does not exist|HTTPError 404|: 404)' <<<"$probe_output"; then
	printf 'cannot prove generated GCP recovery bucket is absent; refusing to continue: %s\n%s\n' "$bucket" "$probe_output" >&2
	exit 2
fi

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'gcp recovery acceptance dry-run project=%s location=%s bucket=%s marker=%s class=%s destination=%s\n' "$project" "$location" "$bucket" "$marker" "$data_class" "$destination"
	exit 0
fi

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

gcloud storage buckets create "gs://${bucket}" \
	--project="$project" \
	--location="$location" \
	--default-storage-class=STANDARD \
	--uniform-bucket-level-access \
	--public-access-prevention >/dev/null
bucket_created=1

(cd "$ROOT" && GOOGLE_CLOUD_PROJECT="$project" go run ./providers/gcp/cmd/gcp-recovery-acceptance \
	--project "$project" \
	--bucket "$bucket" \
	--marker "$marker" \
	--fixture "$fixture" \
	--destination "$destination" \
	--data-class "$data_class")

printf 'GCP Cloud Storage recovery acceptance bucket deleted through exact generated ownership=%s\n' "$bucket"

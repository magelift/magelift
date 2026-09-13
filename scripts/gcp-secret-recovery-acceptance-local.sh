#!/usr/bin/env bash
# Credit-efficient GCP Secret Manager recovery cell. One generated Cloud
# Storage bucket is shared by the full backup/restore/integrity sequence; the
# Go cell owns the exact synthetic source secret and the shell trap provides a
# second exact-name cleanup attempt for interrupted processes.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
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
location="${MAGELIFT_GCP_SECRET_LOCATION:-EUROPE-WEST9}"
run_id="${MAGELIFT_GCP_SECRET_RECOVERY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
bucket="${MAGELIFT_GCP_SECRET_ARCHIVE_BUCKET:-magelift-secret-recovery-${run_id}}"
secret_id="${MAGELIFT_GCP_SECRET_RECOVERY_ID:-magelift-acceptance-secret-${run_id}}"
marker="${MAGELIFT_GCP_SECRET_RECOVERY_MARKER:-magelift/gcp/secret-recovery/${run_id}}"
fixture="${MAGELIFT_GCP_SECRET_RECOVERY_FIXTURE:-fixture-known-secret-${run_id}}"
destination="${MAGELIFT_GCP_SECRET_RECOVERY_DESTINATION:-isolated}"
case "${destination}" in
isolated|in-place) ;;
*)
	printf 'MAGELIFT_GCP_SECRET_RECOVERY_DESTINATION must be isolated or in-place, got %s\n' "${destination}" >&2
	exit 2
	;;
esac
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-secret-recovery-${run_id}-ttl-expired-$$"

if [[ ! "$project" =~ ^[A-Za-z0-9][A-Za-z0-9.-]{4,61}[A-Za-z0-9]$ || ! "$location" =~ ^[A-Za-z0-9-]{2,32}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$secret_id" =~ ^[A-Za-z0-9_-]+$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]] || (( ${#secret_id} > 255 )); then
	printf 'invalid GCP project, location, run ID, bucket, secret ID, marker, or fixture\n' >&2
	exit 2
fi

secret_created=0
bucket_created=0
cleanup() {
	local exit_status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP Secret Manager recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if (( secret_created == 1 )); then
		local secret_json secret_rc=0
		secret_json="$(gcloud secrets describe "$secret_id" --project="$project" --format=json 2>/dev/null)" || secret_rc=$?
		if (( secret_rc == 0 )) && jq -e --arg marker "$marker" '(.labels // {})["magelift_ownership"] == $marker and (.labels // {})["magelift_data_class"] == "configuration-secrets"' <<<"$secret_json" >/dev/null 2>&1; then
			gcloud secrets delete "$secret_id" --project="$project" --quiet >/dev/null 2>&1 || exit_status=1
		elif (( secret_rc == 0 )); then
			printf 'refusing fallback cleanup for GCP source secret with unexpected ownership secret=%s\n' "$secret_id" >&2
			exit_status=1
		fi
		if gcloud secrets describe "$secret_id" --project="$project" >/dev/null 2>&1; then
			printf 'GCP source secret remains after cleanup secret=%s\n' "$secret_id" >&2
			exit_status=1
		fi
	fi
	if (( bucket_created == 1 )); then
		gcloud storage rm --quiet --recursive "gs://$bucket/" --project="$project" >/dev/null 2>&1 || exit_status=1
		if gcloud storage buckets describe "gs://$bucket" --project="$project" >/dev/null 2>&1; then
			printf 'GCP Secret Manager recovery archive bucket remains after cleanup bucket=%s\n' "$bucket" >&2
			exit_status=1
		fi
	fi
	if (( exit_status != 0 )); then
		printf 'GCP Secret Manager recovery acceptance failed; exact source/output cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

probe_output=""
probe_status=0
probe_output="$(gcloud storage buckets describe "gs://$bucket" --project="$project" --format=json 2>&1)" || probe_status=$?
if (( probe_status == 0 )); then
	printf 'generated GCP Secret Manager recovery archive bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
fi
if ! grep -Eqi '(not found|does not exist|HTTPError 404|: 404)' <<<"$probe_output"; then
	printf 'cannot prove generated GCP Secret Manager archive bucket is absent; refusing to continue: %s\n' "$probe_output" >&2
	exit 2
fi

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'GCP Secret Manager recovery acceptance dry-run project=%s location=%s bucket=%s secret=%s marker=%s destination=%s; no mutation invoked\n' "$project" "$location" "$bucket" "$secret_id" "$marker" "$destination"
	exit 0
fi

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

gcloud storage buckets create "gs://$bucket" \
	--project="$project" \
	--location="$location" \
	--default-storage-class=STANDARD \
	--uniform-bucket-level-access \
	--public-access-prevention >/dev/null
bucket_created=1

if gcloud secrets describe "$secret_id" --project="$project" >/dev/null 2>&1; then
	printf 'generated GCP Secret Manager source secret already exists; refusing to adopt it: %s\n' "$secret_id" >&2
	exit 2
fi
secret_created=1

(cd "$ROOT" && GOOGLE_CLOUD_PROJECT="$project" go run ./cmd/gcp-secret-recovery-acceptance \
	--project "$project" \
	--archive-bucket "$bucket" \
	--secret-id "$secret_id" \
	--marker "$marker" \
	--fixture "$fixture" \
	--destination "$destination")

printf 'GCP Secret Manager recovery archive bucket deleted through exact generated ownership=%s\n' "$bucket"

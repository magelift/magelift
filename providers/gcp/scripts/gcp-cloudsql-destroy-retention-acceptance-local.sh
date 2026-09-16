#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands gcloud curl go openssl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

project="${GCP_PROJECT:-digital-lab-341608}"
region="${GCP_REGION:-europe-west1}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
suffix="$(printf '%s' "${run_id}" | tr '[:upper:]' '[:lower:]')"
instance="magelift-csqldr-${suffix}"
marker="magelift-csqldr-${suffix}"
instance_claimed=0
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-cloudsql-destroy-retention-${run_id}-ttl-expired-$$"

delete_leftover_backups_for_instance() {
	gcloud sql backups list --project="${project}" --format=json 2>/dev/null |
		jq -r --arg instance "${instance}" '.[] | select((.instance // "") == $instance or ((.instance // "") | endswith("/instances/" + $instance))) | (.name // empty)' |
		while IFS= read -r backup; do
			if [[ -n "${backup}" ]]; then
				gcloud sql backups delete "${backup}" --project="${project}" --quiet >/dev/null 2>&1 || true
			fi
		done
}

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP Cloud SQL destroy-retention TTL expired; forced exact cleanup instance=%s\n' "${instance}" >&2
		status=1
	fi
	if [[ "${instance_claimed}" == 1 ]] && gcloud sql instances describe "${instance}" --project="${project}" --format=json >/dev/null 2>&1; then
		gcloud sql instances delete "${instance}" --project="${project}" --quiet >/dev/null 2>&1 || true
	fi
	delete_leftover_backups_for_instance
	exit "${status}"
}
trap cleanup EXIT INT TERM
acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

wait_for_cloud_sql_operation() {
	local operation_name="$1"
	local operation_id="${operation_name##*/}"
	local operation_json status operation_error
	local max_attempts=60
	local poll_seconds=5
	local token

	if [[ -z "${operation_name}" || -z "${operation_id}" || ! "${operation_id}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		printf 'Cloud SQL operation response did not contain a safe operation name: %s\n' "${operation_name}" >&2
		return 1
	fi
	token="$(gcloud auth print-access-token)"
	for attempt in $(seq 1 "${max_attempts}"); do
		operation_json="$(curl -fsS \
			-H "Authorization: Bearer ${token}" \
			"https://sqladmin.googleapis.com/sql/v1beta4/projects/${project}/operations/${operation_id}")"
		status="$(jq -r '.status // empty' <<<"${operation_json}")"
		case "${status}" in
		DONE)
			operation_error="$(jq -r '(.error.errors // [])[0].message // empty' <<<"${operation_json}")"
			if [[ -n "${operation_error}" ]]; then
				printf 'Cloud SQL operation %s completed with an error: %s\n' "${operation_id}" "${operation_error}" >&2
				return 1
			fi
			return 0
			;;
		PENDING|RUNNING)
			;;
		*)
			printf 'Cloud SQL operation %s returned unexpected status: %s\n' "${operation_id}" "${status:-<empty>}" >&2
			return 1
			;;
		esac
		if [[ "${attempt}" -lt "${max_attempts}" ]]; then
			sleep "${poll_seconds}"
		fi
	done
	printf 'Cloud SQL operation %s did not complete within %ss\n' "${operation_id}" "$((max_attempts * poll_seconds))" >&2
	return 1
}

gcloud services enable sqladmin.googleapis.com --project="${project}" >/dev/null

existing_instance_probe="$(gcloud sql instances describe "${instance}" --project="${project}" --format='value(name)' 2>&1 || true)"
if [[ -n "${existing_instance_probe}" && "${existing_instance_probe}" != *NOT_FOUND* && "${existing_instance_probe}" != *404* && "${existing_instance_probe}" != *"does not exist"* ]]; then
	printf 'could not prove generated Cloud SQL instance name is unused: %s\n' "${existing_instance_probe}" >&2
	exit 2
fi
if [[ "${existing_instance_probe}" == "${instance}" ]]; then
	printf 'generated Cloud SQL instance name already exists; refusing to adopt it: %s\n' "${instance}" >&2
	exit 2
fi

instance_claimed=1
root_password="$(openssl rand -hex 24)"

# Smallest zonal MySQL that still takes a final backup on delete. A public IP
# is assigned only so create does not require a VPC; this cell never connects
# to MySQL. Automated backups stay enabled so final-backup is a valid instance
# setting. Retained automated backups after delete are refused so the leftover
# set is the final backup.
gcloud sql instances create "${instance}" \
	--project="${project}" \
	--database-version=MYSQL_8_4 \
	--edition=enterprise \
	--tier=db-f1-micro \
	--region="${region}" \
	--availability-type=zonal \
	--backup-start-time=03:00 \
	--retained-backups-count=1 \
	--final-backup \
	--final-backup-retention-days=1 \
	--no-retain-backups-on-delete \
	--assign-ip \
	--root-password="${root_password}" \
	--no-deletion-protection \
	--storage-type=SSD \
	--storage-size=10GB \
	--timeout=1200 \
	--quiet >/dev/null

token="$(gcloud auth print-access-token)"
labels="$(jq -cn --arg owner "${marker}" '{settings:{userLabels:{magelift_ownership:$owner,magelift_data_class:"database"}}}')"
patch_response="$(curl -fsS -X PATCH \
	-H "Authorization: Bearer ${token}" \
	-H 'Content-Type: application/json' \
	"https://sqladmin.googleapis.com/sql/v1beta4/projects/${project}/instances/${instance}" \
	-d "${labels}")"
if ! patch_operation="$(jq -er '.name // empty' <<<"${patch_response}")"; then
	printf 'Cloud SQL label patch did not return an operation identity\n' >&2
	exit 1
fi
wait_for_cloud_sql_operation "${patch_operation}"

ready=0
for attempt in $(seq 1 60); do
	if gcloud sql instances describe "${instance}" --project="${project}" --format=json 2>/dev/null |
		jq -e --arg owner "${marker}" '(.state == "RUNNABLE") and (.settings.userLabels.magelift_ownership == $owner)' >/dev/null; then
		ready=1
		break
	fi
	sleep 10
done
if [[ "${ready}" != 1 ]]; then
	printf 'Cloud SQL source did not become runnable and correctly owned\n' >&2
	exit 1
fi

# Leave final-backup flags unset so delete uses the instance setting.
gcloud sql instances delete "${instance}" --project="${project}" --quiet >/dev/null
instance_claimed=0

gone=0
for attempt in $(seq 1 60); do
	if ! gcloud sql instances describe "${instance}" --project="${project}" --format=json >/dev/null 2>&1; then
		gone=1
		break
	fi
	sleep 10
done
if [[ "${gone}" != 1 ]]; then
	printf 'Cloud SQL instance still exists after delete\n' >&2
	exit 1
fi

leftover_ready=0
for attempt in $(seq 1 36); do
	leftover_count="$(gcloud sql backups list --project="${project}" --format=json |
		jq --arg instance "${instance}" '[.[] | select((.instance // "") == $instance or ((.instance // "") | endswith("/instances/" + $instance)))] | length')"
	if [[ "${leftover_count}" -gt 0 ]]; then
		leftover_ready=1
		break
	fi
	sleep 5
done
if [[ "${leftover_ready}" != 1 ]]; then
	printf 'no leftover Cloud SQL backups appeared after instance deletion\n' >&2
	exit 1
fi

(cd "${ROOT}" && go run ./providers/gcp/cmd/gcp-cloudsql-destroy-retention-acceptance \
	--project="${project}" \
	--instance="${instance}")

printf 'gcp Cloud SQL destroy-retention leftovers were deleted by MageLift; trap will confirm independent inventory\n'

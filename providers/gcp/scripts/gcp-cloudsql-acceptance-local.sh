#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands gcloud curl go shasum openssl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi
if ! command -v mysql >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
	printf 'GCP Cloud SQL application fixture requires mysql or docker on PATH\n' >&2
	exit 2
fi

destination="${MAGELIFT_GCP_CLOUDSQL_DESTINATION:-isolated}"
case "${destination}" in
isolated|in-place) ;;
*)
	printf 'MAGELIFT_GCP_CLOUDSQL_DESTINATION must be isolated or in-place, got %s\n' "${destination}" >&2
	exit 2
	;;
esac
project="${GCP_PROJECT:-digital-lab-341608}"
region="${GCP_REGION:-europe-west1}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
suffix="$(printf '%s' "${run_id}" | tr '[:upper:]' '[:lower:]')"
instance="magelift-csql-${suffix}"
marker="magelift-csql-${suffix}"
fixture="fixture-csql-${suffix}"
marker_digest="$(printf '%s' "${marker}" | shasum -a 256 | awk '{print substr($1, 1, 24)}')"
backup_description_prefix="magelift-recovery:${marker_digest}:"
mysql_image="${MAGELIFT_GCP_CLOUDSQL_MYSQL_IMAGE:-mysql:8.4}"
root_password=""
instance_claimed=0
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-cloudsql-${run_id}-ttl-expired-$$"

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP Cloud SQL acceptance TTL expired; forced exact cleanup marker=%s\n' "${marker}" >&2
		status=1
	fi
	# Backups are deleted before instances so a failed acceptance still gets an
	# explicit owning-service cleanup attempt rather than relying on instance
	# deletion semantics.
	gcloud sql backups list --project="${project}" --format=json 2>/dev/null |
		jq -r --arg prefix "${backup_description_prefix}" '.[] | select((.description // "") | startswith($prefix)) | (.name // .id)' |
		while IFS= read -r backup; do
			if [[ -n "${backup}" ]]; then
				gcloud sql backups delete "${backup}" --project="${project}" --quiet >/dev/null 2>&1 || true
			fi
		done

	# The list response does not include settings.userLabels. Describe each
	# candidate and delete only after the owning label is independently read.
	gcloud sql instances list --project="${project}" --format='value(name)' 2>/dev/null |
		while IFS= read -r candidate; do
			if [[ -z "${candidate}" ]]; then
				continue
			fi
			if gcloud sql instances describe "${candidate}" --project="${project}" --format=json 2>/dev/null |
				jq -e --arg owner "${marker}" '.settings.userLabels.magelift_ownership == $owner' >/dev/null; then
				gcloud sql instances delete "${candidate}" --project="${project}" --quiet >/dev/null 2>&1 || true
			fi
			done

	# Claim the generated source name before the create request. A provider
	# request can be accepted and then return an error before the label PATCH;
	# exact-name cleanup closes that otherwise-leaky failure window. The name is
	# generated from this run's UTC timestamp and PID, and the preflight below
	# refuses both an existing name and an inconclusive ownership check.
	if [[ "${instance_claimed}" == 1 ]] && gcloud sql instances describe "${instance}" --project="${project}" --format=json >/dev/null 2>&1; then
		gcloud sql instances delete "${instance}" --project="${project}" --quiet >/dev/null 2>&1 || true
	fi
	exit "${status}"
}
trap cleanup EXIT INT TERM
acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

# Cloud SQL can report the instance as RUNNABLE while an asynchronous settings
# PATCH still owns the instance operation slot. Wait for the exact operation
# returned by that PATCH before starting the backup/restore command.
wait_for_cloud_sql_operation() {
	local operation_name="$1"
	local operation_id="${operation_name##*/}"
	local operation_json status operation_error
	local max_attempts=60
	local poll_seconds=5

	if [[ -z "${operation_name}" || -z "${operation_id}" || ! "${operation_id}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		printf 'Cloud SQL operation response did not contain a safe operation name: %s\n' "${operation_name}" >&2
		return 1
	fi

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

# The generated source is reachable only from this runner's current public
# IPv4 address. The password is passed to child processes through the
# environment; it never enters the MySQL or Docker argument list.
caller_ipv4="$(curl -4 -fsS https://api.ipify.org)"
if [[ ! "${caller_ipv4}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
	printf 'could not determine a usable runner public IPv4 address\n' >&2
	exit 2
fi
root_password="$(openssl rand -hex 24)"

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

# This is deliberately the smallest managed instance that still exercises the
# Cloud SQL backup/restore control plane and a known-content MySQL read after
# restore. It uses a generated root credential and a runner-scoped authorized
# network for this disposable cell. It has no deletion protection, no PITR log
# stream, and no retained backups after deletion.
gcloud sql instances create "${instance}" \
	--project="${project}" \
	--database-version=MYSQL_8_4 \
	--edition=enterprise \
	--tier=db-f1-micro \
	--region="${region}" \
	--availability-type=zonal \
	--backup-start-time=03:00 \
	--retained-backups-count=1 \
	--assign-ip \
	--authorized-networks="${caller_ipv4}/32" \
	--root-password="${root_password}" \
	--no-deletion-protection \
	--no-retain-backups-on-delete \
	--storage-type=SSD \
	--storage-size=10GB \
	--timeout=1200 \
	--quiet >/dev/null

token="$(gcloud auth print-access-token)"
labels="$(jq -cn --arg owner "${marker}" --arg fixture "${fixture}" '{settings:{userLabels:{magelift_ownership:$owner,magelift_data_class:"database",magelift_fixture:$fixture}}}')"
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
		jq -e --arg owner "${marker}" --arg fixture "${fixture}" '(.state == "RUNNABLE") and (.settings.userLabels.magelift_ownership == $owner) and (.settings.userLabels.magelift_data_class == "database") and (.settings.userLabels.magelift_fixture == $fixture)' >/dev/null; then
		ready=1
		break
	fi
	sleep 10
done
if [[ "${ready}" != 1 ]]; then
	printf 'Cloud SQL source did not become runnable and correctly owned\n' >&2
	exit 1
fi

(cd "${ROOT}" && MAGELIFT_GCP_CLOUDSQL_ROOT_PASSWORD="${root_password}" MAGELIFT_GCP_CLOUDSQL_MYSQL_IMAGE="${mysql_image}" go run ./providers/gcp/cmd/gcp-cloudsql-acceptance \
	--project="${project}" \
	--instance="${instance}" \
	--marker="${marker}" \
	--fixture="${fixture}" \
	--destination="${destination}" \
	--retention-days=1)

printf 'gcp Cloud SQL acceptance source and restore instances will now be removed by exact ownership inventory\n'

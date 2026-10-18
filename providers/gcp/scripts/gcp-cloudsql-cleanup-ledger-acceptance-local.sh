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
instance="magelift-cledgr-${suffix}"
marker="magelift-cledgr-${suffix}"
ledger="${TMPDIR:-/tmp}/magelift-gcp-cloudsql-cleanup-ledger-${run_id}.json"
instance_claimed=0
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-cloudsql-cleanup-ledger-${run_id}-ttl-expired-$$"

cleanup() {
	local cell_status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP Cloud SQL cleanup-ledger TTL expired; forced exact cleanup instance=%s\n' "${instance}" >&2
		cell_status=1
	fi
	if [[ "${instance_claimed}" == 1 ]] && gcloud sql instances describe "${instance}" --project="${project}" --format=json >/dev/null 2>&1; then
		gcloud sql instances delete "${instance}" --project="${project}" --quiet >/dev/null 2>&1 || true
	fi
	rm -f "${ledger}"
	exit "${cell_status}"
}
trap cleanup EXIT INT TERM
acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

wait_for_cloud_sql_operation() {
	local operation_name="$1"
	local operation_id="${operation_name##*/}"
	local operation_json op_status operation_error
	local max_attempts=60
	local poll_seconds=5
	local token

	if [[ -z "${operation_name}" || -z "${operation_id}" || ! "${operation_id}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		printf 'Cloud SQL operation response did not contain a safe operation name: %s\n' "${operation_name}" >&2
		return 1
	fi
	token="$(gcloud auth print-access-token)"
	for _ in $(seq 1 "${max_attempts}"); do
		operation_json="$(curl -fsS \
			-H "Authorization: Bearer ${token}" \
			"https://sqladmin.googleapis.com/sql/v1beta4/projects/${project}/operations/${operation_id}")"
		op_status="$(jq -r '.status // empty' <<<"${operation_json}")"
		case "${op_status}" in
		DONE)
			operation_error="$(jq -r '(.error.errors // [])[0].message // empty' <<<"${operation_json}")"
			if [[ -n "${operation_error}" ]]; then
				printf 'Cloud SQL operation %s completed with an error: %s\n' "${operation_id}" "${operation_error}" >&2
				return 1
			fi
			return 0
			;;
		PENDING|RUNNING)
			sleep "${poll_seconds}"
			;;
		*)
			printf 'Cloud SQL operation %s returned unexpected status %s\n' "${operation_id}" "${op_status}" >&2
			return 1
			;;
		esac
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

(cd "${ROOT}" && go run ./cmd/magelift --output json cleanup claim \
	--ledger "${ledger}" \
	--run-id "${run_id}" \
	--marker "${marker}" \
	--provider gcp \
	--region "${region}" \
	--project "${project}" \
	--kind cloudsql-instance \
	--role source \
	--name "${instance}")

# Smallest zonal MySQL. Automated backups stay off so this cell proves
# instance reconcile, not leftover-backup deletion. Public IP is assigned
# only so create does not require a VPC; this cell never connects to MySQL.
gcloud sql instances create "${instance}" \
	--project="${project}" \
	--database-version=MYSQL_8_4 \
	--edition=enterprise \
	--tier=db-f1-micro \
	--region="${region}" \
	--availability-type=zonal \
	--no-backup \
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
for _ in $(seq 1 60); do
	if gcloud sql instances describe "${instance}" --project="${project}" --format=json 2>/dev/null |
		jq -e --arg owner "${marker}" '(.state == "RUNNABLE") and (.settings.userLabels.magelift_ownership == $owner) and ((.settings.deletionProtectionEnabled // false) | not)' >/dev/null; then
		ready=1
		break
	fi
	sleep 10
done
if [[ "${ready}" != 1 ]]; then
	printf 'Cloud SQL source did not become runnable, unprotected, and correctly owned\n' >&2
	exit 1
fi

(cd "${ROOT}" && go run ./cmd/magelift --output json cleanup record \
	--ledger "${ledger}" \
	--kind cloudsql-instance \
	--name "${instance}" \
	--identity "${instance}")

reconcile_out="$(cd "${ROOT}" && go run ./cmd/magelift --yes --output json cleanup reconcile --ledger "${ledger}")"
printf '%s\n' "${reconcile_out}"
if ! jq -e --arg instance "${instance}" '
	(.status == "complete")
	and ((.deleted // []) | index("cloudsql-instance:" + $instance) != null)
' <<<"${reconcile_out}" >/dev/null; then
	printf 'MageLift cleanup reconcile did not delete the claimed Cloud SQL instance\n' >&2
	exit 1
fi

gone=0
for _ in $(seq 1 60); do
	if ! gcloud sql instances describe "${instance}" --project="${project}" --format=json >/dev/null 2>&1; then
		gone=1
		break
	fi
	sleep 10
done
if [[ "${gone}" != 1 ]]; then
	printf 'Cloud SQL instance still exists after MageLift cleanup reconcile\n' >&2
	exit 1
fi
instance_claimed=0

printf 'gcp Cloud SQL cleanup-ledger reconcile deleted instance=%s; trap will confirm independent inventory\n' "${instance}"

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

project="${GCP_PROJECT:-digital-lab-341608}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
suffix="$(printf '%s' "${run_id}" | tr '[:upper:]' '[:lower:]')"
topic="magelift-acceptance-${suffix}"
subscription="magelift-acceptance-sub-${suffix}"
marker="magelift-live-${suffix}"
fixture="fixture-live-${suffix}"
destination="${MAGELIFT_GCP_PUBSUB_RECOVERY_DESTINATION:-same-region}"
case "${destination}" in
same-region|isolated) ;;
*)
	printf 'MAGELIFT_GCP_PUBSUB_RECOVERY_DESTINATION must be same-region or isolated, got %s\n' "${destination}" >&2
	exit 2
	;;
esac
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-pubsub-${run_id}-ttl-expired-$$"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == 1 || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'gcp Pub/Sub acceptance dry-run ok; no provider mutation invoked\n'
	exit 0
fi

dependency_status=0
acceptance_require_commands gcloud go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

acceptance_prepare_lifecycle

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP Pub/Sub acceptance TTL expired; forced exact cleanup marker=%s\n' "${marker}" >&2
		status=1
	fi
	while IFS=$'\t' read -r snapshot owner data_class; do
		if [[ "${owner}" == "${marker}" && "${data_class}" == "queue" && -n "${snapshot}" ]]; then
			gcloud pubsub snapshots delete "${snapshot}" --project="${project}" --quiet >/dev/null 2>&1
		fi
	done < <(gcloud pubsub snapshots list --project="${project}" --format='value(name,labels.magelift_ownership,labels.magelift_data_class)' 2>/dev/null)
	while IFS=$'\t' read -r restore_sub owner role; do
		if [[ "${owner}" == "${marker}" && "${role}" == "isolated-restore" && -n "${restore_sub}" ]]; then
			gcloud pubsub subscriptions delete "${restore_sub}" --project="${project}" --quiet >/dev/null 2>&1
		fi
	done < <(gcloud pubsub subscriptions list --project="${project}" --format='value(name,labels.magelift_ownership,labels.magelift_recovery_role)' 2>/dev/null)
	gcloud pubsub subscriptions delete "${subscription}" --project="${project}" --quiet >/dev/null 2>&1
	gcloud pubsub topics delete "${topic}" --project="${project}" --quiet >/dev/null 2>&1
	exit "${status}"
}
trap cleanup EXIT INT TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

gcloud pubsub topics create "${topic}" \
	--project="${project}" \
	--labels="magelift_ownership=${marker},magelift_data_class=queue,magelift_fixture=${fixture}" \
	--quiet >/dev/null
gcloud pubsub subscriptions create "${subscription}" \
	--project="${project}" \
	--topic="projects/${project}/topics/${topic}" \
	--labels="magelift_ownership=${marker},magelift_data_class=queue,magelift_fixture=${fixture}" \
	--quiet >/dev/null
gcloud pubsub topics publish "${topic}" \
	--project="${project}" \
	--message="magelift-pubsub:${fixture}:${marker}" \
	--quiet >/dev/null

go run ./providers/gcp/cmd/gcp-pubsub-acceptance \
	--project="${project}" \
	--subscription="${subscription}" \
	--marker="${marker}" \
	--fixture="${fixture}" \
	--retention-days=7 \
	--destination="${destination}"

echo "gcp Pub/Sub acceptance resources cleaned by owning-service inventory"

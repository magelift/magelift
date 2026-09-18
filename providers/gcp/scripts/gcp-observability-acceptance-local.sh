#!/usr/bin/env bash
# Disposable GCP native observability acceptance. One marker owns one
# dashboard, one alert policy, one custom log bucket, and one routing sink;
# log and metric probes are data-plane records. The Go verifier republishes
# marker-scoped probes while a new Cloud Logging sink becomes effective, so a
# long routing delay cannot permanently lose the only probe. Cloud Logging
# bucket deletion is provider-tombstoned for seven days, so the harness reuses
# an exact compatible bucket between retries and records DELETE_REQUESTED as
# pending provider cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

project="${GCP_PROJECT:-digital-lab-341608}"
run_id="${MAGELIFT_GCP_OBSERVABILITY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_GCP_OBSERVABILITY_MARKER:-magelift/gcp/observability/${run_id}}"
verify_budget="${MAGELIFT_GCP_OBSERVABILITY_VERIFY_BUDGET:-4m}"
cleanup_enabled=0

if [[ ! "$project" =~ ^[a-z][a-z0-9-]{4,28}[a-z0-9]$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$verify_budget" =~ ^[0-9]+(ms|s|m|h)$ ]]; then
	printf 'invalid GCP project, run ID, marker, or verification budget\n' >&2
	exit 2
fi
marker_digest="$(printf '%s' "$marker" | shasum -a 256 | awk '{print substr($1,1,16)}')"
delivery_digest="$(printf '%s' delivery | shasum -a 256 | awk '{print substr($1,1,16)}')"
dashboard_display="MageLift ${marker_digest} Dashboard ${delivery_digest}"
alert_display="MageLift ${marker_digest} Alert ${delivery_digest}"
retention_bucket_id="magelift-${marker_digest}"
retention_bucket_description="MageLift managed observability retention ${marker_digest}"
retention_sink_id="magelift-${marker_digest}"
retention_sink_description="MageLift managed observability routing ${marker_digest}"
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-observability-${run_id}-ttl-expired-$$"

cleanup_owned_log_retention() {
	local sink bucket
	sink="$(gcloud logging sinks describe "$retention_sink_id" --project="$project" --format=json 2>/dev/null || true)"
	if jq -e --arg description "$retention_sink_description" '(.description // "") == $description' <<<"$sink" >/dev/null 2>&1; then
		gcloud logging sinks delete "$retention_sink_id" --project="$project" --quiet >/dev/null 2>&1 || true
	fi
	bucket="$(gcloud logging buckets describe "$retention_bucket_id" --location=global --project="$project" --format=json 2>/dev/null || true)"
	if jq -e --arg description "$retention_bucket_description" '(.description // "") == $description and ((.locked // false) | not)' <<<"$bucket" >/dev/null 2>&1; then
		gcloud logging buckets delete "$retention_bucket_id" --location=global --project="$project" --quiet >/dev/null 2>&1 || true
	fi
}

delete_owned_metric_descriptors() {
	local access_token response metric_types metric_type descriptor_name remaining_count attempt
	local metric_type_prefix="custom.googleapis.com/magelift/${marker_digest}/"
	local metric_filter="metric.type = starts_with(\"${metric_type_prefix}\")"

	access_token="$(gcloud auth print-access-token 2>/dev/null || true)"
	if [[ -z "$access_token" ]]; then
		printf 'GCP observability cleanup could not obtain an access token for metric descriptors\n' >&2
		return 1
	fi
	if ! response="$(curl -fsS --path-as-is --get \
		-H "Authorization: Bearer ${access_token}" \
		--data-urlencode "filter=${metric_filter}" \
		"https://monitoring.googleapis.com/v3/projects/${project}/metricDescriptors")"; then
		printf 'GCP observability cleanup could not inventory metric descriptors through the owning API\n' >&2
		return 1
	fi
	if ! metric_types="$(jq -r --arg prefix "$metric_type_prefix" \
		'.metricDescriptors[]? | select((.type // "") | startswith($prefix)) | select(([.labels[]?.key] | index("magelift_ownership")) != null and ([.labels[]?.key] | index("magelift_managed")) != null) | .type' \
		<<<"$response")"; then
		printf 'GCP observability cleanup could not parse metric descriptor inventory\n' >&2
		return 1
	fi
	while IFS= read -r metric_type; do
		[[ -z "$metric_type" ]] && continue
		descriptor_name="projects/${project}/metricDescriptors/${metric_type}"
		if ! curl -fsS --path-as-is -X DELETE \
			-H "Authorization: Bearer ${access_token}" \
			"https://monitoring.googleapis.com/v3/${descriptor_name}" >/dev/null; then
			printf 'GCP observability cleanup could not delete owned metric descriptor type=%s\n' "$metric_type" >&2
			return 1
		fi
	done <<<"$metric_types"

	for ((attempt = 1; attempt <= 18; attempt++)); do
		if ! response="$(curl -fsS --path-as-is --get \
			-H "Authorization: Bearer ${access_token}" \
			--data-urlencode "filter=${metric_filter}" \
			"https://monitoring.googleapis.com/v3/projects/${project}/metricDescriptors")"; then
			printf 'GCP observability cleanup could not verify metric descriptor deletion\n' >&2
			return 1
		fi
		if ! remaining_count="$(jq --arg prefix "$metric_type_prefix" \
			'[.metricDescriptors[]? | select((.type // "") | startswith($prefix)) | select(([.labels[]?.key] | index("magelift_ownership")) != null and ([.labels[]?.key] | index("magelift_managed")) != null)] | length' \
			<<<"$response")"; then
			printf 'GCP observability cleanup could not parse metric descriptor deletion verification\n' >&2
			return 1
		fi
		if [[ "$remaining_count" == "0" ]]; then
			return 0
		fi
		if ((attempt < 18)); then
			sleep 5
		fi
	done
	printf 'GCP observability cleanup left owned metric descriptors=%s\n' "$remaining_count" >&2
	return 1
}

cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP observability acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if ((cleanup_enabled == 0)); then
		exit "$status"
	fi
	while IFS= read -r name; do
		[[ -z "$name" || "$name" == "null" ]] && continue
		owner="$(gcloud monitoring dashboards describe "$name" --project="$project" --format='value(labels.magelift_ownership)' 2>/dev/null || true)"
		if [[ "$owner" == "$marker_digest" ]]; then
			gcloud monitoring dashboards delete "$name" --project="$project" --quiet >/dev/null 2>&1 || true
		fi
	done < <(gcloud monitoring dashboards list --project="$project" --format=json 2>/dev/null | jq -r --arg display "$dashboard_display" '.[] | select(.displayName == $display) | .name' || true)
	while IFS= read -r name; do
		[[ -z "$name" || "$name" == "null" ]] && continue
		owner="$(gcloud monitoring policies describe "$name" --project="$project" --format='value(userLabels.magelift_ownership)' 2>/dev/null || true)"
		if [[ "$owner" == "$marker_digest" ]]; then
			gcloud monitoring policies delete "$name" --project="$project" --quiet >/dev/null 2>&1 || true
		fi
		done < <(gcloud monitoring policies list --project="$project" --format=json 2>/dev/null | jq -r --arg display "$alert_display" '.[] | select(.displayName == $display) | .name' || true)
	cleanup_owned_log_retention
	metric_descriptor_status=0
	delete_owned_metric_descriptors || metric_descriptor_status=$?
	remaining_dashboards="$(gcloud monitoring dashboards list --project="$project" --format=json 2>/dev/null | jq --arg display "$dashboard_display" --arg owner "$marker_digest" '[.[] | select(.displayName == $display and .labels.magelift_ownership == $owner)] | length' || printf 'unknown')"
	remaining_policies="$(gcloud monitoring policies list --project="$project" --format=json 2>/dev/null | jq --arg display "$alert_display" --arg owner "$marker_digest" '[.[] | select(.displayName == $display and .userLabels.magelift_ownership == $owner)] | length' || printf 'unknown')"
	if ((status != 0)) || ((metric_descriptor_status != 0)) || [[ "$remaining_dashboards" != "0" || "$remaining_policies" != "0" ]]; then
		printf 'GCP observability acceptance failed marker=%s; cleanup dashboards=%s policies=%s metricDescriptors=%s\n' "$marker" "$remaining_dashboards" "$remaining_policies" "$metric_descriptor_status" >&2
		status=1
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

gcloud projects describe "$project" --format='value(projectId)' >/dev/null
if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'gcp observability acceptance dry-run project=%s marker=%s\n' "$project" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands gcloud go shasum curl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"
cleanup_enabled=1
(cd "$ROOT" && go run ./providers/gcp/cmd/gcp-observability-acceptance --project "$project" --marker "$marker" --verify-budget "$verify_budget")
printf 'GCP observability acceptance resources cleaned through exact owning-service inventory marker=%s\n' "$marker"

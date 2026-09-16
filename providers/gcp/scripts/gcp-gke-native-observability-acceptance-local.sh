#!/usr/bin/env bash
# Disposable GKE native Cloud Logging/Monitoring/Audit cell, an MSP scrape
# cell when MAGELIFT_GCP_NATIVE_OBS_MSP=1, or a GKE-tied log-based alert cell
# when MAGELIFT_GCP_NATIVE_OBS_ALERT=1. Creates one owned Autopilot or
# Standard cluster. This is not Magento, New Relic, redaction, or notification
# delivery evidence.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands gcloud kubectl mktemp shasum curl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

project="${MAGELIFT_GCP_PROJECT:-${GCP_PROJECT:-digital-lab-341608}}"
region="${MAGELIFT_GCP_NATIVE_OBS_REGION:-${GCP_REGION:-europe-west1}}"
run_id="${MAGELIFT_GCP_NATIVE_OBS_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
marker="${MAGELIFT_GCP_NATIVE_OBS_MARKER:-magelift/gcp/native-obs/${run_id}}"
runtime="${MAGELIFT_GCP_NATIVE_OBS_RUNTIME:-gke-autopilot}"
network="${MAGELIFT_GCP_NATIVE_OBS_NETWORK:-default}"
subnetwork="${MAGELIFT_GCP_NATIVE_OBS_SUBNETWORK:-default}"
machine_type="${MAGELIFT_GCP_NATIVE_OBS_MACHINE_TYPE:-e2-medium}"
node_count="${MAGELIFT_GCP_NATIVE_OBS_NODE_COUNT:-1}"
release_channel="${MAGELIFT_GCP_NATIVE_OBS_RELEASE_CHANNEL:-regular}"
cluster_cleanup_timeout_seconds="${MAGELIFT_GCP_NATIVE_OBS_CLUSTER_CLEANUP_TIMEOUT_SECONDS:-900}"
verify_seconds="${MAGELIFT_GCP_NATIVE_OBS_VERIFY_SECONDS:-600}"
poll_seconds="${MAGELIFT_GCP_NATIVE_OBS_POLL_SECONDS:-15}"
msp_required="${MAGELIFT_GCP_NATIVE_OBS_MSP:-0}"
alert_required="${MAGELIFT_GCP_NATIVE_OBS_ALERT:-0}"

if [[ ! "$project" =~ ^[a-z][a-z0-9-]{4,28}[a-z0-9]$ || ! "$region" =~ ^[a-z0-9-]{2,32}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,110}$ ]]; then
	printf 'set a valid GCP project, region, run ID, and marker\n' >&2
	exit 2
fi
if [[ "$runtime" != "gke-autopilot" && "$runtime" != "gke-standard" ]]; then
	printf 'MAGELIFT_GCP_NATIVE_OBS_RUNTIME must be gke-autopilot or gke-standard\n' >&2
	exit 2
fi
case "$release_channel" in
None|extended|provisioned|rapid|regular|stable) ;;
*)
	printf 'MAGELIFT_GCP_NATIVE_OBS_RELEASE_CHANNEL must be a documented GKE release channel\n' >&2
	exit 2
	;;
esac
if [[ ! "$machine_type" =~ ^[a-z][a-z0-9-]{1,31}$ ]]; then
	printf 'MAGELIFT_GCP_NATIVE_OBS_MACHINE_TYPE must be a GCE machine type name\n' >&2
	exit 2
fi
if [[ ! "$node_count" =~ ^[1-3]$ ]]; then
	printf 'MAGELIFT_GCP_NATIVE_OBS_NODE_COUNT must be 1, 2, or 3\n' >&2
	exit 2
fi
if [[ ! "$verify_seconds" =~ ^[1-9][0-9]*$ ]] || (( verify_seconds < 60 || verify_seconds > 1800 )); then
	printf 'MAGELIFT_GCP_NATIVE_OBS_VERIFY_SECONDS must be an integer from 60 through 1800\n' >&2
	exit 2
fi
if [[ ! "$poll_seconds" =~ ^[1-9][0-9]*$ ]] || (( poll_seconds > verify_seconds )); then
	printf 'MAGELIFT_GCP_NATIVE_OBS_POLL_SECONDS must be a positive integer no greater than the verify budget\n' >&2
	exit 2
fi
if [[ ! "$cluster_cleanup_timeout_seconds" =~ ^[1-9][0-9]*$ ]] || (( cluster_cleanup_timeout_seconds > 3600 )); then
	printf 'MAGELIFT_GCP_NATIVE_OBS_CLUSTER_CLEANUP_TIMEOUT_SECONDS must be an integer from 1 through 3600\n' >&2
	exit 2
fi

if [[ "$msp_required" != "0" && "$msp_required" != "1" ]]; then
	printf 'MAGELIFT_GCP_NATIVE_OBS_MSP must be 0 or 1\n' >&2
	exit 2
fi
if [[ "$alert_required" != "0" && "$alert_required" != "1" ]]; then
	printf 'MAGELIFT_GCP_NATIVE_OBS_ALERT must be 0 or 1\n' >&2
	exit 2
fi
if (( msp_required == 1 && alert_required == 1 )); then
	printf 'MAGELIFT_GCP_NATIVE_OBS_MSP and MAGELIFT_GCP_NATIVE_OBS_ALERT cannot both be 1\n' >&2
	exit 2
fi
cluster="ml-gke-nobs-${run_id//[._]/-}"
cluster="${cluster,,}"
cluster="${cluster:0:40}"
cluster="${cluster%-}"
if [[ ! "$cluster" =~ ^[a-z][a-z0-9-]{0,38}[a-z0-9]$ ]]; then
	printf 'derived GKE cluster name is invalid: %s\n' "$cluster" >&2
	exit 2
fi
ownership_label="$(printf '%s' "$marker" | shasum -a 256 | awk '{print substr($1, 1, 16)}')"
probe_message="magelift-native-obs-${ownership_label}"
namespace="magelift-nobs"
probe_name="marker-${ownership_label:0:12}"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'gcp gke native observability dry-run project=%s region=%s cluster=%s runtime=%s marker=%s create_cluster=1 create_mode=%s msp=%s alert=%s; no GCP mutation invoked\n' \
		"$project" "$region" "$cluster" "$runtime" "$marker" \
		"$([[ "$runtime" == "gke-standard" ]] && printf 'create' || printf 'create-auto')" \
		"$msp_required" "$alert_required"
	exit 0
fi

: "${MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE:?set MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1 for a disposable live GKE observability run}"
if [[ "$MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live GKE native observability without MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1\n' >&2
	exit 2
fi

gcloud auth application-default print-access-token >/dev/null 2>&1 || gcloud auth print-access-token >/dev/null

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-5400}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-native-obs-${run_id}-ttl-expired-$$"
kubeconfig="$(mktemp "${TMPDIR:-/tmp}/magelift-gcp-native-obs-kubeconfig.XXXXXX")"
chmod 600 "$kubeconfig"
cluster_created=0
log_metric_created=0
alert_policy_name=""
log_metric_id="magelift_nobs_${ownership_label}"

cluster_probe() {
	local output
	if output="$(gcloud container clusters describe "$cluster" --location "$region" --project "$project" --format=json 2>&1)"; then
		printf '%s\n' "$output"
		return 0
	fi
	if grep -Eqi 'not found|does not exist|NOT_FOUND|404' <<<"$output"; then
		return 1
	fi
	printf 'unable to inspect exact GKE cluster %s in %s: %s\n' "$cluster" "$region" "$output" >&2
	return 2
}

cluster_owned() {
	local description
	description="$(cluster_probe)" || return 1
	jq -e --arg cluster "$cluster" --arg marker "$ownership_label" '
		.name == $cluster and
		.resourceLabels["magelift-owner"] == "native-obs" and
		.resourceLabels["magelift-marker"] == $marker and
		.resourceLabels["magelift-run"] == $cluster
	' <<<"$description" >/dev/null
}

delete_disposable_cluster() {
	if (( cluster_created == 0 )); then
		return 0
	fi
	local description probe_status=0
	if description="$(cluster_probe)"; then
		if ! jq -e --arg cluster "$cluster" --arg marker "$ownership_label" '
			.name == $cluster and
			.resourceLabels["magelift-owner"] == "native-obs" and
			.resourceLabels["magelift-marker"] == $marker and
			.resourceLabels["magelift-run"] == $cluster
		' <<<"$description" >/dev/null; then
			printf 'refusing to delete GKE cluster with ownership drift cluster=%s location=%s\n' "$cluster" "$region" >&2
			return 1
		fi
	else
		probe_status=$?
		if (( probe_status == 1 )); then
			return 0
		fi
		return 1
	fi
	printf '+ deleting disposable GKE cluster=%s location=%s\n' "$cluster" "$region" >&2
	if ! gcloud container clusters delete "$cluster" --location "$region" --project "$project" --quiet --async >/dev/null 2>&1; then
		printf 'GKE cluster delete request failed cluster=%s location=%s\n' "$cluster" "$region" >&2
		return 1
	fi
	local started_at now elapsed
	started_at="$(date +%s)"
	while :; do
		if cluster_probe >/dev/null 2>&1; then
			probe_status=0
		else
			probe_status=$?
		fi
		if (( probe_status == 1 )); then
			printf '+ disposable GKE cluster cleanup verified cluster=%s location=%s\n' "$cluster" "$region" >&2
			return 0
		fi
		now="$(date +%s)"
		elapsed=$((now - started_at))
		if (( elapsed >= cluster_cleanup_timeout_seconds )); then
			printf 'timed out waiting for exact GKE cluster deletion cluster=%s location=%s elapsed=%ss\n' \
				"$cluster" "$region" "$elapsed" >&2
			return 1
		fi
		sleep 10
	done
}

# GA gcloud has no `monitoring time-series` command. Query the documented
# Monitoring API the same way the control-plane observability cell does.
count_monitoring_series() {
	local filter="$1"
	local access_token interval_end interval_start response
	access_token="$(gcloud auth print-access-token)"
	interval_end="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	if interval_start="$(date -u -d '20 minutes ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
		:
	elif interval_start="$(date -u -v-20M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
		:
	else
		printf 'could not compute a Monitoring lookback interval\n' >&2
		return 1
	fi
	if ! response="$(curl -fsS --path-as-is --get \
		-H "Authorization: Bearer ${access_token}" \
		--data-urlencode "filter=${filter}" \
		--data-urlencode "interval.startTime=${interval_start}" \
		--data-urlencode "interval.endTime=${interval_end}" \
		--data-urlencode "view=HEADERS" \
		--data-urlencode "pageSize=20" \
		"https://monitoring.googleapis.com/v3/projects/${project}/timeSeries")"; then
		printf 'Cloud Monitoring time-series list failed cluster=%s\n' "$cluster" >&2
		return 1
	fi
	jq '.timeSeries // [] | length' <<<"$response"
}

cleanup_gke_log_alert() {
	local remaining
	if [[ -n "$alert_policy_name" ]]; then
		gcloud monitoring policies delete "$alert_policy_name" --project="$project" --quiet >/dev/null 2>&1 || true
		alert_policy_name=""
	fi
	if (( log_metric_created == 1 )); then
		gcloud logging metrics delete "$log_metric_id" --project="$project" --quiet >/dev/null 2>&1 || true
		log_metric_created=0
	fi
	remaining="$(gcloud monitoring policies list --project="$project" --format=json 2>/dev/null | jq --arg owner "$ownership_label" '[.[] | select(.userLabels.magelift_ownership == $owner)] | length' || printf 'unknown')"
	if [[ "$remaining" != "0" ]]; then
		printf 'owned GKE log alert policies remain count=%s owner=%s\n' "$remaining" "$ownership_label" >&2
		return 1
	fi
	if gcloud logging metrics describe "$log_metric_id" --project="$project" >/dev/null 2>&1; then
		printf 'owned GKE log metric still present metric=%s\n' "$log_metric_id" >&2
		return 1
	fi
	return 0
}

apply_gke_log_alert() {
	local filter policy
	filter="resource.type=\"k8s_container\" AND resource.labels.cluster_name=\"${cluster}\" AND resource.labels.namespace_name=\"${namespace}\" AND (textPayload:\"${probe_message}\" OR jsonPayload.message:\"${probe_message}\")"
	if ! gcloud logging metrics create "$log_metric_id" \
		--project="$project" \
		--description="MageLift GKE native-obs ${ownership_label}" \
		--log-filter="$filter" >/dev/null; then
		printf 'failed to create GKE log-based metric metric=%s cluster=%s\n' "$log_metric_id" "$cluster" >&2
		return 1
	fi
	log_metric_created=1
	if ! policy="$(gcloud monitoring policies create \
		--project="$project" \
		--display-name="MageLift nobs ${ownership_label}" \
		--condition-display-name="GKE marker logs ${cluster}" \
		--condition-filter="metric.type=\"logging.googleapis.com/user/${log_metric_id}\" AND resource.type=\"k8s_container\"" \
		--aggregation='{"alignmentPeriod":"60s","perSeriesAligner":"ALIGN_DELTA","crossSeriesReducer":"REDUCE_SUM"}' \
		--duration=60s \
		--if='> 0' \
		--combiner=OR \
		--user-labels="magelift_ownership=${ownership_label},magelift_owner=nativeobs" \
		--documentation="GKE-tied log-based alert for cluster ${cluster}." \
		--format='value(name)')"; then
		printf 'failed to create GKE log-based alert policy cluster=%s\n' "$cluster" >&2
		return 1
	fi
	alert_policy_name="$policy"
	if [[ -z "$alert_policy_name" ]]; then
		printf 'alert policy create returned an empty name cluster=%s\n' "$cluster" >&2
		return 1
	fi
	if ! gcloud monitoring policies describe "$alert_policy_name" --project="$project" --format=json | jq -e --arg owner "$ownership_label" --arg cluster "$cluster" '
		.userLabels.magelift_ownership == $owner and
		(.conditions[0].displayName | contains($cluster))
	' >/dev/null; then
		printf 'created alert policy is missing GKE ownership or cluster identity policy=%s\n' "$alert_policy_name" >&2
		return 1
	fi
	printf '+ GKE log-based alert policy=%s metric=%s cluster=%s\n' "$alert_policy_name" "$log_metric_id" "$cluster" >&2
}

cleanup() {
	local status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP native observability TTL expired; forced cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if ! cleanup_gke_log_alert; then
		status=1
	fi
	if ! delete_disposable_cluster; then
		status=1
	fi
	rm -f "$kubeconfig"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

if cluster_probe >/dev/null; then
	printf 'refusing to create over existing GKE cluster %s in %s\n' "$cluster" "$region" >&2
	exit 1
else
	probe_status=$?
	if (( probe_status != 1 )); then
		exit 1
	fi
fi

printf '+ creating disposable GKE cluster=%s location=%s runtime=%s\n' "$cluster" "$region" "$runtime" >&2
export KUBECONFIG="$kubeconfig"
create_ok=0
if [[ "$runtime" == "gke-standard" ]]; then
	standard_create=(gcloud container clusters create "$cluster"
		--location "$region"
		--project "$project"
		--machine-type "$machine_type"
		--num-nodes "$node_count"
		--release-channel "$release_channel"
		--network "$network"
		--subnetwork "$subnetwork"
		--enable-ip-alias
		--logging=SYSTEM,WORKLOAD
		--monitoring=SYSTEM
		--labels="magelift-owner=native-obs,magelift-marker=$ownership_label,magelift-run=$cluster"
		--quiet)
	if (( msp_required == 1 )); then
		standard_create+=(--enable-managed-prometheus)
	fi
	if "${standard_create[@]}"; then
		create_ok=1
	fi
else
	if gcloud container clusters create-auto "$cluster" \
		--location "$region" \
		--project "$project" \
		--release-channel "$release_channel" \
		--network "$network" \
		--subnetwork "$subnetwork" \
		--labels="magelift-owner=native-obs,magelift-marker=$ownership_label,magelift-run=$cluster" \
		--quiet; then
		create_ok=1
	fi
fi
if (( create_ok == 0 )); then
	if cluster_owned; then
		cluster_created=1
	fi
	printf 'disposable GKE cluster create failed cluster=%s location=%s runtime=%s owned=%s\n' \
		"$cluster" "$region" "$runtime" "$cluster_created" >&2
	exit 1
fi
if ! cluster_owned; then
	printf 'created GKE cluster did not expose the expected ownership labels; refusing cleanup ambiguity cluster=%s\n' "$cluster" >&2
	exit 1
fi
cluster_created=1

cluster_json="$(cluster_probe)"
logging_components="$(jq -c '[.loggingConfig.componentConfig.enableComponents[]? // empty] | sort' <<<"$cluster_json")"
monitoring_components="$(jq -c '[.monitoringConfig.componentConfig.enableComponents[]? // empty] | sort' <<<"$cluster_json")"
if ! jq -e '(index("SYSTEM_COMPONENTS") and index("WORKLOADS")) or (index("SYSTEM") and index("WORKLOAD"))' <<<"$logging_components" >/dev/null; then
	printf 'GKE cluster is missing SYSTEM/WORKLOAD Cloud Logging components: %s\n' "$logging_components" >&2
	exit 1
fi
printf '+ GKE logging=%s monitoring=%s runtime=%s mspEnabled=%s\n' \
	"$logging_components" "$monitoring_components" "$runtime" \
	"$(jq -r '.monitoringConfig.managedPrometheusConfig.enabled // false' <<<"$cluster_json")" >&2

if (( msp_required == 1 )); then
	if ! jq -e '.monitoringConfig.managedPrometheusConfig.enabled == true' <<<"$cluster_json" >/dev/null; then
		printf 'GKE cluster does not have Managed Service for Prometheus enabled cluster=%s\n' "$cluster" >&2
		exit 1
	fi
fi

KUBECONFIG="$kubeconfig" gcloud container clusters get-credentials "$cluster" \
	--location "$region" --project "$project" --quiet >/dev/null
export KUBECONFIG="$kubeconfig"

kubectl create namespace "$namespace" --save-config >/dev/null
if (( msp_required == 0 )); then
	kubectl -n "$namespace" run "$probe_name" \
		--image=busybox:1.36 \
		--restart=Never \
		--labels="magelift-owner=native-obs,magelift-marker=$ownership_label" \
		--command -- /bin/sh -c "while true; do echo ${probe_message}; sleep 5; done" >/dev/null
	kubectl -n "$namespace" wait --for=condition=Ready "pod/${probe_name}" --timeout=180s >/dev/null
else
	gmp_ns=""
	gmp_wait_started="$(date +%s)"
	while :; do
		for candidate in gke-gmp-system gmp-system; do
			if kubectl -n "$candidate" get daemonset collector >/dev/null 2>&1; then
				gmp_ns="$candidate"
				break
			fi
		done
		if [[ -n "$gmp_ns" ]]; then
			break
		fi
		if (( $(date +%s) - gmp_wait_started >= 180 )); then
			printf 'GMP collector DaemonSet missing in gke-gmp-system and gmp-system cluster=%s\n' "$cluster" >&2
			exit 1
		fi
		sleep 5
	done
	printf '+ GMP collector DaemonSet namespace=%s\n' "$gmp_ns" >&2
	kubectl -n "$namespace" apply -f "$ROOT/scripts/acceptance/fixtures/gke-gmp-example.yaml" >/dev/null
	kubectl -n "$namespace" wait --for=condition=Available deployment/prom-example --timeout=180s >/dev/null
fi

started="$(date +%s)"
log_count=0
metric_count=0
audit_count=0
msp_count=0
alert_count=0
log_filter="resource.type=\"k8s_container\" AND resource.labels.cluster_name=\"${cluster}\" AND resource.labels.namespace_name=\"${namespace}\" AND (textPayload:\"${probe_message}\" OR jsonPayload.message:\"${probe_message}\")"
metric_filter="metric.type=\"kubernetes.io/container/uptime\" AND resource.labels.cluster_name=\"${cluster}\" AND resource.labels.namespace_name=\"${namespace}\""
audit_filter="protoPayload.serviceName=\"container.googleapis.com\" AND resource.labels.cluster_name=\"${cluster}\""
msp_filter="metric.type=\"prometheus.googleapis.com/example_requests_total/counter\" AND resource.type=\"prometheus_target\" AND resource.labels.cluster=\"${cluster}\" AND resource.labels.namespace=\"${namespace}\""

while :; do
	if (( msp_required == 0 )); then
		log_json="$(gcloud logging read "$log_filter" --project="$project" --freshness=1h --limit=20 --format=json 2>/dev/null || printf '[]')"
		log_count="$(jq 'length' <<<"$log_json")"
		if ! metric_count="$(count_monitoring_series "$metric_filter")"; then
			metric_count=0
		fi
		audit_json="$(gcloud logging read "$audit_filter" --project="$project" --freshness=2h --limit=20 --format=json 2>/dev/null || printf '[]')"
		audit_count="$(jq 'length' <<<"$audit_json")"
		if (( log_count > 0 && metric_count > 0 && audit_count > 0 )); then
			break
		fi
	else
		if ! msp_count="$(count_monitoring_series "$msp_filter")"; then
			msp_count=0
		fi
		if (( msp_count > 0 )); then
			break
		fi
	fi
	if (( $(date +%s) - started >= verify_seconds )); then
		printf 'GCP native observability timed out logs=%s metrics=%s audit=%s msp=%s cluster=%s\n' \
			"$log_count" "$metric_count" "$audit_count" "$msp_count" "$cluster" >&2
		exit 1
	fi
	sleep "$poll_seconds"
done
measured="$(( $(date +%s) - started ))"
alert_count=0
if (( alert_required == 1 )); then
	if ! apply_gke_log_alert; then
		exit 1
	fi
	alert_count=1
fi

printf 'GCP native observability PASS runtime=%s cluster=%s logs=%s metrics=%s audit=%s msp=%s alerts=%s measuredDeliverySeconds=%s cleanup=pending\n' \
	"$runtime" "$cluster" "$log_count" "$metric_count" "$audit_count" "$msp_count" "$alert_count" "$measured"

if ! cleanup_gke_log_alert; then
	exit 1
fi
if ! delete_disposable_cluster; then
	exit 1
fi
cluster_created=0
if cluster_probe >/dev/null 2>&1; then
	printf 'GKE cluster still present after verified delete cluster=%s\n' "$cluster" >&2
	exit 1
fi
printf 'GCP native observability complete; cluster inventory empty cluster=%s runtime=%s logs=%s metrics=%s audit=%s msp=%s alerts=%s measuredDeliverySeconds=%s\n' \
	"$cluster" "$runtime" "$log_count" "$metric_count" "$audit_count" "$msp_count" "$alert_count" "$measured"

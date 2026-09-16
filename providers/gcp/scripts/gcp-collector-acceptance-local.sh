#!/usr/bin/env bash
# Run the provider-owned GKE collector lifecycle against GKE.
# Existing clusters are the default; disposable Standard clusters are opt-in.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands gcloud newrelic go mktemp shasum || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_GCP_COLLECTOR_ACCEPTANCE:?set MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1 for live collector acceptance}"
if [[ "$MAGELIFT_GCP_COLLECTOR_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live GKE collector acceptance without MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1\n' >&2
	exit 2
fi

project="${GCP_PROJECT:-digital-lab-341608}"
region="${GCP_REGION:-europe-west1}"
cluster="${MAGELIFT_GCP_COLLECTOR_CLUSTER:-}"
runtime="${MAGELIFT_GCP_COLLECTOR_RUNTIME:-gke-autopilot}"
create_cluster="${MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER:-0}"
cluster_location="${MAGELIFT_GCP_COLLECTOR_CLUSTER_LOCATION:-}"
machine_type="${MAGELIFT_GCP_COLLECTOR_MACHINE_TYPE:-e2-medium}"
node_count="${MAGELIFT_GCP_COLLECTOR_NODE_COUNT:-1}"
network="${MAGELIFT_GCP_COLLECTOR_NETWORK:-default}"
subnetwork="${MAGELIFT_GCP_COLLECTOR_SUBNETWORK:-default}"
release_channel="${MAGELIFT_GCP_COLLECTOR_RELEASE_CHANNEL:-regular}"
cluster_cleanup_timeout_seconds="${MAGELIFT_GCP_COLLECTOR_CLUSTER_CLEANUP_TIMEOUT_SECONDS:-900}"
profile="${MAGELIFT_NEWRELIC_PROFILE:-default}"
account_id="${MAGELIFT_NEWRELIC_ACCOUNT_ID:-}"
endpoint="${MAGELIFT_NEWRELIC_OTLP_ENDPOINT:-}"
nerdgraph_endpoint="${MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT:-}"
image_digest="${MAGELIFT_GCP_COLLECTOR_IMAGE_DIGEST:-}"
run_id="${MAGELIFT_GCP_COLLECTOR_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_GCP_COLLECTOR_MARKER:-magelift/gcp/collector/$run_id}"
namespace="${MAGELIFT_GCP_COLLECTOR_NAMESPACE:-default}"
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-collector-${run_id}-ttl-expired-$$"

if [[ "$create_cluster" != "0" && "$create_cluster" != "1" ]]; then
 printf 'MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER must be 0 or 1\n' >&2
 exit 2
fi
if [[ -z "$cluster_location" ]]; then
 if [[ "$create_cluster" == "1" ]]; then
  cluster_location="${GCP_ZONE:-${region}-b}"
 else
  cluster_location="$region"
 fi
fi
if [[ ! "$cluster_location" =~ ^[A-Za-z0-9-]{2,32}$ || ! "$machine_type" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$network" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$subnetwork" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
 printf 'invalid GKE location, machine type, network, subnetwork, or release channel\n' >&2
 exit 2
fi
case "$release_channel" in
None|extended|provisioned|rapid|regular|stable) ;;
*)
 printf 'MAGELIFT_GCP_COLLECTOR_RELEASE_CHANNEL must be one of None, extended, provisioned, rapid, regular, or stable\n' >&2
 exit 2
 ;;
esac
if [[ ! "$node_count" =~ ^[1-9][0-9]*$ ]] || (( node_count > 3 )); then
 printf 'MAGELIFT_GCP_COLLECTOR_NODE_COUNT must be an integer from 1 through 3\n' >&2
 exit 2
fi
if [[ ! "$cluster_cleanup_timeout_seconds" =~ ^[1-9][0-9]*$ ]] || (( cluster_cleanup_timeout_seconds > 3600 )); then
 printf 'MAGELIFT_GCP_COLLECTOR_CLUSTER_CLEANUP_TIMEOUT_SECONDS must be an integer from 1 through 3600\n' >&2
 exit 2
fi
if [[ "$create_cluster" == "1" && "$runtime" != "gke-standard" ]]; then
 printf 'disposable cluster creation currently requires --runtime=gke-standard\n' >&2
 exit 2
fi
if [[ "$create_cluster" == "1" && -z "$cluster" ]]; then
 cluster="magelift-gke-${run_id//[._]/-}"
 cluster="${cluster,,}"
 cluster="${cluster:0:40}"
 cluster="${cluster%-}"
fi
if [[ -z "$cluster" || ! "$cluster" =~ ^[a-z][a-z0-9-]{0,38}[a-z0-9]$ ]]; then
 printf 'cluster must be a lowercase GKE name of 2 through 40 characters\n' >&2
 exit 2
fi
if [[ -z "$account_id" || -z "$endpoint" || -z "$nerdgraph_endpoint" || -z "$image_digest" ]]; then
 printf 'New Relic account, endpoints, and immutable collector image digest are required\n' >&2
 exit 2
fi
if [[ ! "$account_id" =~ ^[1-9][0-9]*$ || ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
 printf 'invalid account, profile, run ID, or marker\n' >&2
 exit 2
fi
ownership_label="$(printf '%s' "$marker" | shasum -a 256 | awk '{print substr($1, 1, 16)}')"
acceptance_prepare_lifecycle

kubeconfig="$(mktemp "${TMPDIR:-/tmp}/magelift-gcp-collector-kubeconfig.XXXXXX")"
chmod 600 "$kubeconfig"
cluster_created=0
key_id=""
license_key=""
query_key=""

cluster_probe() {
 local output
 if output="$(gcloud container clusters describe "$cluster" --location "$cluster_location" --project "$project" --format=json 2>&1)"; then
  printf '%s\n' "$output"
  return 0
 fi
 if grep -Eqi 'not found|does not exist|NOT_FOUND|404' <<<"$output"; then
  return 1
 fi
 printf 'unable to inspect exact GKE cluster %s in %s: %s\n' "$cluster" "$cluster_location" "$output" >&2
 return 2
}

cluster_owned() {
 local description
 description="$(cluster_probe)" || return 1
 jq -e --arg cluster "$cluster" --arg marker "$ownership_label" '
  .name == $cluster and
  .resourceLabels["magelift-owner"] == "collector" and
  .resourceLabels["magelift-marker"] == $marker and
  .resourceLabels["magelift-run"] == $cluster
 ' <<<"$description" >/dev/null
}

create_disposable_cluster() {
 if [[ "$create_cluster" != "1" ]]; then
  return 0
 fi

 local probe_status=0
 if cluster_probe >/dev/null; then
  printf 'refusing to create over existing GKE cluster %s in %s\n' "$cluster" "$cluster_location" >&2
  return 1
 else
  probe_status=$?
  if (( probe_status != 1 )); then
   return 1
  fi
 fi

 printf '+ creating disposable GKE Standard cluster=%s location=%s\n' "$cluster" "$cluster_location"
 local create_status=0
 if gcloud container clusters create "$cluster" \
  --location "$cluster_location" \
  --project "$project" \
  --machine-type "$machine_type" \
  --num-nodes "$node_count" \
  --release-channel "$release_channel" \
  --network "$network" \
  --subnetwork "$subnetwork" \
  --enable-ip-alias \
  --labels="magelift-owner=collector,magelift-marker=$ownership_label,magelift-run=$cluster" \
  --quiet; then
  :
 else
  create_status=$?
  if cluster_owned; then
   cluster_created=1
  fi
  printf 'disposable GKE cluster create failed cluster=%s location=%s owned=%s\n' \
   "$cluster" "$cluster_location" "$cluster_created" >&2
  return "$create_status"
 fi
 if ! cluster_owned; then
  printf 'created GKE cluster did not expose the expected ownership labels; refusing cleanup ambiguity cluster=%s\n' "$cluster" >&2
  return 1
 fi
 cluster_created=1
 printf '+ disposable GKE cluster ready cluster=%s location=%s\n' "$cluster" "$cluster_location"
}

delete_disposable_cluster() {
 if (( cluster_created == 0 )); then
  return 0
 fi

 local description probe_status=0
 if description="$(cluster_probe)"; then
  if ! jq -e --arg cluster "$cluster" --arg marker "$ownership_label" '
   .name == $cluster and
   .resourceLabels["magelift-owner"] == "collector" and
   .resourceLabels["magelift-marker"] == $marker and
   .resourceLabels["magelift-run"] == $cluster
  ' <<<"$description" >/dev/null; then
   printf 'refusing to delete GKE cluster with ownership drift cluster=%s location=%s\n' "$cluster" "$cluster_location" >&2
   return 1
  fi
 else
  probe_status=$?
  if (( probe_status == 1 )); then
   return 0
  fi
  return 1
 fi

 printf '+ deleting disposable GKE cluster=%s location=%s\n' "$cluster" "$cluster_location"
 if ! gcloud container clusters delete "$cluster" --location "$cluster_location" --project "$project" --quiet --async >/dev/null 2>&1; then
  printf 'GKE cluster delete request failed cluster=%s location=%s\n' "$cluster" "$cluster_location" >&2
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
   printf '+ disposable GKE cluster cleanup verified cluster=%s location=%s\n' "$cluster" "$cluster_location"
   return 0
  fi
  now="$(date +%s)"
  elapsed=$((now - started_at))
  if (( elapsed >= cluster_cleanup_timeout_seconds )); then
   printf 'timed out waiting for exact GKE cluster deletion cluster=%s location=%s probe_status=%s elapsed=%ss\n' \
    "$cluster" "$cluster_location" "$probe_status" "$elapsed" >&2
   return 1
  fi
  sleep 10
 done
}

cleanup() {
 local status=$?
 acceptance_stop_ttl_watchdog || true
 if acceptance_ttl_expired; then
  printf 'GCP collector acceptance TTL expired; refusing to report success marker=%s\n' "$marker" >&2
  status=1
 fi
 if ! delete_disposable_cluster; then
  status=1
 fi
 if [[ -n "$key_id" ]]; then
		delete_spec="$(jq -cn --arg id "$key_id" '{ingestKeyIds:[$id]}')"
		if ! newrelic --profile "$profile" --accountId "$account_id" apiAccess apiAccessDeleteKeys --keys "$delete_spec" --format JSON --plain >/dev/null 2>&1; then
			printf 'New Relic disposable collector ingest-key revocation failed; key id=%s\n' "$key_id" >&2
			status=1
		fi
	fi
	if [[ -e "$kubeconfig" ]]; then
		unlink "$kubeconfig"
	fi
	if [[ -e "$ttl_marker" ]]; then
		unlink "$ttl_marker"
	fi
	unset license_key query_key
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
 printf 'gcp collector acceptance dry-run ok project=%s cluster=%s runtime=%s create_cluster=%s location=%s marker=%s\n' \
  "$project" "$cluster" "$runtime" "$create_cluster" "$cluster_location" "$marker"
 exit 0
fi

create_disposable_cluster

query_spec='query { actor { apiAccess { keySearch(query: { types: USER }) { keys { key } } } } }'
query_key="$(newrelic --profile "$profile" --accountId "$account_id" nerdgraph query "$query_spec" --format JSON --plain | jq -r '.actor.apiAccess.keySearch.keys[] | select(.key != null and (.key | length > 0)) | .key' | head -1)"
if [[ -z "$query_key" ]]; then
	printf 'New Relic profile did not expose a usable user key for collector verification\n' >&2
	exit 2
fi

key_name="magelift-gcp-collector-$run_id"
key_notes="Disposable MageLift GKE collector acceptance marker=$marker"
key_spec="$(jq -cn --argjson accountId "$account_id" --arg name "$key_name" --arg notes "$key_notes" '{ingest:[{accountId:$accountId,ingestType:"LICENSE",name:$name,notes:$notes}]}')"
created="$(newrelic --profile "$profile" --accountId "$account_id" apiAccess apiAccessCreateKeys --keys "$key_spec" --format JSON --plain)"
key_id="$(printf '%s' "$created" | jq -r --arg name "$key_name" '.[] | select(.name == $name) | .id' | head -1)"
license_key="$(printf '%s' "$created" | jq -r --arg name "$key_name" '.[] | select(.name == $name) | .key' | head -1)"
if [[ -z "$key_id" || -z "$license_key" ]]; then
	printf 'New Relic disposable collector ingest-key creation returned no usable identity\n' >&2
	exit 1
fi

KUBECONFIG="$kubeconfig" gcloud container clusters get-credentials "$cluster" --location "$cluster_location" --project "$project" >/dev/null

KUBECONFIG="$kubeconfig" \
MAGELIFT_GCP_COLLECTOR_LICENSE_KEY="$license_key" \
MAGELIFT_GCP_COLLECTOR_QUERY_KEY="$query_key" \
go run ./providers/gcp/cmd/gcp-collector-acceptance \
	--project "$project" \
	--region "$region" \
	--cluster "$cluster" \
	--runtime "$runtime" \
	--namespace "$namespace" \
	--endpoint "$endpoint" \
	--nerdgraph-endpoint "$nerdgraph_endpoint" \
	--account-id "$account_id" \
	--image-digest "$image_digest" \
	--marker "$marker"

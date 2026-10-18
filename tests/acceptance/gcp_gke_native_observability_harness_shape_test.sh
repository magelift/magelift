#!/usr/bin/env bash
# Offline contract for the disposable GKE native observability cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/providers/gcp/scripts/gcp-gke-native-observability-acceptance-local.sh"

[[ -x "$SCRIPT" ]] || { printf 'GCP native observability acceptance script is not executable\n' >&2; exit 1; }
bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE' "$SCRIPT"
grep -Fq 'gcloud container clusters create-auto' "$SCRIPT"
grep -Fq 'gcloud container clusters create "$cluster"' "$SCRIPT"
grep -Fq -- '--logging=SYSTEM,WORKLOAD' "$SCRIPT"
grep -Fq 'gcloud container clusters delete "$cluster" --location "$region"' "$SCRIPT"
grep -Fq -- '--async' "$SCRIPT"
grep -Fq 'SYSTEM_COMPONENTS' "$SCRIPT"
grep -Fq 'WORKLOADS' "$SCRIPT"
grep -Fq 'kubernetes.io/container/uptime' "$SCRIPT"
grep -Fq 'container.googleapis.com' "$SCRIPT"
grep -Fq 'gcloud logging read' "$SCRIPT"
grep -Fq 'https://monitoring.googleapis.com/v3/projects/${project}/timeSeries' "$SCRIPT"
grep -Fq 'trap cleanup EXIT' "$SCRIPT"
grep -Fq 'acceptance_start_ttl_watchdog' "$SCRIPT"
grep -Fq 'prometheus.googleapis.com/example_requests_total/counter' "$SCRIPT"
grep -Fq 'kind: PodMonitoring' "$ROOT/scripts/acceptance/fixtures/gke-gmp-example.yaml"
grep -Fq 'gmp-system' "$SCRIPT"
grep -Fq 'gke-gmp-system' "$SCRIPT"
grep -Fq 'gcloud logging metrics create' "$SCRIPT"
grep -Fq 'gcloud monitoring policies create' "$SCRIPT"
grep -Fq 'resource.type=\"k8s_container\"' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_NATIVE_OBS_ALERT' "$SCRIPT"

trap_line="$(grep -n '^trap cleanup EXIT$' "$SCRIPT" | head -1 | cut -d: -f1)"
watchdog_line="$(grep -n 'acceptance_start_ttl_watchdog' "$SCRIPT" | tail -1 | cut -d: -f1)"
if [[ -z "$trap_line" || -z "$watchdog_line" || "$watchdog_line" -le "$trap_line" ]]; then
	printf 'native observability acceptance must install cleanup before the TTL watchdog\n' >&2
	exit 1
fi
dry_run_line="$(grep -n 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT" | head -1 | cut -d: -f1)"
create_auto_line="$(grep -n 'gcloud container clusters create-auto' "$SCRIPT" | head -1 | cut -d: -f1)"
create_std_line="$(grep -n 'gcloud container clusters create "\$cluster"' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$dry_run_line" || -z "$create_auto_line" || -z "$create_std_line" || "$create_auto_line" -le "$dry_run_line" || "$create_std_line" -le "$dry_run_line" ]]; then
	printf 'GKE creation must be after the dry-run exit gate\n' >&2
	exit 1
fi

output="$(
	MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_GCP_PROJECT=digital-lab-341608 \
	MAGELIFT_GCP_NATIVE_OBS_REGION=europe-west1 \
	MAGELIFT_GCP_NATIVE_OBS_RUN_ID=shape-test \
	MAGELIFT_GCP_NATIVE_OBS_MARKER=magelift/gcp/native-obs/shape-test \
	bash "$SCRIPT"
)"
grep -Fq 'no GCP mutation invoked' <<<"$output"
grep -Fq 'cluster=ml-gke-nobs-shape-test' <<<"$output"
grep -Fq 'runtime=gke-autopilot' <<<"$output"
grep -Fq 'create_mode=create-auto' <<<"$output"
grep -Fq 'msp=0' <<<"$output"
grep -Fq 'alert=0' <<<"$output"
if grep -Eq 'creating disposable GKE cluster=' <<<"$output"; then
	printf 'dry-run must not create a cluster\n' >&2
	exit 1
fi

std_output="$(
	MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_GCP_PROJECT=digital-lab-341608 \
	MAGELIFT_GCP_NATIVE_OBS_REGION=europe-west1 \
	MAGELIFT_GCP_NATIVE_OBS_RUNTIME=gke-standard \
	MAGELIFT_GCP_NATIVE_OBS_RUN_ID=shape-std \
	MAGELIFT_GCP_NATIVE_OBS_MARKER=magelift/gcp/native-obs/shape-std \
	bash "$SCRIPT"
)"
grep -Fq 'runtime=gke-standard' <<<"$std_output"
grep -Fq 'create_mode=create' <<<"$std_output"
grep -Fq 'no GCP mutation invoked' <<<"$std_output"

msp_output="$(
	MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_GCP_PROJECT=digital-lab-341608 \
	MAGELIFT_GCP_NATIVE_OBS_REGION=europe-west1 \
	MAGELIFT_GCP_NATIVE_OBS_MSP=1 \
	MAGELIFT_GCP_NATIVE_OBS_RUN_ID=shape-msp \
	MAGELIFT_GCP_NATIVE_OBS_MARKER=magelift/gcp/native-obs/shape-msp \
	bash "$SCRIPT"
)"
grep -Fq 'msp=1' <<<"$msp_output"
grep -Fq 'no GCP mutation invoked' <<<"$msp_output"

alert_output="$(
	MAGELIFT_GCP_NATIVE_OBS_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_GCP_PROJECT=digital-lab-341608 \
	MAGELIFT_GCP_NATIVE_OBS_REGION=europe-west1 \
	MAGELIFT_GCP_NATIVE_OBS_ALERT=1 \
	MAGELIFT_GCP_NATIVE_OBS_RUN_ID=shape-alert \
	MAGELIFT_GCP_NATIVE_OBS_MARKER=magelift/gcp/native-obs/shape-alert \
	bash "$SCRIPT"
)"
grep -Fq 'alert=1' <<<"$alert_output"
grep -Fq 'no GCP mutation invoked' <<<"$alert_output"

printf 'gcp_gke_native_observability_harness_shape_test OK\n'

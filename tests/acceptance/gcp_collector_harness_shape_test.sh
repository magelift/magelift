#!/usr/bin/env bash
# Offline shape checks for the bounded GKE collector delivery cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/providers/gcp/scripts/gcp-collector-acceptance-local.sh"

[[ -x "$SCRIPT" ]] || { printf 'GCP collector acceptance script is not executable\n' >&2; exit 1; }
bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'acceptance_require_commands gcloud newrelic go mktemp shasum' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_COLLECTOR_ACCEPTANCE' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER' "$SCRIPT"
grep -Fq 'cluster_probe' "$SCRIPT"
grep -Fq 'cluster_owned' "$SCRIPT"
grep -Fq 'gcloud container clusters delete "$cluster" --location "$cluster_location"' "$SCRIPT"
grep -Fq -- '--async' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_COLLECTOR_CLUSTER_CLEANUP_TIMEOUT_SECONDS' "$SCRIPT"
grep -Fq 'apiAccessCreateKeys' "$SCRIPT"
grep -Fq 'apiAccessDeleteKeys' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_COLLECTOR_LICENSE_KEY' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_COLLECTOR_QUERY_KEY' "$SCRIPT"
grep -Fq 'go run ./providers/gcp/cmd/gcp-collector-acceptance' "$SCRIPT"
grep -Fq 'acceptance_start_ttl_watchdog' "$SCRIPT"

trap_line="$(rg -n '^trap cleanup EXIT$' "$SCRIPT" | head -1 | cut -d: -f1)"
watchdog_line="$(rg -n 'acceptance_start_ttl_watchdog' "$SCRIPT" | tail -1 | cut -d: -f1)"
if [[ -z "$trap_line" || -z "$watchdog_line" || "$watchdog_line" -le "$trap_line" ]]; then
	printf 'GCP collector acceptance must install cleanup before the TTL watchdog\n' >&2
	exit 1
fi
dry_run_line="$(rg -n '^if \[\[ \"\$\{MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT" | head -1 | cut -d: -f1)"
create_call_line="$(rg -n '^create_disposable_cluster$' "$SCRIPT" | head -1 | cut -d: -f1)"
if [[ -z "$dry_run_line" || -z "$create_call_line" || "$create_call_line" -le "$dry_run_line" ]]; then
	printf 'disposable GKE creation must be after the dry-run exit gate\n' >&2
	exit 1
fi

output="$(
	MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_ACCEPTANCE_TTL_SECONDS=60 \
	GCP_PROJECT=digital-lab-341608 \
	GCP_REGION=europe-west1 \
	MAGELIFT_GCP_COLLECTOR_CLUSTER=offline-shape \
	MAGELIFT_NEWRELIC_ACCOUNT_ID=8368691 \
	MAGELIFT_NEWRELIC_OTLP_ENDPOINT=https://otlp.eu01.nr-data.net \
	MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT=https://api.eu.newrelic.com/graphql \
	MAGELIFT_GCP_COLLECTOR_IMAGE_DIGEST=docker.io/otel/opentelemetry-collector-contrib@sha256:$(printf '0%.0s' {1..64}) \
	MAGELIFT_GCP_COLLECTOR_RUN_ID=offline-shape \
	MAGELIFT_GCP_COLLECTOR_MARKER=magelift/gcp/collector/offline-shape \
	bash "$SCRIPT" 2>/dev/null
)"
[[ "$output" == *"gcp collector acceptance dry-run ok"* ]]

standard_output="$({
	MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1 \
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_ACCEPTANCE_TTL_SECONDS=60 \
	GCP_PROJECT=digital-lab-341608 \
	GCP_REGION=europe-west1 \
	MAGELIFT_GCP_COLLECTOR_RUNTIME=gke-standard \
	MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER=1 \
	MAGELIFT_NEWRELIC_ACCOUNT_ID=8368691 \
	MAGELIFT_NEWRELIC_OTLP_ENDPOINT=https://otlp.eu01.nr-data.net \
	MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT=https://api.eu.newrelic.com/graphql \
	MAGELIFT_GCP_COLLECTOR_IMAGE_DIGEST=docker.io/otel/opentelemetry-collector-contrib@sha256:$(printf '0%.0s' {1..64}) \
	MAGELIFT_GCP_COLLECTOR_RUN_ID=offline-standard \
	MAGELIFT_GCP_COLLECTOR_MARKER=magelift/gcp/collector/offline-standard \
	bash "$SCRIPT"
} 2>/dev/null)"
[[ "$standard_output" == *"create_cluster=1"* ]]
[[ "$standard_output" == *"cluster=magelift-gke-offline-standard"* ]]
[[ "$standard_output" == *"location=europe-west1-b"* ]]
if grep -Eq 'creating disposable GKE|gcloud container clusters create|created=1' <<<"$standard_output"; then
	printf 'standard dry-run must not create a cluster\n' >&2
	exit 1
fi

printf 'gcp_collector_harness_shape_test OK\n'

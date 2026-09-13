#!/usr/bin/env bash
# Disposable live New Relic NerdGraph operational-object acceptance. The
# authenticated CLI profile supplies a user key to the Go provider adapter;
# all policy, condition, dashboard, and service-level objects are marker-owned
# and destroyed by the Go command before it exits.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands newrelic go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_NEWRELIC_OPERATIONS_ACCEPTANCE:?set MAGELIFT_NEWRELIC_OPERATIONS_ACCEPTANCE=1 for live New Relic NerdGraph acceptance}"
if [[ "$MAGELIFT_NEWRELIC_OPERATIONS_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live New Relic NerdGraph acceptance without MAGELIFT_NEWRELIC_OPERATIONS_ACCEPTANCE=1\n' >&2
	exit 2
fi

profile="${MAGELIFT_NEWRELIC_PROFILE:-default}"
account_id="${MAGELIFT_NEWRELIC_ACCOUNT_ID:-}"
endpoint="${MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT:-}"
run_id="${MAGELIFT_NEWRELIC_OPERATIONS_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_NEWRELIC_OPERATIONS_MARKER:-magelift/acceptance/newrelic-operations/${run_id}}"
if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$account_id" =~ ^[1-9][0-9]*$ || ! "$endpoint" =~ ^https://[^[:space:]]+$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid New Relic profile, account, NerdGraph endpoint, run ID, or marker\n' >&2
	exit 2
fi

acceptance_prepare_lifecycle
cleanup_marker_file="$(mktemp "${TMPDIR:-/tmp}/magelift-newrelic-operations-acceptance.XXXXXX")"
cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	rm -f "$cleanup_marker_file"
	unset query_key
	if (( status != 0 )); then
		printf 'New Relic NerdGraph operations acceptance failed; inspect marker-scoped resources before reuse: %s\n' "$marker" >&2
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "${ACCEPTANCE_TTL_SECONDS:-1800}" "$cleanup_marker_file"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'newrelic NerdGraph operations acceptance dry-run ok profile=%s account=%s endpoint=%s marker=%s\n' "$profile" "$account_id" "$endpoint" "$marker"
	exit 0
fi

query_key_spec='query { actor { apiAccess { keySearch(query: { types: USER }) { keys { key } } } } }'
query_key="$(newrelic --profile "$profile" --accountId "$account_id" nerdgraph query "$query_key_spec" --format JSON --plain | jq -r '.actor.apiAccess.keySearch.keys[] | select(.key != null and (.key | length > 0)) | .key' | head -1)"
if [[ -z "$query_key" ]]; then
	printf 'New Relic profile did not expose a usable user key for NerdGraph operations\n' >&2
	exit 2
fi

entity_query="query { actor { entitySearch(query: \"accountId = ${account_id}\") { results { entities { guid entityType } } } } }"
slo_entity_guid="$(newrelic --profile "$profile" --accountId "$account_id" nerdgraph query "$entity_query" --format JSON --plain | jq -r '.actor.entitySearch.results.entities[] | select(.entityType == "THIRD_PARTY_SERVICE_ENTITY" or .entityType == "APM_APPLICATION_ENTITY" or .entityType == "SERVICE") | .guid' | head -1)"
if [[ -z "$slo_entity_guid" ]]; then
	printf 'no supported New Relic service entity was available for the SLO acceptance\n' >&2
	exit 2
fi

MAGELIFT_NEWRELIC_ACCOUNT_ID="$account_id" \
MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT="$endpoint" \
MAGELIFT_NEWRELIC_OPERATIONS_API_KEY="$query_key" \
MAGELIFT_NEWRELIC_OPERATIONS_MARKER="$marker" \
MAGELIFT_NEWRELIC_OPERATIONS_SLO_ENTITY_GUID="$slo_entity_guid" \
	go run ./cmd/newrelic-operations-acceptance

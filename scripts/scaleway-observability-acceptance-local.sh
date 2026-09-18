#!/usr/bin/env bash
# Disposable Scaleway Cockpit native observability acceptance. Custom data
# sources and a marker-scoped token are deleted through their owning APIs; the
# pre-existing Scaleway data sources are never adopted or deleted.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${SCW_PROFILE:-default}"
project="${SCW_PROJECT_ID:-2be47b4e-0d39-444b-9d25-b2026bfa7e10}"
region="${SCW_COCKPIT_REGION:-fr-par}"
run_id="${MAGELIFT_SCALEWAY_OBSERVABILITY_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_SCALEWAY_OBSERVABILITY_MARKER:-magelift/scaleway/observability/${run_id}}"
verify_budget="${MAGELIFT_SCALEWAY_OBSERVABILITY_VERIFY_BUDGET:-4m}"
cleanup_enabled=0

if [[ ! "$project" =~ ^[0-9a-fA-F-]{36}$ || ! "$region" =~ ^(fr-par|nl-ams|pl-waw)$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid Scaleway project, region, run ID, or observability marker\n' >&2
	exit 2
fi
marker_digest="$(printf '%s' "$marker" | shasum -a 256 | awk '{print substr($1,1,16)}')"
source_prefix="magelift-${marker_digest}-"
token_name="${source_prefix}token"
ttl_marker="${TMPDIR:-/tmp}/magelift-scaleway-observability-${run_id}-ttl-expired-$$"

cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'Scaleway observability acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if ((cleanup_enabled == 0)); then
		exit "$status"
	fi
	while IFS= read -r id; do
		[[ -z "$id" || "$id" == "null" ]] && continue
		scw --profile "$profile" --output json cockpit data-source delete "$id" "region=$region" >/dev/null 2>&1 || true
	done < <(scw --profile "$profile" --output json cockpit data-source list "project-id=$project" "region=$region" 2>/dev/null | jq -r --arg prefix "$source_prefix" '.[] | select(.origin == "custom" and (.name | startswith($prefix))) | .id' || true)
	while IFS= read -r id; do
		[[ -z "$id" || "$id" == "null" ]] && continue
		scw --profile "$profile" --output json cockpit token delete "$id" "region=$region" >/dev/null 2>&1 || true
	done < <(scw --profile "$profile" --output json cockpit token list "project-id=$project" "region=$region" 2>/dev/null | jq -r --arg name "$token_name" '.[] | select(.name == $name) | .id' || true)
	remaining_sources="$(scw --profile "$profile" --output json cockpit data-source list "project-id=$project" "region=$region" 2>/dev/null | jq --arg prefix "$source_prefix" '[.[] | select(.origin == "custom" and (.name | startswith($prefix)))] | length' || printf 'unknown')"
	remaining_tokens="$(scw --profile "$profile" --output json cockpit token list "project-id=$project" "region=$region" 2>/dev/null | jq --arg name "$token_name" '[.[] | select(.name == $name)] | length' || printf 'unknown')"
	if ((status != 0)) || [[ "$remaining_sources" != "0" || "$remaining_tokens" != "0" ]]; then
		printf 'Scaleway observability acceptance failed marker=%s; cleanup dataSources=%s tokens=%s\n' "$marker" "$remaining_sources" "$remaining_tokens" >&2
		status=1
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

scw --profile "$profile" --output json cockpit data-source list "project-id=$project" "region=$region" >/dev/null
if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'scaleway observability acceptance dry-run project=%s region=%s marker=%s\n' "$project" "$region" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands scw go shasum || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"
cleanup_enabled=1
(export SCW_PROFILE="$profile"; cd "$ROOT"; go run ./cmd/scaleway-observability-acceptance --project "$project" --region "$region" --marker "$marker" --verify-budget "$verify_budget")
printf 'Scaleway Cockpit observability acceptance resources cleaned through exact owning-service inventory marker=%s\n' "$marker"

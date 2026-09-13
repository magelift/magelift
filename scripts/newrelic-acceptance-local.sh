#!/usr/bin/env bash
# Bounded New Relic data-plane acceptance. The CLI owns the authenticated
# profile; this probe creates one uniquely marked custom event and verifies it
# through NRQL. It does not create a dashboard, alert, entity, or collector.
#
# Current API references:
#   https://docs.newrelic.com/docs/opentelemetry/best-practices/opentelemetry-otlp/
#   https://github.com/newrelic/newrelic-cli/blob/main/docs/GETTING_STARTED.md
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands newrelic || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_NEWRELIC_ACCEPTANCE:?set MAGELIFT_NEWRELIC_ACCEPTANCE=1 for a bounded live New Relic probe}"
if [[ "$MAGELIFT_NEWRELIC_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live New Relic acceptance without MAGELIFT_NEWRELIC_ACCEPTANCE=1\n' >&2
	exit 2
fi

profile="${MAGELIFT_NEWRELIC_PROFILE:-default}"
run_id="${MAGELIFT_NEWRELIC_ACCEPTANCE_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
event_type="${MAGELIFT_NEWRELIC_EVENT_TYPE:-MageLiftAcceptance}"
marker="${MAGELIFT_NEWRELIC_MARKER:-magelift/acceptance/newrelic/${run_id}}"
wait_seconds="${MAGELIFT_NEWRELIC_VERIFY_TIMEOUT_SECONDS:-180}"
poll_seconds="${MAGELIFT_NEWRELIC_VERIFY_POLL_SECONDS:-10}"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$event_type" =~ ^[A-Za-z][A-Za-z0-9_]{0,63}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$wait_seconds" =~ ^[1-9][0-9]*$ || ! "$poll_seconds" =~ ^[1-9][0-9]*$ ]]; then
	printf 'invalid New Relic acceptance profile, run ID, event type, marker, or timeout\n' >&2
	exit 2
fi

acceptance_prepare_lifecycle
cleanup_marker_file="$(mktemp "${TMPDIR:-/tmp}/magelift-newrelic-acceptance.XXXXXX")"
rm -f "$cleanup_marker_file"
cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	rm -f "$cleanup_marker_file"
	if (( status != 0 )); then
		printf 'New Relic acceptance failed; no provider resource cleanup is required (data-plane event marker=%s)\n' "$marker" >&2
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'newrelic acceptance dry-run ok profile=%s eventType=%s marker=%s\n' "$profile" "$event_type" "$marker"
	exit 0
fi

# `profile list` intentionally exposes only masked credentials. Parse the
# account identity from the selected profile without reading or printing a
# secret value. The CLI currently renders this command as a table.
profile_output="$(TERM=dumb NO_COLOR=1 newrelic --profile "$profile" --plain profile list 2>/dev/null || true)"
profile_line="$(printf '%s\n' "$profile_output" | sed $'s/\033\\[[0-9;]*m//g' | awk -v profile="$profile" '$1 == profile { print; exit }')"
account_id="$(printf '%s\n' "$profile_line" | awk '{ for (field = 1; field <= NF; field++) if ($field ~ /^[0-9]+$/) { print $field; exit } }')"
if [[ ! "$account_id" =~ ^[1-9][0-9]*$ ]]; then
	printf 'New Relic profile %q did not expose a usable account identity\n' "$profile" >&2
	exit 2
fi

acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$cleanup_marker_file"
event="$(jq -cn --arg eventType "$event_type" --arg marker "$marker" --arg runID "$run_id" '{eventType:$eventType, mageliftAcceptanceMarker:$marker, mageliftAcceptanceRunId:$runID, mageliftAcceptanceSource:"magelift-newrelic-acceptance"}')"
if ! newrelic --profile "$profile" --accountId "$account_id" events post --event "$event" >/dev/null; then
	printf 'New Relic custom event submission failed\n' >&2
	exit 1
fi

query="SELECT count(*) FROM ${event_type} WHERE mageliftAcceptanceMarker = '${marker}' SINCE 10 minutes ago"
deadline=$(( $(date +%s) + wait_seconds ))
while (( $(date +%s) < deadline )); do
	if acceptance_ttl_expired; then
		printf 'New Relic acceptance TTL expired while waiting for NRQL visibility\n' >&2
		exit 1
	fi
	result="$(newrelic --profile "$profile" --accountId "$account_id" nrql query --query "$query" --format JSON --plain 2>/dev/null || true)"
	count="$(printf '%s' "$result" | jq -r '.[0].count // 0' 2>/dev/null || printf '0')"
	if [[ "$count" =~ ^[1-9][0-9]*$ ]]; then
		printf 'New Relic acceptance PASS profile=%s eventType=%s marker=%s observedCount=%s retention=provider-managed\n' "$profile" "$event_type" "$marker" "$count"
		exit 0
	fi
	sleep "$poll_seconds"
done

printf 'New Relic custom event was accepted but was not visible through NRQL within %ss\n' "$wait_seconds" >&2
exit 1

#!/usr/bin/env bash
# Disposable live Go OTLP acceptance. The authenticated New Relic CLI creates
# one additional ingest key, the Go exporter/query verifier uses it for logs,
# metrics, and traces, and the exit trap revokes the key. No New Relic resource
# is owned after the run; the probe data remains under account retention.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_NEWRELIC_OTLP_ACCEPTANCE:?set MAGELIFT_NEWRELIC_OTLP_ACCEPTANCE=1 for live Go OTLP acceptance}"
if [[ "$MAGELIFT_NEWRELIC_OTLP_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live New Relic Go OTLP acceptance without MAGELIFT_NEWRELIC_OTLP_ACCEPTANCE=1\n' >&2
	exit 2
fi

profile="${MAGELIFT_NEWRELIC_PROFILE:-default}"
account_id="${MAGELIFT_NEWRELIC_ACCOUNT_ID:-}"
endpoint="${MAGELIFT_NEWRELIC_OTLP_ENDPOINT:-}"
nerdgraph_endpoint="${MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT:-}"
run_id="${MAGELIFT_NEWRELIC_OTLP_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_NEWRELIC_OTLP_MARKER:-magelift/acceptance/newrelic-otlp/${run_id}}"
if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$account_id" =~ ^[1-9][0-9]*$ || ! "$endpoint" =~ ^https://[^[:space:]]+$ || ! "$nerdgraph_endpoint" =~ ^https://[^[:space:]]+$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid New Relic profile, account, OTLP endpoint, NerdGraph endpoint, run ID, or marker\n' >&2
	exit 2
fi

acceptance_prepare_lifecycle
cleanup_marker_file="$(mktemp "${TMPDIR:-/tmp}/magelift-newrelic-otlp-acceptance.XXXXXX")"
key_id=""
api_key=""
query_key=""
cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	if [[ -n "$key_id" ]]; then
		delete_spec="$(jq -cn --arg id "$key_id" '{ingestKeyIds:[$id]}')"
		if ! newrelic --profile "$profile" --accountId "$account_id" apiAccess apiAccessDeleteKeys --keys "$delete_spec" --format JSON --plain >/dev/null 2>&1; then
			printf 'New Relic disposable ingest key revocation failed; key id=%s\n' "$key_id" >&2
			status=1
		fi
	fi
	if [[ -n "$cleanup_marker_file" ]]; then
		rm -f "$cleanup_marker_file"
	fi
	unset api_key query_key
	if (( status != 0 )); then
		printf 'New Relic Go OTLP acceptance failed; probe data marker=%s remains provider-retained\n' "$marker" >&2
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

acceptance_start_ttl_watchdog "${ACCEPTANCE_TTL_SECONDS:-1800}" "$cleanup_marker_file"
if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'newrelic Go OTLP acceptance dry-run ok profile=%s account=%s endpoint=%s nerdgraph=%s marker=%s\n' "$profile" "$account_id" "$endpoint" "$nerdgraph_endpoint" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands newrelic go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

query_spec='query { actor { apiAccess { keySearch(query: { types: USER }) { keys { key } } } } }'
query_key="$(newrelic --profile "$profile" --accountId "$account_id" nerdgraph query "$query_spec" --format JSON --plain | jq -r '.actor.apiAccess.keySearch.keys[] | select(.key != null and (.key | length > 0)) | .key' | head -1)"
if [[ -z "$query_key" ]]; then
	printf 'New Relic profile did not expose a usable user key for NerdGraph verification\n' >&2
	exit 2
fi

name="magelift-acceptance-${run_id}"
notes="Disposable MageLift Go OTLP acceptance marker=${marker}"
key_spec="$(jq -cn --argjson accountId "$account_id" --arg name "$name" --arg notes "$notes" '{ingest:[{accountId:$accountId,ingestType:"LICENSE",name:$name,notes:$notes}]}')"
created="$(newrelic --profile "$profile" --accountId "$account_id" apiAccess apiAccessCreateKeys --keys "$key_spec" --format JSON --plain)"
key_id="$(printf '%s' "$created" | jq -r --arg name "$name" '.[] | select(.name == $name) | .id' | head -1)"
api_key="$(printf '%s' "$created" | jq -r --arg name "$name" '.[] | select(.name == $name) | .key' | head -1)"
if [[ -z "$key_id" || -z "$api_key" ]]; then
	printf 'New Relic disposable ingest key creation returned no usable key identity; response was intentionally not printed\n' >&2
	exit 1
fi

# NerdGraph creates the key synchronously, but the regional OTLP edge can take
# a short time to recognize it. Probe only authentication with an empty
# request; do not send fixture telemetry until the key is accepted. A 400 is
# also an authenticated response for this readiness probe because the payload
# is intentionally empty.
key_propagation_timeout="${MAGELIFT_NEWRELIC_KEY_PROPAGATION_TIMEOUT_SECONDS:-90}"
key_propagation_poll="${MAGELIFT_NEWRELIC_KEY_PROPAGATION_POLL_SECONDS:-5}"
if [[ ! "$key_propagation_timeout" =~ ^[1-9][0-9]*$ || ! "$key_propagation_poll" =~ ^[1-9][0-9]*$ ]]; then
	printf 'invalid New Relic key propagation timeout or poll interval\n' >&2
	exit 2
fi
key_propagation_deadline=$(( $(date +%s) + key_propagation_timeout ))
key_ready=0
while (( $(date +%s) < key_propagation_deadline )); do
	key_http_code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -X POST \
		-H 'Content-Type: application/x-protobuf' -H "api-key: $api_key" --data-binary '' \
		"${endpoint%/}/v1/metrics" 2>/dev/null || printf '000')"
	case "$key_http_code" in
	200|201|202|400|404|405|411|413|415)
		key_ready=1
		break
		;;
	401|403|000)
		if (( $(date +%s) + key_propagation_poll >= key_propagation_deadline )); then
			break
		fi
		sleep "$key_propagation_poll"
		;;
	*)
		printf 'New Relic OTLP key readiness returned an unexpected HTTP status; provider response was not printed\n' >&2
		exit 1
		;;
	esac
done
if (( key_ready == 0 )); then
	printf 'New Relic disposable ingest key was not accepted by the OTLP endpoint within %ss\n' "$key_propagation_timeout" >&2
	exit 1
fi

MAGELIFT_NEWRELIC_ACCOUNT_ID="$account_id" \
MAGELIFT_NEWRELIC_OTLP_ENDPOINT="$endpoint" \
MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT="$nerdgraph_endpoint" \
MAGELIFT_NEWRELIC_OTLP_API_KEY="$api_key" \
MAGELIFT_NEWRELIC_NERDGRAPH_API_KEY="$query_key" \
MAGELIFT_NEWRELIC_OTLP_MARKER="$marker" \
	go run ./cmd/newrelic-otlp-acceptance

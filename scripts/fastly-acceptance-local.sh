#!/usr/bin/env bash
# Disposable Fastly adapter acceptance. The Go test owns the Fastly service,
# backend, domain, purge, and service cleanup; this wrapper owns the explicit
# live gate, Cloudflare DNS record, run marker, TTL watchdog, and DNS cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands fastly cf || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_FASTLY_ACCEPTANCE:?set MAGELIFT_FASTLY_ACCEPTANCE=1 for a disposable live Fastly run}"
if [[ "$MAGELIFT_FASTLY_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live Fastly acceptance without MAGELIFT_FASTLY_ACCEPTANCE=1\n' >&2
	exit 2
fi

# Do not use `fastly whoami` here: an unauthenticated invocation can start a
# browser OAuth flow. Do not default MAGELIFT_FASTLY_TOKEN_NAME to `default`:
# `--token default` selects a named stored token, which is not the same
# credential as the CLI login session used when `--token` is omitted.
# FASTLY_API_TOKEN keeps the CLI's documented environment-variable precedence.
# Never print token values; never run `fastly profile list` or `fastly auth token`.
fastly_acceptance_cli() {
	if [[ -n "${FASTLY_API_TOKEN:-}" ]]; then
		fastly "$@"
		return
	fi
	if [[ -n "${MAGELIFT_FASTLY_TOKEN_NAME:-}" ]]; then
		if [[ "$MAGELIFT_FASTLY_TOKEN_NAME" == *$'\n'* || "$MAGELIFT_FASTLY_TOKEN_NAME" == *$'\r'* ]]; then
			printf 'MAGELIFT_FASTLY_TOKEN_NAME must be a single-line token name\n' >&2
			return 2
		fi
		fastly --token "$MAGELIFT_FASTLY_TOKEN_NAME" "$@"
		return
	fi
	fastly "$@"
}

if ! fastly_acceptance_cli service list --non-interactive --quiet --json >/dev/null 2>&1; then
	printf 'an authenticated Fastly API token is required; configure FASTLY_API_TOKEN, MAGELIFT_FASTLY_TOKEN_NAME for a stored named token, or a Fastly CLI login session\n' >&2
	exit 2
fi

run_id="${MAGELIFT_FASTLY_ACCEPTANCE_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
domain="${MAGELIFT_FASTLY_ACCEPTANCE_DOMAIN:?set the exact disposable verified Fastly domain}"
cname_target="${MAGELIFT_FASTLY_ACCEPTANCE_CNAME_TARGET:?set the exact Fastly CNAME target assigned for this acceptance route}"
origin_address="${MAGELIFT_FASTLY_ACCEPTANCE_ORIGIN_ADDRESS:?set the stable origin hostname for this acceptance route}"
origin_host="${MAGELIFT_FASTLY_ACCEPTANCE_ORIGIN_HOST:-$origin_address}"
origin_url="${MAGELIFT_FASTLY_ACCEPTANCE_ORIGIN_URL:?set the stable origin health URL for this acceptance route}"
route_path="${MAGELIFT_FASTLY_ACCEPTANCE_ROUTE_PATH:-/}"
origin_use_tls="${MAGELIFT_FASTLY_ACCEPTANCE_ORIGIN_USE_TLS:-1}"
tls_enabled="${MAGELIFT_FASTLY_ACCEPTANCE_TLS:-1}"
expected_status="${MAGELIFT_FASTLY_ACCEPTANCE_EXPECTED_STATUS:-200}"
route_timeout_seconds="${MAGELIFT_FASTLY_ACCEPTANCE_ROUTE_TIMEOUT_SECONDS:-300}"
marker="magelift/acceptance/live-${run_id}"
dns_marker="magelift/acceptance/dns/${run_id}"
tls_dns_marker="magelift/acceptance/tls-dns/${run_id}"
if [[ ! "$run_id" =~ ^[a-zA-Z0-9._-]+$ || ! "$domain" =~ ^[a-z0-9.-]+$ || ! "$origin_address" =~ ^[a-zA-Z0-9.:-]+$ || ! "$origin_host" =~ ^[a-zA-Z0-9.:-]+$ || ! "$origin_url" =~ ^https?://[^[:space:]]+$ || ! "$route_path" =~ ^/[^[:space:]]*$ || ! "$expected_status" =~ ^[1-5][0-9][0-9]$ || ! "$route_timeout_seconds" =~ ^[1-9][0-9]*$ || "$origin_use_tls" != 0 && "$origin_use_tls" != 1 || "$tls_enabled" != 0 && "$tls_enabled" != 1 ]]; then
	printf 'invalid Fastly acceptance run ID or domain\n' >&2
	exit 2
fi
if [[ "$tls_enabled" != 1 ]]; then
	printf 'the current Fastly versionless-domain acceptance path requires managed TLS; use a separately verified classic-domain account for non-TLS tests\n' >&2
	exit 2
fi

cloudflare_acceptance_dns_init
	existing_tls_count="$(fastly_acceptance_cli tls-subscription list --json --filter-domain "$domain" 2>/dev/null | jq 'length' 2>/dev/null || printf '0')"
if [[ ! "$existing_tls_count" =~ ^[0-9]+$ ]] || (( existing_tls_count != 0 )); then
	printf 'refusing Fastly acceptance: the disposable domain already has a TLS subscription\n' >&2
	exit 2
fi
acceptance_prepare_lifecycle
cleanup_marker_file="$(mktemp "${TMPDIR:-/tmp}/magelift-fastly-acceptance.XXXXXX")"
cleanup_status=0
cleanup_done=0
cleanup() {
	local status=$?
	if (( cleanup_done == 1 )); then
		exit "$status"
	fi
	cleanup_done=1
	acceptance_stop_ttl_watchdog || cleanup_status=$?
	if (( status != 0 )); then
		fastly_acceptance_cleanup_owned || cleanup_status=1
	fi
	if ! cloudflare_acceptance_dns_cleanup "$domain" "$dns_marker" "$cname_target"; then
		cleanup_status=1
	fi
	if ! cloudflare_acceptance_dns_cleanup "_acme-challenge.$domain" "$tls_dns_marker"; then
		cleanup_status=1
	fi
	rm -f "$cleanup_marker_file"
	if (( status != 0 )); then
		exit "$status"
	fi
	exit "$cleanup_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

fastly_acceptance_cleanup_owned() {
	local domain_json tls_json service_json domain_id tls_id service_id
	local -a domain_ids=() tls_ids=() service_ids=()

	domain_json="$(fastly_acceptance_cli domain list --json --fqdn "$domain" 2>/dev/null || true)"
	while IFS= read -r domain_id; do
		[[ -z "$domain_id" ]] || domain_ids+=("$domain_id")
	done < <(printf '%s' "$domain_json" | jq -r --arg marker "$marker" '.data[]? | select(.description == $marker) | .id')
	while IFS= read -r service_id; do
		[[ -z "$service_id" ]] || service_ids+=("$service_id")
	done < <(printf '%s' "$domain_json" | jq -r --arg marker "$marker" '.data[]? | select(.description == $marker) | .service_id')

	# A managed TLS subscription is a dependency of the versionless domain.
	# The adapter normally records its ID; this marker-scoped fallback handles
	# a process crash before the Go test can run its deferred cleanup.
	tls_json="$(fastly_acceptance_cli tls-subscription list --json --filter-domain "$domain" 2>/dev/null || true)"
	while IFS= read -r tls_id; do
		[[ -z "$tls_id" ]] || tls_ids+=("$tls_id")
	done < <(printf '%s' "$tls_json" | jq -r '.[].ID // .[].id // empty')
	for tls_id in "${tls_ids[@]}"; do
		fastly_acceptance_cli tls-subscription delete --id "$tls_id" --force --non-interactive >/dev/null 2>&1 || true
	done
	for domain_id in "${domain_ids[@]}"; do
		fastly_acceptance_cli domain delete --domain-id "$domain_id" --non-interactive >/dev/null 2>&1 || true
	done

	service_json="$(fastly_acceptance_cli service list --json 2>/dev/null || true)"
	while IFS= read -r service_id; do
		[[ -z "$service_id" ]] || service_ids+=("$service_id")
	done < <(printf '%s' "$service_json" | jq -r --arg marker "$marker" '.[]? | select(.Comment == $marker or .comment == $marker) | .ServiceID // .service_id // .id')
	if ((${#service_ids[@]} > 0)); then
		while IFS= read -r service_id; do
			[[ -z "$service_id" ]] || fastly_acceptance_cli service delete --force --service-id "$service_id" --non-interactive >/dev/null 2>&1 || true
		done < <(printf '%s\n' "${service_ids[@]}" | sort -u)
	fi
}

cloudflare_acceptance_dns_prepare "$domain" "$cname_target" "$dns_marker"
cloudflare_acceptance_dns_wait_for_cname "$domain" "$cname_target"
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$cleanup_marker_file"

cd "$ROOT"
live_status=0
MAGELIFT_FASTLY_LIVE=1 \
	MAGELIFT_FASTLY_LIVE_DOMAIN="$domain" \
	MAGELIFT_FASTLY_LIVE_MARKER="$marker" \
	MAGELIFT_FASTLY_LIVE_CNAME_TARGET="$cname_target" \
	MAGELIFT_FASTLY_LIVE_ORIGIN_ADDRESS="$origin_address" \
	MAGELIFT_FASTLY_LIVE_ORIGIN_HOST="$origin_host" \
	MAGELIFT_FASTLY_LIVE_ORIGIN_URL="$origin_url" \
	MAGELIFT_FASTLY_LIVE_ROUTE_PATH="$route_path" \
	MAGELIFT_FASTLY_LIVE_ORIGIN_USE_TLS="$origin_use_tls" \
	MAGELIFT_FASTLY_LIVE_TLS="$tls_enabled" \
	MAGELIFT_FASTLY_LIVE_EXPECTED_STATUS="$expected_status" \
	MAGELIFT_FASTLY_LIVE_ROUTE_TIMEOUT_SECONDS="$route_timeout_seconds" \
	MAGELIFT_FASTLY_LIVE_TLS_DNS_MARKER="$tls_dns_marker" \
	MAGELIFT_FASTLY_LIVE_CLOUDFLARE_PROFILE="$CLOUDFLARE_ACCEPTANCE_PROFILE" \
	MAGELIFT_FASTLY_LIVE_CLOUDFLARE_ZONE="$CLOUDFLARE_ACCEPTANCE_ZONE" \
	go test ./internal/external/fastly -run '^TestLiveFastlyAdapter$' -count=1 -v || live_status=$?
if (( live_status != 0 )); then
	exit "$live_status"
fi

printf 'Fastly acceptance complete; verify direct inventory is empty for marker=%s domain=%s\n' "$marker" "$domain"

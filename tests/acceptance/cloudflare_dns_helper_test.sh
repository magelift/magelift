#!/usr/bin/env bash
# Offline contract test for the ownership-scoped Cloudflare acceptance helper.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/acceptance/lib-cloudflare-dns.sh
source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"

FAIL=0
STATE_FILE="$(mktemp)"
trap 'rm -f "$STATE_FILE"' EXIT

assert_eq() {
	local label="$1" got="$2" want="$3"
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL %s: got=%q want=%q\n' "$label" "$got" "$want" >&2
		FAIL=1
	else
		printf 'ok %s\n' "$label"
	fi
}

assert_fails() {
	local label="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		printf 'FAIL %s: expected failure\n' "$label" >&2
		FAIL=1
	else
		printf 'ok %s\n' "$label"
	fi
}

CLOUDFLARE_ACCEPTANCE_ZONE=acourtiol.com
CLOUDFLARE_ACCEPTANCE_PROFILE=default
CLOUDFLARE_ACCEPTANCE_TTL_SECONDS=60
CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS=1
CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS=1

cloudflare_acceptance_dns_cli() {
	local operation="$*"
	case "$operation" in
	*"dns records list"*)
		if [[ -s "$STATE_FILE" ]]; then
			printf '%s\n' "$(<"$STATE_FILE")"
		else
			printf '{"result":[]}\n'
		fi
		;;
	*"dns records create"*)
		if [[ "${CLOUDFLARE_TEST_CREATE_A:-0}" == 1 ]]; then
			printf '%s\n' '{"result":[{"id":"cf-record-a-1","name":"m249p-gcp-origin-test.acourtiol.com","type":"A","content":"203.0.113.50","comment":"magelift/acceptance/dns/a-test-1"}]}' >"$STATE_FILE"
			printf '%s\n' '{"result":{"id":"cf-record-a-1"}}'
		else
			printf '%s\n' '{"result":[{"id":"cf-record-1","name":"m249p-fastly-live-test.acourtiol.com","type":"CNAME","content":"dualstack.nonssl.global.fastly.net","comment":"magelift/acceptance/dns/test-1"}]}' >"$STATE_FILE"
			printf '%s\n' '{"result":{"id":"cf-record-1"}}'
		fi
		;;
	*"dns records delete cf-record-1"*)
		: >"$STATE_FILE"
		printf '%s\n' '{"result":{"id":"cf-record-1"}}'
		;;
	*"dns records delete cf-record-a-1"*)
		: >"$STATE_FILE"
		printf '%s\n' '{"result":{"id":"cf-record-a-1"}}'
		;;
	*)
		printf 'unexpected fake Cloudflare call: %s\n' "$operation" >&2
		return 1
		;;
	esac
}

domain="m249p-fastly-live-test.acourtiol.com"
target="dualstack.nonssl.global.fastly.net"
marker="magelift/acceptance/dns/test-1"

cloudflare_acceptance_dns_prepare "$domain" "$target" "$marker"
assert_eq "created-record-id" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID" "cf-record-1"
assert_eq "created-record-flag" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED" "1"

cloudflare_acceptance_dns_prepare "$domain" "$target." "$marker"
assert_eq "reused-record-id" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID" "cf-record-1"
assert_eq "reused-record-flag" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED" "0"

cloudflare_acceptance_dns_cleanup "$domain" "$marker" "$target"
assert_eq "cleanup-state" "$(<"$STATE_FILE")" ""

a_domain="m249p-gcp-origin-test.acourtiol.com"
a_marker="magelift/acceptance/dns/a-test-1"
a_address="203.0.113.50"
CLOUDFLARE_TEST_CREATE_A=1
cloudflare_acceptance_dns_prepare_a "$a_domain" "$a_address" "$a_marker"
assert_eq "created-a-record-id" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID" "cf-record-a-1"
assert_eq "created-a-record-flag" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED" "1"
cloudflare_acceptance_dns_prepare_a "$a_domain" "$a_address" "$a_marker"
assert_eq "reused-a-record-id" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID" "cf-record-a-1"
assert_eq "reused-a-record-flag" "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED" "0"
cloudflare_acceptance_dns_cleanup_a "$a_domain" "$a_marker" "$a_address"
assert_eq "cleanup-a-state" "$(<"$STATE_FILE")" ""

printf '%s\n' '{"result":[{"id":"foreign","name":"m249p-fastly-live-test.acourtiol.com","type":"CNAME","content":"other.example.net","comment":"operator-owned"}]}' >"$STATE_FILE"
assert_fails "foreign-record-refused" cloudflare_acceptance_dns_prepare "$domain" "$target" "$marker"
assert_eq "foreign-record-preserved" "$(<"$STATE_FILE")" '{"result":[{"id":"foreign","name":"m249p-fastly-live-test.acourtiol.com","type":"CNAME","content":"other.example.net","comment":"operator-owned"}]}'

assert_fails "apex-refused" cloudflare_acceptance_dns_validate_domain acourtiol.com
assert_fails "wrong-zone-refused" cloudflare_acceptance_dns_validate_domain example.com
if cloudflare_acceptance_dns_validate_domain "_acme-challenge.$domain"; then
	printf 'ok acme-challenge-domain-accepted\n'
else
	printf 'FAIL acme-challenge-domain-accepted\n' >&2
	FAIL=1
fi
assert_fails "marker-refused" cloudflare_acceptance_dns_validate_marker operator/test
assert_fails "target-refused" cloudflare_acceptance_dns_validate_target "https://fastly.example.net"

if [[ "$FAIL" -ne 0 ]]; then
	printf 'cloudflare_dns_helper_test: FAILED\n' >&2
	exit 1
fi
printf 'cloudflare_dns_helper_test: ok\n'

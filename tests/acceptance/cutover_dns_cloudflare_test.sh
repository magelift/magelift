#!/usr/bin/env bash
# Offline self-test for scripts/cutover-dns-cloudflare.sh (MIGRATE-04 / D-04).
# Stubs curl via CURL_BIN; never calls production Cloudflare.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT}/scripts/cutover-dns-cloudflare.sh"
FAIL=0

assert_eq() {
	local label="$1" got="$2" want="$3"
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL %s: got=%q want=%q\n' "$label" "$got" "$want" >&2
		FAIL=1
	else
		printf 'ok %s\n' "$label"
	fi
}

assert_contains() {
	local label="$1" hay="$2" needle="$3"
	if [[ "$hay" != *"$needle"* ]]; then
		printf 'FAIL %s: missing %q in:\n%s\n' "$label" "$needle" "$hay" >&2
		FAIL=1
	else
		printf 'ok %s\n' "$label"
	fi
}

assert_exit() {
	local label="$1" want="$2"
	shift 2
	set +e
	"$@" >/tmp/cutover-dns-test-out.$$ 2>/tmp/cutover-dns-test-err.$$
	local got=$?
	set -e
	if [[ "$got" -ne "$want" ]]; then
		printf 'FAIL %s: exit=%s want=%s\nstdout:\n%s\nstderr:\n%s\n' \
			"$label" "$got" "$want" "$(cat /tmp/cutover-dns-test-out.$$)" "$(cat /tmp/cutover-dns-test-err.$$)" >&2
		FAIL=1
	else
		printf 'ok %s (exit %s)\n' "$label" "$want"
	fi
}

cleanup() {
	rm -f /tmp/cutover-dns-test-out.$$ /tmp/cutover-dns-test-err.$$ \
		/tmp/cutover-dns-mock-log.$$ /tmp/cutover-dns-fake-curl.$$
}
trap cleanup EXIT

# --- missing token (not dry-run) must fail on curl path (force-curl skips live `cf` CLI) ---
unset CLOUDFLARE_API_TOKEN CF_API_TOKEN || true
assert_exit "missing-token-fails" 1 env -u CLOUDFLARE_API_TOKEN -u CF_API_TOKEN \
	MAGELIFT_CUTOVER_FORCE_CURL=1 TARGET=example.invalid "$SCRIPT"

# --- dry-run works without token ---
assert_exit "dry-run-no-token" 0 env -u CLOUDFLARE_API_TOKEN -u CF_API_TOKEN \
	MAGELIFT_CUTOVER_HOST=magelift-preview.example.com \
	TARGET=example.invalid "$SCRIPT" --dry-run

# --- fake curl stub: zone resolve + create + update + cleanup ---
FAKE_CURL="/tmp/cutover-dns-fake-curl.$$"
MOCK_LOG="/tmp/cutover-dns-mock-log.$$"
: >"$MOCK_LOG"

cat >"$FAKE_CURL" <<'FAKE'
#!/usr/bin/env bash
set -Eeuo pipefail
LOG="${CUTOVER_DNS_MOCK_LOG:?}"
# Reconstruct a stable log line from argv (method, url, optional body).
method="GET"
url=""
body=""
auth=""
prev=""
for a in "$@"; do
	case "$prev" in
	-X) method="$a" ;;
	--data) body="$a" ;;
	-H)
		case "$a" in
		Authorization:\ Bearer\ *) auth="$a" ;;
		esac
		;;
	esac
	if [[ "$a" == https://* || "$a" == http://* ]]; then
		url="$a"
	fi
	prev="$a"
done
if [[ -z "$auth" ]]; then
	printf '{"success":false,"errors":[{"message":"missing Authorization Bearer"}]}\n' >&2
	exit 1
fi
printf '%s %s' "$method" "$url" >>"$LOG"
if [[ -n "$body" ]]; then
	printf ' body=%s' "$body" >>"$LOG"
fi
printf '\n' >>"$LOG"

case "$method $url" in
"GET https://api.cloudflare.com/client/v4/zones?name=example.com")
	printf '{"success":true,"result":[{"id":"zone-abc","name":"example.com"}]}\n'
	;;
"GET https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records?name=magelift-preview.example.com")
	# First list: empty (create path). Later calls: existing record (update/cleanup).
	count="$(grep -c 'dns_records?name=magelift-preview' "$LOG" || true)"
	if [[ "$count" -le 1 ]]; then
		printf '{"success":true,"result":[]}\n'
	else
		printf '{"success":true,"result":[{"id":"rec-1","name":"magelift-preview.example.com","type":"CNAME"}]}\n'
	fi
	;;
"POST https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records")
	printf '{"success":true,"result":{"id":"rec-1","name":"magelift-preview.example.com"}}\n'
	;;
"PUT https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records/rec-1")
	printf '{"success":true,"result":{"id":"rec-1"}}\n'
	;;
"DELETE https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records/rec-1")
	printf '{"success":true,"result":{"id":"rec-1"}}\n'
	;;
*)
	printf '{"success":false,"errors":[{"message":"unexpected mock call method=%s url=%s"}]}\n' "$method" "$url" >&2
	printf '{"success":false,"errors":[{"message":"unexpected mock call"}]}\n'
	exit 1
	;;
esac
FAKE
chmod +x "$FAKE_CURL"

export CLOUDFLARE_API_TOKEN="test-token-not-real"
export CF_API_TOKEN=""
export CURL_BIN="$FAKE_CURL"
export CUTOVER_DNS_MOCK_LOG="$MOCK_LOG"
export MAGELIFT_CUTOVER_HOST=magelift-preview.example.com

: >"$MOCK_LOG"
out="$(TARGET=lb.example.invalid "$SCRIPT" 2>&1)"
assert_contains "create-log-zone" "$(cat "$MOCK_LOG")" "GET https://api.cloudflare.com/client/v4/zones?name=example.com"
assert_contains "create-log-post" "$(cat "$MOCK_LOG")" 'POST https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records body={"type":"CNAME","name":"magelift-preview.example.com","content":"lb.example.invalid","ttl":120,"proxied":false}'
assert_contains "create-ok" "$out" "upsert ok"

# Second upsert → update existing
out="$(TARGET=203.0.113.50 "$SCRIPT" 2>&1)"
assert_contains "update-log-put" "$(cat "$MOCK_LOG")" 'PUT https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records/rec-1 body={"type":"A","name":"magelift-preview.example.com","content":"203.0.113.50","ttl":120,"proxied":false}'
assert_contains "update-ok" "$out" "upsert ok"

# Cleanup deletes by name
out="$("$SCRIPT" --cleanup 2>&1)"
assert_contains "cleanup-log-delete" "$(cat "$MOCK_LOG")" "DELETE https://api.cloudflare.com/client/v4/zones/zone-abc/dns_records/rec-1"
assert_contains "cleanup-ok" "$out" "cleanup ok"

# CF_API_TOKEN alias works when CLOUDFLARE_API_TOKEN unset (Bearer still required by stub)
: >"$MOCK_LOG"
unset CLOUDFLARE_API_TOKEN
export CF_API_TOKEN="alt-token-not-real"
assert_exit "cf-api-token-alias" 0 env -u CLOUDFLARE_API_TOKEN CF_API_TOKEN=alt-token-not-real \
	CURL_BIN="$FAKE_CURL" CUTOVER_DNS_MOCK_LOG="$MOCK_LOG" \
	MAGELIFT_CUTOVER_HOST=magelift-preview.example.com \
	TARGET=cname.example.invalid "$SCRIPT"
assert_contains "cf-api-token-zone" "$(cat "$MOCK_LOG")" "zones?name=example.com"

if [[ "$FAIL" -ne 0 ]]; then
	printf 'cutover-dns-cloudflare_test: FAILED\n' >&2
	exit 1
fi
printf 'cutover-dns-cloudflare_test: ok\n'
exit 0

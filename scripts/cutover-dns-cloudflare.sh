#!/usr/bin/env bash
# Upsert or delete Cloudflare DNS for MageLift cutover rehearsal (MIGRATE-04 / D-04).
#
# Auth preference:
#   1) `cf` CLI (Cloudflare CLI) when available — uses `cf auth` OAuth with
#      dns_records:edit (this is the maintainer path; Wrangler does NOT manage DNS).
#   2) CLOUDFLARE_API_TOKEN / CF_API_TOKEN Bearer via curl (CI / token path).
#   3) When CURL_BIN is overridden (acceptance mocks), always use curl path.
#
# Usage:
#   TARGET=<hostname|IPv4> ./scripts/cutover-dns-cloudflare.sh [--dry-run] [--proxied]
#   ./scripts/cutover-dns-cloudflare.sh --cleanup [--dry-run]
#
# Env:
#   MAGELIFT_CUTOVER_HOST  default magelift-preview.alexandrecourtiol.com
#   CURL_BIN               curl binary override (acceptance mocks → forces token/curl path)
#   CF_API_BASE            API base (default https://api.cloudflare.com/client/v4)
#   MAGELIFT_CUTOVER_FORCE_CURL=1  skip cf CLI even if installed
set -Eeuo pipefail

CF_API_BASE="${CF_API_BASE:-https://api.cloudflare.com/client/v4}"
CURL_BIN="${CURL_BIN:-curl}"
HOST="${MAGELIFT_CUTOVER_HOST:-magelift-preview.alexandrecourtiol.com}"
TTL="${MAGELIFT_CUTOVER_TTL:-120}"
DRY_RUN=0
CLEANUP=0
PROXIED=false
TARGET="${TARGET:-}"

usage() {
	cat >&2 <<'EOF'
Usage:
  TARGET=<hostname|IPv4> cutover-dns-cloudflare.sh [--dry-run] [--proxied]
  cutover-dns-cloudflare.sh --cleanup [--dry-run]

Prefers authenticated `cf` CLI (dns_records:edit). Falls back to
CLOUDFLARE_API_TOKEN / CF_API_TOKEN. Wrangler OAuth cannot write DNS.
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--cleanup)
		CLEANUP=1
		shift
		;;
	--proxied)
		PROXIED=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'unknown flag: %s\n' "$1" >&2
		usage
		exit 2
		;;
	esac
done

token="${CLOUDFLARE_API_TOKEN:-${CF_API_TOKEN:-}}"

# Use curl/token path when CURL_BIN is overridden (tests) or forced.
use_cf_cli=0
if [[ "${MAGELIFT_CUTOVER_FORCE_CURL:-}" != "1" && "$CURL_BIN" == "curl" && "$DRY_RUN" -eq 0 ]]; then
	if command -v cf >/dev/null 2>&1; then
		if cf auth whoami 2>/dev/null | grep -q 'dns_records:edit'; then
			use_cf_cli=1
		fi
	fi
fi

if [[ "$use_cf_cli" -eq 0 && -z "$token" && "$DRY_RUN" -eq 0 ]]; then
	printf 'cutover-dns-cloudflare: need `cf auth login` (dns_records:edit) or CLOUDFLARE_API_TOKEN.\n' >&2
	printf 'Hint: wrangler whoami is Workers-only; DNS is via `cf dns records …`.\n' >&2
	exit 1
fi

if [[ "$CLEANUP" -eq 0 && -z "$TARGET" ]]; then
	printf 'cutover-dns-cloudflare: TARGET is required unless --cleanup\n' >&2
	usage
	exit 2
fi

zone_for_host() {
	local h="$1"
	case "$h" in
	*.alexandrecourtiol.com | alexandrecourtiol.com) printf '%s' alexandrecourtiol.com ;;
	*.acourtiol.com | acourtiol.com) printf '%s' acourtiol.com ;;
	*) printf '%s' alexandrecourtiol.com ;;
	esac
}

record_type_for_target() {
	local t="$1"
	if [[ "$t" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
		printf 'A'
	else
		printf 'CNAME'
	fi
}

normalize_cname_content() {
	local t="$1"
	t="${t%.}"
	printf '%s' "$t"
}

# --- cf CLI path -----------------------------------------------------------

cf_list_record_id() {
	local zone="$1"
	local raw id
	raw="$(cf dns records list --zone "$zone" --name "$HOST" 2>/dev/null || true)"
	id="$(printf '%s' "$raw" | python3 -c "import sys,json
try:
  d=json.load(sys.stdin)
except Exception:
  sys.exit(0)
items=d if isinstance(d,list) else (d.get('result') or [])
print(items[0]['id'] if items else '')" 2>/dev/null || true)"
	printf '%s' "$id"
}

upsert_via_cf() {
	local zone rtype content body existing
	zone="$(zone_for_host "$HOST")"
	rtype="$(record_type_for_target "$TARGET")"
	content="$TARGET"
	if [[ "$rtype" == "CNAME" ]]; then
		content="$(normalize_cname_content "$TARGET")"
	fi
	body="$(printf '{"type":"%s","name":"%s","content":"%s","ttl":%s,"proxied":%s}' \
		"$rtype" "$HOST" "$content" "$TTL" "$PROXIED")"
	existing="$(cf_list_record_id "$zone" || true)"
	if [[ -n "$existing" ]]; then
		printf 'cutover-dns-cloudflare: cf update %s %s -> %s (id=%s)\n' "$rtype" "$HOST" "$content" "$existing" >&2
		cf dns records update --zone "$zone" "$existing" --body "$body" >/dev/null
	else
		printf 'cutover-dns-cloudflare: cf create %s %s -> %s\n' "$rtype" "$HOST" "$content" >&2
		cf dns records create --zone "$zone" --body "$body" >/dev/null
	fi
	printf 'cutover-dns-cloudflare: upsert ok host=%s type=%s target=%s (via cf)\n' "$HOST" "$rtype" "$content"
}

cleanup_via_cf() {
	local zone existing
	zone="$(zone_for_host "$HOST")"
	existing="$(cf_list_record_id "$zone" || true)"
	if [[ -z "$existing" ]]; then
		printf 'cutover-dns-cloudflare: no record named %s; nothing to delete\n' "$HOST"
		return 0
	fi
	printf 'cutover-dns-cloudflare: cf delete %s (id=%s)\n' "$HOST" "$existing" >&2
	cf dns records delete --zone "$zone" "$existing" --force >/dev/null
	printf 'cutover-dns-cloudflare: cleanup ok host=%s (via cf)\n' "$HOST"
}

# --- curl / API token path -------------------------------------------------

zone_candidates_for_host() {
	local h="$1"
	case "$h" in
	*.alexandrecourtiol.com | alexandrecourtiol.com)
		printf '%s\n' alexandrecourtiol.com
		;;
	*.acourtiol.com | acourtiol.com)
		printf '%s\n' acourtiol.com alexandrecourtiol.com
		;;
	*)
		printf '%s\n' alexandrecourtiol.com acourtiol.com
		;;
	esac
}

cf_curl() {
	local method="$1"
	local path="$2"
	local body="${3:-}"
	local url="${CF_API_BASE}${path}"

	if [[ "$DRY_RUN" -eq 1 ]]; then
		if [[ -n "$body" ]]; then
			printf 'DRY-RUN %s %s body=%s\n' "$method" "$url" "$body" >&2
		else
			printf 'DRY-RUN %s %s\n' "$method" "$url" >&2
		fi
		if [[ "$path" == "/zones" || "$path" == /zones\?* ]]; then
			printf '{"success":true,"result":[{"id":"dry-run-zone","name":"alexandrecourtiol.com"}]}\n'
		elif [[ "$path" == */dns_records/* ]]; then
			printf '{"success":true,"result":{"id":"dry-run-record"}}\n'
		elif [[ "$path" == */dns_records || "$path" == */dns_records\?* ]]; then
			if [[ "$method" == "GET" ]]; then
				printf '{"success":true,"result":[]}\n'
			else
				printf '{"success":true,"result":{"id":"dry-run-record","name":"%s"}}\n' "$HOST"
			fi
		else
			printf '{"success":true,"result":{}}\n'
		fi
		return 0
	fi

	local -a args=(-sS -X "$method" "$url"
		-H "Authorization: Bearer ${token}"
		-H "Content-Type: application/json")
	if [[ -n "$body" ]]; then
		args+=(--data "$body")
	fi
	"$CURL_BIN" "${args[@]}"
}

json_success() {
	printf '%s' "$1" | grep -q '"success"[[:space:]]*:[[:space:]]*true'
}

extract_first_id() {
	local raw="$1"
	local id
	id="$(printf '%s' "$raw" | sed -n 's/.*"result"[[:space:]]*:[[:space:]]*\[[[:space:]]*{[^}]*"id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
	if [[ -z "$id" ]]; then
		id="$(printf '%s' "$raw" | sed -n 's/.*"result"[[:space:]]*:[[:space:]]*{[^}]*"id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
	fi
	printf '%s' "$id"
}

resolve_zone_id() {
	local zone_name zone_resp zone_id
	while IFS= read -r zone_name; do
		[[ -z "$zone_name" ]] && continue
		zone_resp="$(cf_curl GET "/zones?name=${zone_name}")"
		if ! json_success "$zone_resp"; then
			printf 'cutover-dns-cloudflare: zone lookup failed for %s: %s\n' "$zone_name" "$zone_resp" >&2
			continue
		fi
		zone_id="$(extract_first_id "$zone_resp")"
		if [[ -n "$zone_id" ]]; then
			printf '%s' "$zone_id"
			return 0
		fi
	done < <(zone_candidates_for_host "$HOST")
	printf 'cutover-dns-cloudflare: could not resolve zone id for host %s\n' "$HOST" >&2
	return 1
}

find_record_id() {
	local zone_id="$1"
	local enc_name
	enc_name="$(printf '%s' "$HOST" | sed 's/ /%20/g')"
	local resp
	resp="$(cf_curl GET "/zones/${zone_id}/dns_records?name=${enc_name}")"
	if ! json_success "$resp"; then
		printf 'cutover-dns-cloudflare: list dns_records failed: %s\n' "$resp" >&2
		return 1
	fi
	extract_first_id "$resp"
}

upsert_via_curl() {
	local zone_id rtype content body resp existing
	zone_id="$(resolve_zone_id)"
	rtype="$(record_type_for_target "$TARGET")"
	content="$TARGET"
	if [[ "$rtype" == "CNAME" ]]; then
		content="$(normalize_cname_content "$TARGET")"
	fi
	body="$(printf '{"type":"%s","name":"%s","content":"%s","ttl":%s,"proxied":%s}' \
		"$rtype" "$HOST" "$content" "$TTL" "$PROXIED")"

	existing="$(find_record_id "$zone_id" || true)"
	if [[ -n "$existing" ]]; then
		printf 'cutover-dns-cloudflare: updating %s %s -> %s (id=%s)\n' "$rtype" "$HOST" "$content" "$existing" >&2
		resp="$(cf_curl PUT "/zones/${zone_id}/dns_records/${existing}" "$body")"
	else
		printf 'cutover-dns-cloudflare: creating %s %s -> %s\n' "$rtype" "$HOST" "$content" >&2
		resp="$(cf_curl POST "/zones/${zone_id}/dns_records" "$body")"
	fi
	if ! json_success "$resp"; then
		printf 'cutover-dns-cloudflare: upsert failed: %s\n' "$resp" >&2
		return 1
	fi
	printf 'cutover-dns-cloudflare: upsert ok host=%s type=%s target=%s\n' "$HOST" "$rtype" "$content"
}

cleanup_via_curl() {
	local zone_id existing resp
	zone_id="$(resolve_zone_id)"
	existing="$(find_record_id "$zone_id" || true)"
	if [[ -z "$existing" ]]; then
		printf 'cutover-dns-cloudflare: no record named %s; nothing to delete\n' "$HOST"
		return 0
	fi
	printf 'cutover-dns-cloudflare: deleting %s (id=%s)\n' "$HOST" "$existing" >&2
	resp="$(cf_curl DELETE "/zones/${zone_id}/dns_records/${existing}")"
	if ! json_success "$resp"; then
		printf 'cutover-dns-cloudflare: delete failed: %s\n' "$resp" >&2
		return 1
	fi
	printf 'cutover-dns-cloudflare: cleanup ok host=%s\n' "$HOST"
}

if [[ "$use_cf_cli" -eq 1 ]]; then
	if [[ "$CLEANUP" -eq 1 ]]; then
		cleanup_via_cf
	else
		upsert_via_cf
	fi
else
	if [[ "$CLEANUP" -eq 1 ]]; then
		cleanup_via_curl
	else
		upsert_via_curl
	fi
fi

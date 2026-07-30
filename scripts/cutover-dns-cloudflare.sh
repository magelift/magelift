#!/usr/bin/env bash
# Upsert or delete Cloudflare DNS for MageLift cutover rehearsal (MIGRATE-04 / D-04).
#
# Auth: CLOUDFLARE_API_TOKEN or CF_API_TOKEN with Zone.DNS Edit (and Zone.Zone Read).
# Wrangler OAuth is zone:read only and MUST NOT be used for DNS writes.
#
# Usage:
#   TARGET=<hostname|IPv4> ./scripts/cutover-dns-cloudflare.sh [--dry-run] [--proxied]
#   ./scripts/cutover-dns-cloudflare.sh --cleanup [--dry-run]
#
# Env:
#   MAGELIFT_CUTOVER_HOST  default magelift-preview.alexandrecourtiol.com
#   CURL_BIN               curl binary override (acceptance mocks)
#   CF_API_BASE            API base (default https://api.cloudflare.com/client/v4)
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

Requires CLOUDFLARE_API_TOKEN or CF_API_TOKEN (Zone.DNS Edit) except for --dry-run.
Wrangler OAuth cannot write DNS (zone:read only).
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
if [[ -z "$token" && "$DRY_RUN" -eq 0 ]]; then
	printf 'cutover-dns-cloudflare: set CLOUDFLARE_API_TOKEN or CF_API_TOKEN (Zone.DNS Edit). Wrangler OAuth is insufficient.\n' >&2
	exit 1
fi

if [[ "$CLEANUP" -eq 0 && -z "$TARGET" ]]; then
	printf 'cutover-dns-cloudflare: TARGET is required unless --cleanup\n' >&2
	usage
	exit 2
fi

# Prefer the zone that matches the host suffix; alexandrecourtiol.com first, then acourtiol.com.
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

record_type_for_target() {
	local t="$1"
	if [[ "$t" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
		printf 'A'
	else
		printf 'CNAME'
	fi
}

# Cloudflare CNAME content should not end with a trailing dot in API payloads.
normalize_cname_content() {
	local t="$1"
	t="${t%.}"
	printf '%s' "$t"
}

cf_curl() {
	# Args: METHOD PATH [json-body]
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
		# Emit a minimal successful JSON shape so callers can parse in dry-run.
		# Only the zone *collection* endpoint — never /zones/{id}/...
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
	# Cheap success check without jq dependency.
	printf '%s' "$1" | grep -q '"success"[[:space:]]*:[[:space:]]*true'
}

extract_first_id() {
	# Prefer result[0].id (list) then result.id (object).
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
	printf 'cutover-dns-cloudflare: could not resolve zone id for host %s (tried alexandrecourtiol.com, acourtiol.com)\n' "$HOST" >&2
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

upsert_record() {
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

cleanup_record() {
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

if [[ "$CLEANUP" -eq 1 ]]; then
	cleanup_record
else
	upsert_record
fi

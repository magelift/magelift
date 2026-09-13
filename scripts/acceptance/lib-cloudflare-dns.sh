#!/usr/bin/env bash
# Cloudflare DNS lifecycle for disposable acceptance records.
# shellcheck shell=bash

# This helper deliberately has no zone-wide cleanup path. Every mutation is
# scoped to one subdomain, one CNAME target, and one exact ownership marker.

CLOUDFLARE_ACCEPTANCE_ZONE_DEFAULT="acourtiol.com"
CLOUDFLARE_ACCEPTANCE_TTL_DEFAULT_SECONDS=60
CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_DEFAULT_SECONDS=180
CLOUDFLARE_ACCEPTANCE_DNS_POLL_DEFAULT_SECONDS=5

cloudflare_acceptance_dns_init() {
	CLOUDFLARE_ACCEPTANCE_ZONE="${MAGELIFT_CLOUDFLARE_ZONE:-$CLOUDFLARE_ACCEPTANCE_ZONE_DEFAULT}"
	CLOUDFLARE_ACCEPTANCE_PROFILE="${MAGELIFT_CLOUDFLARE_PROFILE:-default}"
	CLOUDFLARE_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_CLOUDFLARE_DNS_TTL_SECONDS:-$CLOUDFLARE_ACCEPTANCE_TTL_DEFAULT_SECONDS}"
	CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS="${MAGELIFT_CLOUDFLARE_DNS_TIMEOUT_SECONDS:-$CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_DEFAULT_SECONDS}"
	CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS="${MAGELIFT_CLOUDFLARE_DNS_POLL_SECONDS:-$CLOUDFLARE_ACCEPTANCE_DNS_POLL_DEFAULT_SECONDS}"
	if [[ ! "$CLOUDFLARE_ACCEPTANCE_ZONE" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]]; then
		printf 'invalid Cloudflare acceptance zone %q\n' "$CLOUDFLARE_ACCEPTANCE_ZONE" >&2
		return 1
	fi
	if [[ ! "$CLOUDFLARE_ACCEPTANCE_TTL_SECONDS" =~ ^[1-9][0-9]*$ ]] || (( CLOUDFLARE_ACCEPTANCE_TTL_SECONDS > 300 )); then
		printf 'Cloudflare acceptance DNS TTL must be a positive integer no greater than 300 seconds\n' >&2
		return 1
	fi
	if [[ ! "$CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]] || [[ ! "$CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
		printf 'Cloudflare acceptance DNS timeout and poll intervals must be positive integers\n' >&2
		return 1
	fi
	if (( CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS > CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS )); then
		printf 'Cloudflare acceptance DNS poll interval cannot exceed the timeout\n' >&2
		return 1
	fi
	if ! command -v cf >/dev/null 2>&1; then
		printf 'Cloudflare acceptance DNS requires the cf CLI\n' >&2
		return 1
	fi
	if ! cf --profile "$CLOUDFLARE_ACCEPTANCE_PROFILE" auth whoami >/dev/null 2>&1; then
		printf 'Cloudflare acceptance DNS requires an authenticated cf profile %q\n' "$CLOUDFLARE_ACCEPTANCE_PROFILE" >&2
		return 1
	fi
}

cloudflare_acceptance_dns_validate_domain() {
	local domain="${1:?domain required}"
	if [[ "$domain" == "$CLOUDFLARE_ACCEPTANCE_ZONE" || "$domain" != *".$CLOUDFLARE_ACCEPTANCE_ZONE" ]]; then
		printf 'Cloudflare acceptance domain must be a subdomain of %s: %q\n' "$CLOUDFLARE_ACCEPTANCE_ZONE" "$domain" >&2
		return 1
	fi
	if [[ ! "$domain" =~ ^[a-z0-9_.-]+$ || "$domain" == .* || "$domain" == *..* || "$domain" == *. || "$domain" == *.-* || "$domain" == *-.* ]]; then
		printf 'invalid Cloudflare acceptance domain %q\n' "$domain" >&2
		return 1
	fi
	local label
	while IFS= read -r label; do
		if [[ "$label" == "_acme-challenge" ]]; then
			continue
		fi
		if [[ ! "$label" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]]; then
			printf 'invalid Cloudflare acceptance domain label %q\n' "$label" >&2
			return 1
		fi
	done < <(tr '.' '\n' <<<"$domain")
}

cloudflare_acceptance_dns_validate_marker() {
	local marker="${1:?ownership marker required}"
	if [[ ! "$marker" =~ ^magelift/acceptance/[a-zA-Z0-9._/-]+$ || "$marker" == *..* || "$marker" == */ || "$marker" == *//** ]]; then
		printf 'invalid Cloudflare acceptance ownership marker %q\n' "$marker" >&2
		return 1
	fi
}

cloudflare_acceptance_dns_validate_target() {
	local target="${1:?CNAME target required}"
	target="${target%.}"
	if [[ ! "$target" =~ ^[a-zA-Z0-9.-]+$ || "$target" == .* || "$target" == *..* || "$target" == *. || "$target" == *.-* || "$target" == *-.* ]]; then
		printf 'invalid Cloudflare acceptance CNAME target %q\n' "$target" >&2
		return 1
	fi
}

cloudflare_acceptance_dns_validate_ipv4() {
	local address="${1:?IPv4 address required}" octet
	local -a octets
	if [[ ! "$address" =~ ^[0-9]+(\.[0-9]+){3}$ ]]; then
		printf 'invalid Cloudflare acceptance IPv4 address %q\n' "$address" >&2
		return 1
	fi
	IFS=. read -r -a octets <<<"$address"
	for octet in "${octets[@]}"; do
		if (( 10#$octet > 255 )); then
			printf 'invalid Cloudflare acceptance IPv4 address %q\n' "$address" >&2
			return 1
		fi
	done
}

cloudflare_acceptance_dns_cli() {
	cf --profile "$CLOUDFLARE_ACCEPTANCE_PROFILE" --zone "$CLOUDFLARE_ACCEPTANCE_ZONE" "$@"
}

cloudflare_acceptance_dns_records_json() {
	local domain="${1:?domain required}"
	cloudflare_acceptance_dns_cli dns records list --name-exact "$domain" --per-page 100 --quiet
}

cloudflare_acceptance_dns_owned_records_json() {
	local domain="${1:?domain required}" marker="${2:?ownership marker required}"
	cloudflare_acceptance_dns_records_json "$domain" | jq -c --arg domain "$domain" --arg marker "$marker" '
		(if type == "array" then . else (.result // []) end)
		| map(select((.name // "" | ascii_downcase) == ($domain | ascii_downcase)
			and (.type // "") == "CNAME"
			and (.comment // "") == $marker))'
}

cloudflare_acceptance_dns_record_ids() {
	local domain="${1:?domain required}" marker="${2:?ownership marker required}"
	cloudflare_acceptance_dns_owned_records_json "$domain" "$marker" | jq -r '.[].id'
}

cloudflare_acceptance_dns_prepare() {
	local domain="${1:?domain required}" target="${2:?CNAME target required}" marker="${3:?ownership marker required}"
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_target "$target"
	cloudflare_acceptance_dns_validate_marker "$marker"
	target="${target%.}"

	local records owned_count owned_target record body
	records="$(cloudflare_acceptance_dns_records_json "$domain")"
	owned_count="$(printf '%s' "$records" | jq --arg domain "$domain" --arg marker "$marker" '[if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "CNAME" and (.comment // "") == $marker)] | length')"
	if [[ "$owned_count" -gt 1 ]]; then
		printf 'refusing Cloudflare DNS mutation: multiple records carry marker %q at %q\n' "$marker" "$domain" >&2
		return 1
	fi
	if [[ "$owned_count" -eq 1 ]]; then
		owned_target="$(printf '%s' "$records" | jq -r --arg domain "$domain" --arg marker "$marker" 'if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "CNAME" and (.comment // "") == $marker) | .content' | sed 's/\.$//')"
		if [[ "$owned_target" != "$target" ]]; then
			printf 'refusing Cloudflare DNS mutation: owned record target drift at %q (found %q, want %q)\n' "$domain" "$owned_target" "$target" >&2
			return 1
		fi
		CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID="$(printf '%s' "$records" | jq -r --arg domain "$domain" --arg marker "$marker" 'if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "CNAME" and (.comment // "") == $marker) | .id')"
		CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED=0
		export CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED
		return 0
	fi

	if printf '%s' "$records" | jq -e --arg domain "$domain" '[if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase))] | length > 0' >/dev/null; then
		printf 'refusing Cloudflare DNS mutation: unowned record already exists at %q\n' "$domain" >&2
		return 1
	fi
	body="$(jq -cn --arg name "$domain" --arg content "$target" --arg comment "$marker" --argjson ttl "$CLOUDFLARE_ACCEPTANCE_TTL_SECONDS" '{type:"CNAME",name:$name,content:$content,ttl:$ttl,proxied:false,comment:$comment}')"
	record="$(cloudflare_acceptance_dns_cli dns records create --body "$body" --quiet)"
	CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID="$(printf '%s' "$record" | jq -r 'if type == "array" then .[0].id // empty else (.id // .result.id // empty) end')"
	if [[ -z "$CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID" ]]; then
		printf 'Cloudflare DNS create returned no record ID; refusing to claim ownership\n' >&2
		return 1
	fi
	CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED=1
	export CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED
}

cloudflare_acceptance_dns_wait_for_cname() {
	local domain="${1:?domain required}" target="${2:?CNAME target required}" expected now deadline observed
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_target "$target"
	target="${target%.}"
	expected="${target}."
	deadline=$(( $(date +%s) + CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS ))
	while :; do
		observed="$(dig +short CNAME "$domain" 2>/dev/null | sed -n '1p')"
		if [[ "${observed%.}." == "$expected" ]]; then
			return 0
		fi
		now="$(date +%s)"
		if (( now >= deadline )); then
			printf 'Cloudflare DNS did not converge for %q: observed %q, expected %q\n' "$domain" "$observed" "$expected" >&2
			return 1
		fi
		sleep "$CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS"
	done
}

cloudflare_acceptance_dns_cleanup() {
	local domain="${1:?domain required}" marker="${2:?ownership marker required}" target="${3:-}"
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_marker "$marker"
	if [[ -n "$target" ]]; then
		cloudflare_acceptance_dns_validate_target "$target"
		target="${target%.}"
	fi

	local records ids id observed
	records="$(cloudflare_acceptance_dns_owned_records_json "$domain" "$marker")"
	if [[ -n "$target" ]]; then
		observed="$(printf '%s' "$records" | jq -r '.[].content' | sed 's/\.$//' | sort -u | paste -sd, -)"
		if [[ -n "$observed" && "$observed" != "$target" ]]; then
			printf 'refusing Cloudflare DNS cleanup: marker %q has unexpected target(s) %q\n' "$marker" "$observed" >&2
			return 1
		fi
	fi
	ids="$(printf '%s' "$records" | jq -r '.[].id')"
	while IFS= read -r id; do
		[[ -z "$id" ]] && continue
		cloudflare_acceptance_dns_cli dns records delete "$id" --force --quiet >/dev/null
	done <<<"$ids"
	if [[ -n "$(cloudflare_acceptance_dns_record_ids "$domain" "$marker")" ]]; then
		printf 'Cloudflare DNS cleanup did not confirm deletion for %q marker=%q\n' "$domain" "$marker" >&2
		return 1
	fi
}

cloudflare_acceptance_dns_owned_a_records_json() {
	local domain="${1:?domain required}" marker="${2:?ownership marker required}"
	cloudflare_acceptance_dns_records_json "$domain" | jq -c --arg domain "$domain" --arg marker "$marker" '
		(if type == "array" then . else (.result // []) end)
		| map(select((.name // "" | ascii_downcase) == ($domain | ascii_downcase)
			and (.type // "") == "A"
			and (.comment // "") == $marker))'
}

cloudflare_acceptance_dns_prepare_a() {
	local domain="${1:?domain required}" address="${2:?IPv4 address required}" marker="${3:?ownership marker required}"
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_ipv4 "$address"
	cloudflare_acceptance_dns_validate_marker "$marker"

	local records owned_count owned_address record body record_id
	records="$(cloudflare_acceptance_dns_records_json "$domain")"
	owned_count="$(printf '%s' "$records" | jq --arg domain "$domain" --arg marker "$marker" '[if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "A" and (.comment // "") == $marker)] | length')"
	if [[ "$owned_count" -gt 1 ]]; then
		printf 'refusing Cloudflare DNS mutation: multiple A records carry marker %q at %q\n' "$marker" "$domain" >&2
		return 1
	fi
	if [[ "$owned_count" -eq 1 ]]; then
		owned_address="$(printf '%s' "$records" | jq -r --arg domain "$domain" --arg marker "$marker" 'if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "A" and (.comment // "") == $marker) | .content')"
		if [[ "$owned_address" != "$address" ]]; then
			printf 'refusing Cloudflare DNS mutation: owned A record drift at %q (found %q, want %q)\n' "$domain" "$owned_address" "$address" >&2
			return 1
		fi
		CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID="$(printf '%s' "$records" | jq -r --arg domain "$domain" --arg marker "$marker" 'if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase) and (.type // "") == "A" and (.comment // "") == $marker) | .id')"
		CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED=0
		export CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED
		return 0
	fi

	if printf '%s' "$records" | jq -e --arg domain "$domain" '[if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($domain | ascii_downcase))] | length > 0' >/dev/null; then
		printf 'refusing Cloudflare DNS mutation: unowned record already exists at %q\n' "$domain" >&2
		return 1
	fi
	body="$(jq -cn --arg name "$domain" --arg content "$address" --arg comment "$marker" --argjson ttl "$CLOUDFLARE_ACCEPTANCE_TTL_SECONDS" '{type:"A",name:$name,content:$content,ttl:$ttl,proxied:false,comment:$comment}')"
	record="$(cloudflare_acceptance_dns_cli dns records create --body "$body" --quiet)"
	record_id="$(printf '%s' "$record" | jq -r 'if type == "array" then .[0].id // empty else (.id // .result.id // empty) end')"
	if [[ -z "$record_id" ]]; then
		printf 'Cloudflare DNS A record create returned no record ID; refusing to claim ownership at %q\n' "$domain" >&2
		return 1
	fi
	CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID="$record_id"
	CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED=1
	export CLOUDFLARE_ACCEPTANCE_DNS_RECORD_ID CLOUDFLARE_ACCEPTANCE_DNS_RECORD_CREATED
}

cloudflare_acceptance_dns_wait_for_a() {
	local domain="${1:?domain required}" address="${2:?IPv4 address required}" observed now deadline
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_ipv4 "$address"
	deadline=$(( $(date +%s) + CLOUDFLARE_ACCEPTANCE_DNS_TIMEOUT_SECONDS ))
	while :; do
		observed="$(dig +short A "$domain" 2>/dev/null | sed -n '1p')"
		if [[ "$observed" == "$address" ]]; then
			return 0
		fi
		now="$(date +%s)"
		if (( now >= deadline )); then
			printf 'Cloudflare DNS did not converge for %q: observed %q, expected %q\n' "$domain" "$observed" "$address" >&2
			return 1
		fi
		sleep "$CLOUDFLARE_ACCEPTANCE_DNS_POLL_SECONDS"
	done
}

cloudflare_acceptance_dns_cleanup_a() {
	local domain="${1:?domain required}" marker="${2:?ownership marker required}" address="${3:-}"
	cloudflare_acceptance_dns_validate_domain "$domain"
	cloudflare_acceptance_dns_validate_marker "$marker"
	if [[ -n "$address" ]]; then
		cloudflare_acceptance_dns_validate_ipv4 "$address"
	fi

	local records ids id observed
	records="$(cloudflare_acceptance_dns_owned_a_records_json "$domain" "$marker")"
	if [[ -n "$address" ]]; then
		observed="$(printf '%s' "$records" | jq -r '.[].content' | sort -u | paste -sd, -)"
		if [[ -n "$observed" && "$observed" != "$address" ]]; then
			printf 'refusing Cloudflare DNS cleanup: marker %q has unexpected address(es) %q\n' "$marker" "$observed" >&2
			return 1
		fi
	fi
	ids="$(printf '%s' "$records" | jq -r '.[].id')"
	while IFS= read -r id; do
		[[ -z "$id" ]] && continue
		cloudflare_acceptance_dns_cli dns records delete "$id" --force --quiet >/dev/null
	done <<<"$ids"
	if [[ -n "$(cloudflare_acceptance_dns_owned_a_records_json "$domain" "$marker" | jq -r '.[].id')" ]]; then
		printf 'Cloudflare DNS A cleanup did not confirm deletion for %q marker=%q\n' "$domain" "$marker" >&2
		return 1
	fi
}

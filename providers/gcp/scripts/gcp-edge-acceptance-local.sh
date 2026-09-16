#!/usr/bin/env bash
# Disposable GCP native edge acceptance. The wrapper owns the Internet FQDN NEG,
# CDN-enabled backend service, optional Magento-safe Cloud Armor policy, Google-managed
# certificate, Cloudflare DNS, cleanup ledger, and TTL watchdog; the Go cell owns
# plan/apply/verify/optional origin-group failover/destroy and ownership inventory
# verification for URL map, target HTTPS proxy, and forwarding rule. Set
# MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC=1 for viewer traffic; add
# MAGELIFT_GCP_EDGE_ORIGIN_TITLE to compare a known origin title through the
# alias; set MAGELIFT_GCP_EDGE_ARMOR_TRAFFIC=1 with WAF=1 to exercise an owned
# temporary Cloud Armor deny rule and require HTTP 403; set
# MAGELIFT_GCP_EDGE_FAILOVER=1 with ORIGIN_GROUP=1 and ALIAS_TRAFFIC=1 to prove
# health-gated URL-map failover through distinct origin HTML titles.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-cloudflare-dns.sh
source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"
# shellcheck source=acceptance/lib-cleanup-ledger.sh
source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands gcloud go cf dig curl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

run_id="${MAGELIFT_GCP_EDGE_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
domain="${MAGELIFT_GCP_EDGE_DOMAIN:-ml-gcp-edge-${run_id}.acourtiol.com}"
marker="${MAGELIFT_GCP_EDGE_MARKER:-magelift/gcp/edge/${run_id}}"
origin_host="${MAGELIFT_GCP_EDGE_ORIGIN_HOST:-example.com}"
secondary_origin="${MAGELIFT_GCP_EDGE_SECONDARY_ORIGIN:-developers.google.com}"
origin_title="${MAGELIFT_GCP_EDGE_ORIGIN_TITLE:-}"
alias_traffic="${MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC:-0}"
origin_group="${MAGELIFT_GCP_EDGE_ORIGIN_GROUP:-0}"
failover="${MAGELIFT_GCP_EDGE_FAILOVER:-0}"
waf_enabled="${MAGELIFT_GCP_EDGE_WAF:-0}"
armor_traffic="${MAGELIFT_GCP_EDGE_ARMOR_TRAFFIC:-0}"
dns_marker="magelift/acceptance/gcp-edge-dns/${run_id}"
resource_suffix="magelift-edge-${run_id}"
neg_name="${resource_suffix}-neg"
secondary_neg_name="${resource_suffix}-neg-sec"
backend_name="${resource_suffix}-bs"
secondary_backend_name="${resource_suffix}-bs-sec"
cert_name="${resource_suffix}-cert"
armor_name="${resource_suffix}-armor"
# The native-origin policy already owns twelve CEVAL rules, and this outer
# policy consumes the project's remaining global rule budget. Reuse the
# policy's existing static/media allow rule for the short data-plane probe
# instead of creating a twenty-first rule when the project quota is exactly
# 20. Cloud Armor does not permit changing the required default rule's match
# expression from `*`, so the probe must use a non-default rule and restore its
# exact original definition before cleanup.
armor_test_rule_priority=100
armor_test_rule_original_action='allow'
armor_test_rule_original_description='allow Magento static and media'
armor_test_rule_original_expression="request.path.startsWith('/static/') || request.path.startsWith('/media/') || request.path.startsWith('/pub/static/') || request.path.startsWith('/pub/media/')"
armor_test_header="x-magelift-armor-test"
armor_test_value="block-${run_id}"

if [[ "$alias_traffic" == 1 || "$failover" == 1 ]]; then
	# example.com is Cloudflare-fronted and 403s Google Front End IPs.
	# www.wikipedia.org 403s Go's default User-Agent during the origin health probe.
	if [[ "$origin_host" == "example.com" || "$origin_host" == "www.wikipedia.org" ]]; then
		origin_host="www.google.com"
	fi
	if [[ "$secondary_origin" == "example.com" || "$secondary_origin" == "www.wikipedia.org" ]]; then
		secondary_origin="developers.google.com"
	fi
fi

if [[ ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$domain" =~ ^[a-z0-9][a-z0-9.-]*[a-z0-9]$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,110}$ || ! "$origin_host" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$ || ! "$secondary_origin" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$ || "$alias_traffic" != 0 && "$alias_traffic" != 1 || "$origin_group" != 0 && "$origin_group" != 1 || "$failover" != 0 && "$failover" != 1 || "$waf_enabled" != 0 && "$waf_enabled" != 1 || "$armor_traffic" != 0 && "$armor_traffic" != 1 || "$origin_title" == *$'\r'* || "$origin_title" == *$'\n'* ]]; then
	printf 'set valid GCP edge run ID, domain, marker, origin hostnames, origin title, alias flag, origin-group flag, failover flag, WAF flag, and Armor traffic flag\n' >&2
	exit 2
fi
if [[ "$origin_title" != "" && "$alias_traffic" != 1 ]]; then
	printf 'GCP edge origin title verification requires MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC=1\n' >&2
	exit 2
fi
if [[ "$armor_traffic" == 1 && ( "$waf_enabled" != 1 || "$alias_traffic" != 1 ) ]]; then
	printf 'GCP edge Armor traffic verification requires both WAF=1 and ALIAS_TRAFFIC=1\n' >&2
	exit 2
fi
if [[ "$failover" == 1 && ( "$origin_group" != 1 || "$alias_traffic" != 1 ) ]]; then
	printf 'GCP edge failover requires MAGELIFT_GCP_EDGE_ORIGIN_GROUP=1, MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC=1, and distinct origin hostnames\n' >&2
	exit 2
fi
if [[ "$origin_group" == 1 && "$origin_host" == "$secondary_origin" ]]; then
	printf 'GCP edge origin-group requires distinct primary and secondary origin hostnames\n' >&2
	exit 2
fi

gcp_edge_resolve_project() {
	if [[ -n "${MAGELIFT_GCP_PROJECT:-}" ]]; then
		printf '%s' "$MAGELIFT_GCP_PROJECT"
		return 0
	fi
	local configured
	configured="$(gcloud config get-value project 2>/dev/null || true)"
	if [[ -z "$configured" || "$configured" == "(unset)" ]]; then
		printf 'set MAGELIFT_GCP_PROJECT or gcloud config project\n' >&2
		return 1
	fi
	printf '%s' "$configured"
}

gcp_edge_owned_description_count() {
	local project="${1:?project required}" kind="${2:?kind required}" extra_global="${3:-}"
	local count=0 description args=(compute "$kind" list --project="$project")
	if [[ "$extra_global" == global ]]; then
		args+=(--global)
	fi
	while IFS= read -r description; do
		[[ -z "$description" ]] && continue
		if [[ "$description" == magelift\ ownership=* ]]; then
			count=$((count + 1))
		fi
	done < <(gcloud "${args[@]}" --format='value(description)' 2>/dev/null || true)
	printf '%s' "$count"
}

gcp_edge_owned_edge_resource_count() {
	local project="${1:?project required}"
	local total=0 count
	count="$(gcp_edge_owned_description_count "$project" forwarding-rules global)"
	if [[ ! "$count" =~ ^[0-9]+$ ]]; then
		printf 'unable to query GCP forwarding-rules ownership inventory\n' >&2
		return 1
	fi
	total=$((total + count))
	for kind in url-maps target-https-proxies backend-buckets security-policies; do
		count="$(gcp_edge_owned_description_count "$project" "$kind")"
		if [[ ! "$count" =~ ^[0-9]+$ ]]; then
			printf 'unable to query GCP %s ownership inventory\n' "$kind" >&2
			return 1
		fi
		total=$((total + count))
	done
	printf '%s' "$total"
}

gcp_edge_owned_comment() {
	local marker="${1:?marker required}"
	printf 'magelift ownership=%s' "$marker"
}

gcp_edge_verify_magento_armor() {
	local project="${1:?project required}" name="${2:?security policy name required}"
	local policy_file
	policy_file="${TMPDIR:-/tmp}/magelift-gcp-edge-armor-verify-$$.json"
	python3 "$ROOT/scripts/acceptance/gcp-armor-beta-body-exclusions.py" export --project "$project" --policy "$name" --output "$policy_file"
	if ! jq -e '
		(if type == "array" then .[0] else . end) as $p
		| ($p.advancedOptionsConfig.jsonParsing == "STANDARD_WITH_GRAPHQL")
		and (($p.advancedOptionsConfig.requestBodyInspectionSize // "" | ascii_upcase) == "64KB")
		and ([$p.rules[]? | select(.match.expr.expression != null and (.match.expr.expression | contains("sensitivity")))] | length > 0)
		and ([$p.rules[]? | select(.description == "scannerdetection" and .preview != true)] | length > 0)
		and ([$p.rules[]? | .preconfiguredWafConfig.exclusions[]? | select(.targetRuleSet == "sqli-v33-stable" and ((.requestBodiesToExclude // []) | length) > 0)] | length > 0)
	' "$policy_file" >/dev/null; then
		printf 'Cloud Armor policy is not Magento-safe protect mode name=%s jsonType=%s jsonParsing=%s body=%s scanner=%s sqliBodyExcl=%s\n' \
			"$name" \
			"$(jq -r 'type' "$policy_file" 2>/dev/null || printf unknown)" \
			"$(jq -r '.advancedOptionsConfig.jsonParsing // empty' "$policy_file" 2>/dev/null || true)" \
			"$(jq -r '.advancedOptionsConfig.requestBodyInspectionSize // empty' "$policy_file" 2>/dev/null || true)" \
			"$(jq -r '[.rules[]? | select(.description == "scannerdetection")] | length' "$policy_file" 2>/dev/null || true)" \
			"$(jq -r '[.rules[]? | .preconfiguredWafConfig.exclusions[]? | select(.targetRuleSet == "sqli-v33-stable") | (.requestBodiesToExclude // [])[]] | length' "$policy_file" 2>/dev/null || true)" >&2
		rm -f "$policy_file"
		return 1
	fi
	rm -f "$policy_file"
}

gcp_edge_delete_armor() {
	local project="${1:?project required}" name="${2:?security policy name required}"
	if ! gcloud compute security-policies delete "$name" --project="$project" --quiet >/dev/null 2>&1; then
		if gcloud compute security-policies describe "$name" --project="$project" --format='value(name)' 2>/dev/null | grep -qv '^$'; then
			printf 'Cloud Armor security policy cleanup failed name=%s\n' "$name" >&2
			return 1
		fi
	fi
	return 0
}

gcp_edge_detach_armor() {
	local project="${1:?project required}" backend="${2:?backend service required}"
	gcloud compute backend-services update "$backend" --project="$project" --global --security-policy="" --quiet >/dev/null 2>&1 || true
}

gcp_edge_cloudflare_create_a_record() {
	cloudflare_acceptance_dns_prepare_a "$@"
}

gcp_edge_cloudflare_cleanup_a_record() {
	cloudflare_acceptance_dns_cleanup_a "$@"
}

gcp_edge_wait_managed_cert_active() {
	local project="${1:?project required}" cert_name="${2:?certificate required}" deadline wait_seconds status details failed_domain last_failed_domain=""
	wait_seconds="${MAGELIFT_GCP_EDGE_CERT_WAIT_SECONDS:-3600}"
	if [[ ! "$wait_seconds" =~ ^[0-9]+$ ]] || (( wait_seconds < 60 )); then
		printf 'MAGELIFT_GCP_EDGE_CERT_WAIT_SECONDS must be an integer number of seconds >= 60\n' >&2
		return 2
	fi
	deadline=$(( $(date +%s) + wait_seconds ))
	while :; do
		details="$(gcloud compute ssl-certificates describe "$cert_name" --project="$project" --global --format=json 2>/dev/null || true)"
		status="$(jq -r '.managed.status // empty' <<<"$details" 2>/dev/null || true)"
		failed_domain="$(jq -r '[.managed.domainStatus // {} | to_entries[] | select(.value != "PROVISIONING" and .value != "ACTIVE") | "\(.key)=\(.value)"] | join(",")' <<<"$details" 2>/dev/null || true)"
		case "$status" in
			ACTIVE)
				return 0
				;;
			PROVISIONING)
				if [[ -n "$failed_domain" ]]; then
					if [[ "$failed_domain" != *"=FAILED_NOT_VISIBLE"* ]]; then
						printf 'managed SSL certificate has terminal domain status: %s for %s\n' "$failed_domain" "$cert_name" >&2
						return 1
					fi
					if [[ "$failed_domain" != "$last_failed_domain" ]]; then
						last_failed_domain="$failed_domain"
						printf 'managed SSL certificate domain status is not ready: %s; continuing until the %ss wait budget expires\n' "$failed_domain" "$wait_seconds" >&2
					fi
				else
					last_failed_domain=""
				fi
				;;
			PROVISIONING_FAILED|PROVISIONING_FAILED_PERMANENTLY|RENEWAL_FAILED)
				printf 'managed SSL certificate entered terminal state %s (domainStatus=%s) for %s\n' "$status" "${failed_domain:-none}" "$cert_name" >&2
				return 1
				;;
			*)
				if [[ -n "$status" ]]; then
					printf 'managed SSL certificate entered terminal state %s (domainStatus=%s) for %s\n' "$status" "${failed_domain:-none}" "$cert_name" >&2
					return 1
				fi
				;;
		esac
		if (( $(date +%s) >= deadline )); then
			printf 'managed SSL certificate did not reach ACTIVE within %s seconds: %s\n' "$wait_seconds" "$cert_name" >&2
			return 1
		fi
		sleep 15
	done
}

gcp_edge_wait_alias_https() {
	local hostname="${1:?hostname required}" address="${2:?address required}" deadline wait_seconds status
	wait_seconds="${MAGELIFT_GCP_EDGE_ALIAS_WAIT_SECONDS:-1800}"
	if [[ ! "$wait_seconds" =~ ^[0-9]+$ ]] || (( wait_seconds < 60 )); then
		printf 'MAGELIFT_GCP_EDGE_ALIAS_WAIT_SECONDS must be an integer number of seconds >= 60\n' >&2
		return 2
	fi
	deadline=$(( $(date +%s) + wait_seconds ))
	printf '+ gcp-edge: waiting up to %ss for alias HTTPS after managed cert ACTIVE host=%s address=%s\n' "$wait_seconds" "$hostname" "$address" >&2
	while :; do
		status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 --resolve "${hostname}:443:${address}" "https://${hostname}/" || true)"
		if [[ "$status" =~ ^[23][0-9][0-9]$ ]]; then
			printf '%s' "$status"
			return 0
		fi
		if (( $(date +%s) >= deadline )); then
			printf 'alias HTTPS did not succeed for %s at %s within %s seconds (last HTTP %s)\n' "$hostname" "$address" "$wait_seconds" "${status:-none}" >&2
			return 1
		fi
		sleep 15
	done
}

gcp_edge_origin_html_title() {
	local hostname="${1:?hostname required}"
	# macOS tr in a UTF-8 locale exits on illegal bytes in origin HTML.
	LC_ALL=C curl -sS -A 'MageLift-origin-health/1' --max-time 20 "https://${hostname}/" |
		LC_ALL=C tr '\n' ' ' |
		grep -oE '<title[^>]*>[^<]+' |
		head -1 |
		sed -E 's/<title[^>]*>//;s/^[[:space:]]+//;s/[[:space:]]+$//'
}

gcp_edge_wait_alias_html_title() {
	local hostname="${1:?hostname required}" address="${2:?address required}" want="${3:?title required}" deadline wait_seconds got
	wait_seconds="${MAGELIFT_GCP_EDGE_ALIAS_WAIT_SECONDS:-1800}"
	if [[ ! "$wait_seconds" =~ ^[0-9]+$ ]] || (( wait_seconds < 60 )); then
		printf 'MAGELIFT_GCP_EDGE_ALIAS_WAIT_SECONDS must be an integer number of seconds >= 60\n' >&2
		return 2
	fi
	deadline=$(( $(date +%s) + wait_seconds ))
	while :; do
		got="$(LC_ALL=C curl -sS -A 'MageLift-origin-health/1' --max-time 20 --resolve "${hostname}:443:${address}" "https://${hostname}/" | LC_ALL=C tr '\n' ' ' | grep -oE '<title[^>]*>[^<]+' | head -1 | sed -E 's/<title[^>]*>//;s/^[[:space:]]+//;s/[[:space:]]+$//' || true)"
		if [[ "$got" == "$want" ]]; then
			printf '%s' "$got"
			return 0
		fi
		if (( $(date +%s) >= deadline )); then
			printf 'GCP edge alias HTML title did not become %q for %s (last %q)\n' "$want" "$hostname" "${got:-none}" >&2
			return 1
		fi
		sleep 15
	done
}

gcp_edge_invalidate_url_map() {
	local project="${1:?project required}" name="${2:?url map name required}"
	printf '+ gcp-edge: invalidating Cloud CDN cache urlMap=%s path=/*\n' "$name" >&2
	gcloud compute url-maps invalidate-cdn-cache "$name" \
		--project="$project" \
		--path='/*' \
		--quiet >/dev/null
}

gcp_edge_enable_armor_test_rule() {
	local project="${1:?project required}" name="${2:?security policy name required}" description expression existing_action existing_description existing_expression
	description="$(gcp_edge_owned_comment "$marker") data-plane-test"
	expression="has(request.headers['${armor_test_header}']) && request.headers['${armor_test_header}'] == '${armor_test_value}'"
	existing_action="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(action)' 2>/dev/null || true)"
	existing_description="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(description)' 2>/dev/null || true)"
	existing_expression="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(match.expr.expression)' 2>/dev/null || true)"
	if [[ "$existing_action" != "$armor_test_rule_original_action" || "$existing_description" != "$armor_test_rule_original_description" || "$existing_expression" != "$armor_test_rule_original_expression" ]]; then
		printf 'refusing to repurpose drifted Cloud Armor rule priority=%s policy=%s action=%s description=%s expression=%s\n' \
			"$armor_test_rule_priority" "$name" "${existing_action:-none}" "${existing_description:-none}" "${existing_expression:-none}" >&2
		return 1
	fi
	gcloud compute security-policies rules update "$armor_test_rule_priority" \
		--project="$project" \
		--security-policy="$name" \
		--expression="$expression" \
		--action=deny-403 \
		--description="$description" \
		--quiet >/dev/null
}

gcp_edge_restore_armor_test_rule() {
	local project="${1:?project required}" name="${2:?security policy name required}" description expression action
	action="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(action)' 2>/dev/null || true)"
	description="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(description)' 2>/dev/null || true)"
	expression="$(gcloud compute security-policies rules describe "$armor_test_rule_priority" --project="$project" --security-policy="$name" --format='value(match.expr.expression)' 2>/dev/null || true)"
	if [[ -z "$description" || -z "$expression" ]]; then
		printf 'Cloud Armor test rule disappeared priority=%s policy=%s\n' "$armor_test_rule_priority" "$name" >&2
		return 1
	fi
	if [[ "$action" != deny\(403\) || "$description" != "$(gcp_edge_owned_comment "$marker") data-plane-test" || "$expression" != "has(request.headers['${armor_test_header}']) && request.headers['${armor_test_header}'] == '${armor_test_value}'" ]]; then
		printf 'refusing to restore unowned Cloud Armor test rule priority=%s policy=%s\n' "$armor_test_rule_priority" "$name" >&2
		return 1
	fi
	if ! gcloud compute security-policies rules update "$armor_test_rule_priority" \
		--project="$project" \
		--security-policy="$name" \
		--expression="$armor_test_rule_original_expression" \
		--action="$armor_test_rule_original_action" \
		--description="$armor_test_rule_original_description" \
		--quiet >/dev/null; then
		printf 'Cloud Armor test rule restore failed priority=%s policy=%s\n' "$armor_test_rule_priority" "$name" >&2
		return 1
	fi
}

gcp_edge_wait_armor_block() {
	local hostname="${1:?hostname required}" address="${2:?address required}" deadline wait_seconds status
	wait_seconds="${MAGELIFT_GCP_EDGE_ARMOR_WAIT_SECONDS:-300}"
	if [[ ! "$wait_seconds" =~ ^[0-9]+$ ]] || (( wait_seconds < 60 )); then
		printf 'MAGELIFT_GCP_EDGE_ARMOR_WAIT_SECONDS must be an integer number of seconds >= 60\n' >&2
		return 2
	fi
	deadline=$(( $(date +%s) + wait_seconds ))
	while :; do
		status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 \
			-H "${armor_test_header}: ${armor_test_value}" \
			--resolve "${hostname}:443:${address}" "https://${hostname}/?magelift_armor_probe=${run_id}" || true)"
		if [[ "$status" == 403 ]]; then
			printf '%s' "$status"
			return 0
		fi
		if (( $(date +%s) >= deadline )); then
			printf 'Cloud Armor data-plane test did not return 403 for %s (last HTTP %s)\n' "$hostname" "${status:-none}" >&2
			return 1
		fi
		sleep 15
	done
}

project="$(gcp_edge_resolve_project)" || exit 2

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'gcp edge acceptance dry-run project=%s domain=%s marker=%s originGroup=%s waf=%s armorTraffic=%s failover=%s originTitle=%s trafficImpact=not-run; no GCP mutation invoked\n' \
		"$project" "$domain" "$marker" "$origin_group" "$waf_enabled" "$armor_traffic" "$failover" "${origin_title:-not-requested}"
	exit 0
fi

: "${MAGELIFT_GCP_EDGE_ACCEPTANCE:?set MAGELIFT_GCP_EDGE_ACCEPTANCE=1 for a disposable live GCP edge run}"
if [[ "$MAGELIFT_GCP_EDGE_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live GCP edge acceptance without MAGELIFT_GCP_EDGE_ACCEPTANCE=1\n' >&2
	exit 2
fi

gcloud auth application-default print-access-token >/dev/null 2>&1 || gcloud auth print-access-token >/dev/null
if [[ -n "$origin_title" ]]; then
	got_origin_title="$(gcp_edge_origin_html_title "$origin_host" || true)"
	if [[ "$got_origin_title" != "$origin_title" ]]; then
		printf 'GCP edge origin title mismatch host=%s expected=%q got=%q\n' "$origin_host" "$origin_title" "${got_origin_title:-none}" >&2
		exit 2
	fi
	printf '+ gcp-edge: verified origin HTML title=%q host=%s\n' "$got_origin_title" "$origin_host" >&2
fi
primary_title=""
secondary_title=""
if [[ "$failover" == 1 ]]; then
	primary_title="$(gcp_edge_origin_html_title "$origin_host" || true)"
	secondary_title="$(gcp_edge_origin_html_title "$secondary_origin" || true)"
	if [[ -z "$primary_title" || -z "$secondary_title" || "$primary_title" == "$secondary_title" ]]; then
		printf 'GCP edge failover requires distinct origin HTML titles primary=%q secondary=%q\n' "$primary_title" "$secondary_title" >&2
		exit 2
	fi
	if [[ -n "$origin_title" && "$origin_title" != "$primary_title" ]]; then
		printf 'GCP edge origin title does not match measured primary title expected=%q got=%q\n' "$origin_title" "$primary_title" >&2
		exit 2
	fi
	printf '+ gcp-edge: origin HTML titles primary=%q secondary=%q\n' "$primary_title" "$secondary_title" >&2
fi
owned_count="$(gcp_edge_owned_edge_resource_count "$project")" || exit 2
if (( owned_count != 0 )); then
	printf 'refusing GCP edge acceptance: %s resource(s) already carry magelift ownership descriptions\n' "$owned_count" >&2
	exit 2
fi

cloudflare_acceptance_dns_init

export MAGELIFT_CLEANUP_LEDGER_DIR="${MAGELIFT_CLEANUP_LEDGER_DIR:-$ROOT/.magelift/cleanup}"
export MAGELIFT_CLEANUP_RUN_ID="$run_id"
export MAGELIFT_CLEANUP_MARKER="$marker"
export MAGELIFT_CLEANUP_PROVIDER=gcp
export MAGELIFT_CLEANUP_REGION=global
export MAGELIFT_CLEANUP_PROJECT="$project"
ledger_path="$(acceptance_cleanup_ledger_path "$run_id")"

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-7200}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-gcp-edge-${run_id}-ttl-expired-$$"

owned_comment="$(gcp_edge_owned_comment "$marker")"
backend_created=0
secondary_backend_created=0
neg_created=0
secondary_neg_created=0
cert_created=0
armor_created=0
armor_test_rule_created=0
forwarding_ip=""
url_map_name=""
state_path="${TMPDIR:-/tmp}/magelift-gcp-edge-${run_id}-state-$$.json"
cleanup_status=0
security_policy_args=()
origin_group_args=()
if [[ "$origin_group" == 1 ]]; then
	origin_group_args=(
		--origin-group
		--secondary-backend-service "$secondary_backend_name"
		--secondary-origin-host "$secondary_origin"
	)
fi
traffic_impact=not-run
armor_data_plane=not-run
failover_rto=""

go_gcp_edge() {
	go run ./cmd/gcp-edge-acceptance \
		--project "$project" \
		--marker "$marker" \
		--domain "$domain" \
		--ssl-certificate "$cert_name" \
		--backend-service "$backend_name" \
		--origin-host "$origin_host" \
		"${origin_group_args[@]}" \
		"${security_policy_args[@]}" \
		"$@"
}

cleanup() {
	local exit_status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'GCP edge acceptance TTL expired; forced cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if [[ -n "$forwarding_ip" ]]; then
		if ! gcp_edge_cloudflare_cleanup_a_record "$domain" "$dns_marker" "$forwarding_ip"; then
			cleanup_status=1
		fi
	fi
	if [[ -f "$state_path" ]]; then
		if ! go_gcp_edge --phase destroy --state-path "$state_path" >/dev/null 2>&1; then
			printf 'GCP edge destroy from leftover state failed path=%s\n' "$state_path" >&2
			cleanup_status=1
		fi
		rm -f "$state_path"
	fi
	if (( cert_created == 1 )); then
		if ! gcloud compute ssl-certificates delete "$cert_name" --project="$project" --global --quiet >/dev/null 2>&1; then
			if gcloud compute ssl-certificates describe "$cert_name" --project="$project" --global --format='value(name)' 2>/dev/null | grep -qv '^$'; then
				printf 'managed SSL certificate cleanup failed name=%s\n' "$cert_name" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( backend_created == 1 )); then
		gcp_edge_detach_armor "$project" "$backend_name"
		if ! gcloud compute backend-services delete "$backend_name" --project="$project" --global --quiet >/dev/null 2>&1; then
			if gcloud compute backend-services describe "$backend_name" --project="$project" --global --format='value(name)' 2>/dev/null | grep -qv '^$'; then
				printf 'backend service cleanup failed name=%s\n' "$backend_name" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( secondary_backend_created == 1 )); then
		gcp_edge_detach_armor "$project" "$secondary_backend_name"
		if ! gcloud compute backend-services delete "$secondary_backend_name" --project="$project" --global --quiet >/dev/null 2>&1; then
			if gcloud compute backend-services describe "$secondary_backend_name" --project="$project" --global --format='value(name)' 2>/dev/null | grep -qv '^$'; then
				printf 'secondary backend service cleanup failed name=%s\n' "$secondary_backend_name" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( armor_created == 1 )); then
		if (( armor_test_rule_created == 1 )); then
			if ! gcp_edge_restore_armor_test_rule "$project" "$armor_name"; then
				cleanup_status=1
			fi
			armor_test_rule_created=0
		fi
		if ! gcp_edge_delete_armor "$project" "$armor_name"; then
			cleanup_status=1
		fi
	fi
	if (( neg_created == 1 )); then
		if ! gcloud compute network-endpoint-groups delete "$neg_name" --project="$project" --global --quiet >/dev/null 2>&1; then
			if gcloud compute network-endpoint-groups describe "$neg_name" --project="$project" --global --format='value(name)' 2>/dev/null | grep -qv '^$'; then
				printf 'internet FQDN NEG cleanup failed name=%s\n' "$neg_name" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( secondary_neg_created == 1 )); then
		if ! gcloud compute network-endpoint-groups delete "$secondary_neg_name" --project="$project" --global --quiet >/dev/null 2>&1; then
			if gcloud compute network-endpoint-groups describe "$secondary_neg_name" --project="$project" --global --format='value(name)' 2>/dev/null | grep -qv '^$'; then
				printf 'secondary internet FQDN NEG cleanup failed name=%s\n' "$secondary_neg_name" >&2
				cleanup_status=1
			fi
		fi
	fi
	if (( exit_status != 0 || cleanup_status != 0 )); then
		printf 'GCP edge acceptance failed marker=%s; wrapper cleanup attempted trafficImpact=%s armorDataPlane=%s\n' "$marker" "$traffic_impact" "$armor_data_plane" >&2
	fi
	exit "$(( exit_status != 0 ? exit_status : cleanup_status ))"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

printf '+ gcp-edge: claiming cleanup ledger run_id=%s marker=%s\n' "$run_id" "$marker" >&2
acceptance_cleanup_ledger_claim "$ledger_path" internet-fqdn-neg origin "$neg_name" 20
acceptance_cleanup_ledger_claim "$ledger_path" backend-service origin "$backend_name" 15
if [[ "$origin_group" == 1 ]]; then
	acceptance_cleanup_ledger_claim "$ledger_path" internet-fqdn-neg origin "$secondary_neg_name" 19
	acceptance_cleanup_ledger_claim "$ledger_path" backend-service origin "$secondary_backend_name" 14
fi
acceptance_cleanup_ledger_claim "$ledger_path" ssl-certificate tls "$cert_name" 10

printf '+ gcp-edge: creating Internet FQDN NEG origin=%s\n' "$origin_host" >&2
gcloud compute network-endpoint-groups create "$neg_name" \
	--project="$project" \
	--global \
	--network-endpoint-type=internet-fqdn-port \
	--quiet >/dev/null
neg_created=1
acceptance_cleanup_ledger_record "$ledger_path" internet-fqdn-neg "$neg_name" "$neg_name"
gcloud compute network-endpoint-groups update "$neg_name" \
	--project="$project" \
	--global \
	--add-endpoint="fqdn=${origin_host},port=443" \
	--quiet >/dev/null

printf '+ gcp-edge: creating CDN-enabled backend service name=%s\n' "$backend_name" >&2
gcloud compute backend-services create "$backend_name" \
	--project="$project" \
	--global \
	--load-balancing-scheme=EXTERNAL_MANAGED \
	--protocol=HTTPS \
	--enable-cdn \
	--custom-request-header="Host: ${origin_host}" \
	--description="$owned_comment" \
	--quiet >/dev/null
backend_created=1
acceptance_cleanup_ledger_record "$ledger_path" backend-service "$backend_name" "$backend_name"
gcloud compute backend-services add-backend "$backend_name" \
	--project="$project" \
	--global \
	--network-endpoint-group="$neg_name" \
	--global-network-endpoint-group \
	--quiet >/dev/null

if [[ "$origin_group" == 1 ]]; then
	printf '+ gcp-edge: creating secondary Internet FQDN NEG origin=%s\n' "$secondary_origin" >&2
	gcloud compute network-endpoint-groups create "$secondary_neg_name" \
		--project="$project" \
		--global \
		--network-endpoint-type=internet-fqdn-port \
		--quiet >/dev/null
	secondary_neg_created=1
	acceptance_cleanup_ledger_record "$ledger_path" internet-fqdn-neg "$secondary_neg_name" "$secondary_neg_name"
	gcloud compute network-endpoint-groups update "$secondary_neg_name" \
		--project="$project" \
		--global \
		--add-endpoint="fqdn=${secondary_origin},port=443" \
		--quiet >/dev/null
	printf '+ gcp-edge: creating secondary CDN-enabled backend service name=%s\n' "$secondary_backend_name" >&2
	gcloud compute backend-services create "$secondary_backend_name" \
		--project="$project" \
		--global \
		--load-balancing-scheme=EXTERNAL_MANAGED \
		--protocol=HTTPS \
		--enable-cdn \
		--custom-request-header="Host: ${secondary_origin}" \
		--description="$owned_comment" \
		--quiet >/dev/null
	secondary_backend_created=1
	acceptance_cleanup_ledger_record "$ledger_path" backend-service "$secondary_backend_name" "$secondary_backend_name"
	gcloud compute backend-services add-backend "$secondary_backend_name" \
		--project="$project" \
		--global \
		--network-endpoint-group="$secondary_neg_name" \
		--global-network-endpoint-group \
		--quiet >/dev/null
fi

if [[ "$waf_enabled" == 1 ]]; then
	armor_file="${TMPDIR:-/tmp}/magelift-gcp-edge-${run_id}-armor-$$.json"
	go run ./cmd/magento-waf-rules --document armor --description "$owned_comment" >"$armor_file"
	printf '+ gcp-edge: creating Magento-safe Cloud Armor policy name=%s\n' "$armor_name" >&2
	acceptance_cleanup_ledger_claim "$ledger_path" security-policy waf "$armor_name" 25
	gcloud compute security-policies create "$armor_name" \
		--project="$project" \
		--global \
		--type=CLOUD_ARMOR \
		--description="$owned_comment" \
		--quiet >/dev/null
	armor_created=1
	gcloud compute security-policies update "$armor_name" \
		--project="$project" \
		--global \
		--json-parsing=STANDARD_WITH_GRAPHQL \
		--log-level=VERBOSE \
		--quiet >/dev/null
	fingerprint="$(gcloud compute security-policies describe "$armor_name" --project="$project" --format='value(fingerprint)')"
	if [[ -z "$fingerprint" || "$fingerprint" == "None" ]]; then
		printf 'Cloud Armor create did not return an import fingerprint name=%s\n' "$armor_name" >&2
		exit 2
	fi
	jq --arg fingerprint "$fingerprint" '.fingerprint=$fingerprint' "$armor_file" >"${armor_file}.import"
	gcloud compute security-policies import "$armor_name" \
		--project="$project" \
		--global \
		--file-name="${armor_file}.import" \
		--file-format=json \
		--quiet >/dev/null
	python3 "$ROOT/scripts/acceptance/gcp-armor-beta-body-exclusions.py" apply \
		--project "$project" \
		--policy "$armor_name" \
		--want-json "$armor_file"
	rm -f "$armor_file" "${armor_file}.import"
	acceptance_cleanup_ledger_record "$ledger_path" security-policy "$armor_name" "$armor_name"
	if ! gcp_edge_verify_magento_armor "$project" "$armor_name"; then
		exit 2
	fi
	gcloud compute backend-services update "$backend_name" \
		--project="$project" \
		--global \
		--security-policy="$armor_name" \
		--quiet >/dev/null
	if [[ "$origin_group" == 1 ]]; then
		gcloud compute backend-services update "$secondary_backend_name" \
			--project="$project" \
			--global \
			--security-policy="$armor_name" \
			--quiet >/dev/null
	fi
	security_policy_args=(--security-policy "$armor_name")
	printf '+ gcp-edge: Cloud Armor Magento-safe policy attached name=%s bodyInspection=64KB\n' "$armor_name" >&2
fi

printf '+ gcp-edge: requesting Google-managed SSL certificate domain=%s\n' "$domain" >&2
gcloud compute ssl-certificates create "$cert_name" \
	--project="$project" \
	--domains="$domain" \
	--global \
	--description="$owned_comment" \
	--quiet >/dev/null
cert_created=1
acceptance_cleanup_ledger_record "$ledger_path" ssl-certificate "$cert_name" "$cert_name"

cell_output="$(go_gcp_edge --phase apply --state-path "$state_path")"
printf '%s\n' "$cell_output"

forwarding_ip="$(printf '%s\n' "$cell_output" | sed -n 's/^+ gcp-edge: applied forwardingIPAddress=//p' | tail -n1)"
url_map_name="$(printf '%s\n' "$cell_output" | sed -n 's/^+ gcp-edge: applied urlMapName=//p' | tail -n1)"
if [[ -z "$forwarding_ip" ]]; then
	printf 'GCP edge cell did not report forwardingIPAddress\n' >&2
	exit 2
fi

printf '+ gcp-edge: preparing Cloudflare A record domain=%s address=%s\n' "$domain" "$forwarding_ip" >&2
acceptance_cleanup_ledger_claim "$ledger_path" cloudflare-dns route "$domain" 5
gcp_edge_cloudflare_create_a_record "$domain" "$forwarding_ip" "$dns_marker"
acceptance_cleanup_ledger_record "$ledger_path" cloudflare-dns "$domain" "$domain"
cloudflare_acceptance_dns_wait_for_a "$domain" "$forwarding_ip"

if ! gcp_edge_wait_managed_cert_active "$project" "$cert_name"; then
	printf 'managed certificate did not become ACTIVE for %s; refusing to leave forwarding rule without completed TLS\n' "$domain" >&2
	go_gcp_edge --phase destroy --state-path "$state_path" >/dev/null 2>&1 || true
	exit 2
fi
printf '+ gcp-edge: managed SSL certificate ACTIVE name=%s\n' "$cert_name" >&2

if [[ "$alias_traffic" == 1 ]]; then
	alias_status="$(gcp_edge_wait_alias_https "$domain" "$forwarding_ip")"
	printf '+ gcp-edge: alias HTTPS status=%s domain=%s\n' "$alias_status" "$domain" >&2
	traffic_impact=alias-https
	if [[ -n "$origin_title" ]]; then
		alias_title="$(gcp_edge_wait_alias_html_title "$domain" "$forwarding_ip" "$origin_title")"
		printf '+ gcp-edge: alias origin HTML title=%q domain=%s\n' "$alias_title" "$domain" >&2
		traffic_impact=alias-https-origin
	fi
	if [[ "$failover" == 1 ]]; then
		got_title="$(gcp_edge_wait_alias_html_title "$domain" "$forwarding_ip" "$primary_title")"
		printf '+ gcp-edge: pre-failover HTML title=%q domain=%s\n' "$got_title" "$domain" >&2
		failover_started="$(date +%s)"
		go_gcp_edge --phase failover --state-path "$state_path"
		if [[ -n "$url_map_name" ]]; then
			gcp_edge_invalidate_url_map "$project" "$url_map_name"
		fi
		got_title="$(gcp_edge_wait_alias_html_title "$domain" "$forwarding_ip" "$secondary_title")"
		failover_rto="$(( $(date +%s) - failover_started ))"
		printf '+ gcp-edge: post-failover HTML title=%q rtoSeconds=%s domain=%s\n' "$got_title" "$failover_rto" "$domain" >&2
		traffic_impact=failover-https
	fi
	if [[ "$armor_traffic" == 1 ]]; then
		printf '+ gcp-edge: temporarily updating owned Cloud Armor static/media rule priority=%s for data-plane deny probe (no new rule)\n' "$armor_test_rule_priority" >&2
		if ! gcp_edge_enable_armor_test_rule "$project" "$armor_name"; then
			exit 2
		fi
		armor_test_rule_created=1
		armor_status="$(gcp_edge_wait_armor_block "$domain" "$forwarding_ip")"
		printf '+ gcp-edge: Cloud Armor data-plane status=%s domain=%s\n' "$armor_status" "$domain" >&2
		armor_data_plane=blocked-403
		if ! gcp_edge_restore_armor_test_rule "$project" "$armor_name"; then
			exit 2
		fi
		armor_test_rule_created=0
	fi
fi

destroy_output="$(go_gcp_edge --phase destroy --state-path "$state_path")"
printf '%s\n' "$destroy_output"

if [[ "$failover" == 1 ]]; then
	printf 'GCP edge acceptance complete; ownership inventory empty for marker=%s domain=%s trafficImpact=%s armorDataPlane=%s rtoSeconds=%s\n' "$marker" "$domain" "$traffic_impact" "$armor_data_plane" "$failover_rto"
else
	printf 'GCP edge acceptance complete; ownership inventory empty for marker=%s domain=%s trafficImpact=%s armorDataPlane=%s\n' "$marker" "$domain" "$traffic_impact" "$armor_data_plane"
fi

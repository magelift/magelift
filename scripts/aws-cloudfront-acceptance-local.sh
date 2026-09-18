#!/usr/bin/env bash
# Disposable AWS CloudFront native edge acceptance. The wrapper owns ACM in
# us-east-1, Cloudflare DNS, the cleanup ledger, and the TTL watchdog; the Go
# cell owns plan/apply/purge/destroy and ownership inventory verification.
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
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_AWS_PROFILE:-default}"
run_id="${MAGELIFT_AWS_CLOUDFRONT_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
domain="${MAGELIFT_AWS_CLOUDFRONT_DOMAIN:-ml-cf-${run_id}.acourtiol.com}"
marker="${MAGELIFT_AWS_CLOUDFRONT_MARKER:-magelift/aws/cloudfront/${run_id}}"
primary_origin="${MAGELIFT_AWS_CLOUDFRONT_PRIMARY_ORIGIN:-example.com}"
secondary_origin="${MAGELIFT_AWS_CLOUDFRONT_SECONDARY_ORIGIN:-www.example.com}"
origin_group="${MAGELIFT_AWS_CLOUDFRONT_ORIGIN_GROUP:-1}"
waf_enabled="${MAGELIFT_AWS_CLOUDFRONT_WAF:-0}"
alias_traffic="${MAGELIFT_AWS_CLOUDFRONT_ALIAS_TRAFFIC:-0}"
failover="${MAGELIFT_AWS_CLOUDFRONT_FAILOVER:-0}"
acm_dns_marker="magelift/acceptance/acm-dns/${run_id}"
alias_dns_marker="magelift/acceptance/cf-alias/${run_id}"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$domain" =~ ^[a-z0-9][a-z0-9.-]*[a-z0-9]$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,110}$ || ! "$primary_origin" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$ || ! "$secondary_origin" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$ || "$origin_group" != 0 && "$origin_group" != 1 || "$waf_enabled" != 0 && "$waf_enabled" != 1 || "$alias_traffic" != 0 && "$alias_traffic" != 1 || "$failover" != 0 && "$failover" != 1 ]]; then
	printf 'set valid AWS profile, run ID, domain, marker, and origin hostnames\n' >&2
	exit 2
fi
if [[ "$failover" == 1 && "$origin_group" != 1 ]]; then
	printf 'CloudFront failover requires MAGELIFT_AWS_CLOUDFRONT_ORIGIN_GROUP=1 and distinct origin hostnames\n' >&2
	exit 2
fi

export AWS_PROFILE="$profile"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

cloudfront_owned_distribution_count() {
	local comments count=0 comment
	comments="$(aws cloudfront list-distributions --query 'DistributionList.Items[*].Comment' --output text 2>/dev/null || true)"
	if [[ -z "$comments" || "$comments" == "None" ]]; then
		printf '0'
		return 0
	fi
	for comment in $comments; do
		if [[ "$comment" == magelift\ ownership=* ]]; then
			count=$((count + 1))
		fi
	done
	printf '%s' "$count"
}

cloudfront_acceptance_dns_cleanup_validation() {
	local record_name="${1:?validation record name required}" marker="${2:?ownership marker required}"
	record_name="${record_name%.}"
	local records ids id
	records="$(cloudflare_acceptance_dns_cli dns records list --name-exact "$record_name" --per-page 100 --quiet)"
	ids="$(printf '%s' "$records" | jq -r --arg name "$record_name" --arg marker "$marker" 'if type == "array" then .[] else (.result // [])[] end | select((.name // "" | ascii_downcase) == ($name | ascii_downcase) and (.comment // "") == $marker) | .id')"
	while IFS= read -r id; do
		[[ -z "$id" ]] && continue
		cloudflare_acceptance_dns_cli dns records delete "$id" --force --quiet >/dev/null
	done <<<"$ids"
}

cloudfront_acceptance_dns_create_validation_cname() {
	local record_name="${1:?validation record name required}" target="${2:?validation target required}" marker="${3:?ownership marker required}"
	record_name="${record_name%.}"
	target="${target%.}"
	local body record_id
	body="$(jq -cn --arg name "$record_name" --arg content "$target" --arg comment "$marker" --argjson ttl "$CLOUDFLARE_ACCEPTANCE_TTL_SECONDS" '{type:"CNAME",name:$name,content:$content,ttl:$ttl,proxied:false,comment:$comment}')"
	record="$(cloudflare_acceptance_dns_cli dns records create --body "$body" --quiet)"
	record_id="$(printf '%s' "$record" | jq -r 'if type == "array" then .[0].id // empty else (.id // .result.id // empty) end')"
	if [[ -z "$record_id" ]]; then
		printf 'Cloudflare ACM validation DNS create returned no record ID for %q\n' "$record_name" >&2
		return 1
	fi
}

cloudfront_acceptance_wait_acm_issued() {
	local cert_arn="${1:?certificate ARN required}" deadline status
	deadline=$(( $(date +%s) + 1200 ))
	while :; do
		status="$(aws acm describe-certificate --region us-east-1 --certificate-arn "$cert_arn" --query 'Certificate.Status' --output text 2>/dev/null || true)"
		case "$status" in
			ISSUED)
				return 0
				;;
			FAILED|REVOKED|VALIDATION_TIMED_OUT)
				printf 'ACM certificate entered terminal state %s for %s\n' "$status" "$cert_arn" >&2
				return 1
				;;
		esac
		if (( $(date +%s) >= deadline )); then
			printf 'ACM certificate did not reach ISSUED within 20 minutes: %s\n' "$cert_arn" >&2
			return 1
		fi
		sleep 15
	done
}

cloudfront_owned_waf_count() {
	local names count=0 name
	names="$(aws wafv2 list-web-acls --scope CLOUDFRONT --region us-east-1 --query 'WebACLs[*].Name' --output text 2>/dev/null || true)"
	if [[ -z "$names" || "$names" == "None" ]]; then
		printf '0'
		return 0
	fi
	for name in $names; do
		if [[ "$name" == magelift-cf-waf-* ]]; then
			count=$((count + 1))
		fi
	done
	printf '%s' "$count"
}

cloudfront_verify_magento_waf() {
	local name="${1:?web acl name required}" id="${2:?web acl id required}"
	local acl
	acl="$(aws wafv2 get-web-acl --scope CLOUDFRONT --region us-east-1 --name "$name" --id "$id" --output json)"
	if ! jq -e '
		.WebACL as $acl
		| ($acl.AssociationConfig.RequestBody.CLOUDFRONT.DefaultSizeInspectionLimit == "KB_64")
		and ([ $acl.Rules[]? | select(.OverrideAction.Count != null) ] | length == 0)
		and ([ $acl.Rules[]? | select(.OverrideAction.None != null) ] | length > 0)
		and ([ $acl.Rules[]? | .Statement.ManagedRuleGroupStatement.RuleActionOverrides[]? | select(.Name == "SizeRestrictions_BODY" and .ActionToUse.Count != null) ] | length > 0)
		and ([ $acl.Rules[]? | .Statement.ManagedRuleGroupStatement.RuleActionOverrides[]? | select(.Name == "SQLi_BODY" and .ActionToUse.Count != null) ] | length > 0)
	' <<<"$acl" >/dev/null; then
		printf 'WAFv2 WebACL is not Magento-safe protect mode name=%s id=%s\n' "$name" "$id" >&2
		return 1
	fi
}

cloudfront_delete_waf() {
	local name="${1:?web acl name required}" id="${2:?web acl id required}"
	local token
	token="$(aws wafv2 get-web-acl --scope CLOUDFRONT --region us-east-1 --name "$name" --id "$id" --query LockToken --output text 2>/dev/null || true)"
	if [[ -z "$token" || "$token" == "None" ]]; then
		return 0
	fi
	if ! aws wafv2 delete-web-acl --scope CLOUDFRONT --region us-east-1 --name "$name" --id "$id" --lock-token "$token" >/dev/null 2>&1; then
		if aws wafv2 get-web-acl --scope CLOUDFRONT --region us-east-1 --name "$name" --id "$id" --query Id --output text 2>/dev/null | grep -qv 'None'; then
			printf 'WAFv2 WebACL cleanup failed name=%s id=%s\n' "$name" "$id" >&2
			return 1
		fi
	fi
	return 0
}

cloudfront_wait_alias_https() {
	local hostname="${1:?hostname required}" target="${2:?cloudfront hostname required}" deadline status
	deadline=$(( $(date +%s) + 300 ))
	while :; do
		if dig +short "$hostname" CNAME | grep -q "$target"; then
			status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "https://${hostname}/" || true)"
			if [[ "$status" =~ ^[23][0-9][0-9]$ ]]; then
				printf '%s' "$status"
				return 0
			fi
		fi
		if (( $(date +%s) >= deadline )); then
			printf 'alias HTTPS did not succeed for %s via %s within 5 minutes (last HTTP %s)\n' "$hostname" "$target" "${status:-none}" >&2
			return 1
		fi
		sleep 15
	done
}

cloudfront_html_title() {
	local url="${1:?url required}"
	# macOS tr in a UTF-8 locale exits on illegal bytes in origin HTML (Google).
	LC_ALL=C curl -sS -A 'MageLift-origin-health/1' --max-time 20 "$url" | LC_ALL=C tr '\n' ' ' | grep -oE '<title[^>]*>[^<]+' | head -1 | sed -E 's/<title[^>]*>//;s/^[[:space:]]+//;s/[[:space:]]+$//'
}

cloudfront_wait_html_title() {
	local url="${1:?url required}" want="${2:?title required}" deadline got
	deadline=$(( $(date +%s) + 300 ))
	while :; do
		got="$(cloudfront_html_title "$url" || true)"
		if [[ "$got" == "$want" ]]; then
			printf '%s' "$got"
			return 0
		fi
		if (( $(date +%s) >= deadline )); then
			printf 'CloudFront HTML title did not become %q for %s (last %q)\n' "$want" "$url" "${got:-none}" >&2
			return 1
		fi
		sleep 5
	done
}

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'aws CloudFront acceptance dry-run profile=%s domain=%s marker=%s originGroup=%s waf=%s aliasTraffic=%s failover=%s trafficImpact=not-run; no AWS mutation invoked\n' \
		"$profile" "$domain" "$marker" "$origin_group" "$waf_enabled" "$alias_traffic" "$failover"
	exit 0
fi

dependency_status=0
acceptance_require_commands aws go cf dig curl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_AWS_CLOUDFRONT_ACCEPTANCE:?set MAGELIFT_AWS_CLOUDFRONT_ACCEPTANCE=1 for a disposable live CloudFront run}"
if [[ "$MAGELIFT_AWS_CLOUDFRONT_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live CloudFront acceptance without MAGELIFT_AWS_CLOUDFRONT_ACCEPTANCE=1\n' >&2
	exit 2
fi

aws sts get-caller-identity >/dev/null
owned_count="$(cloudfront_owned_distribution_count)"
if [[ ! "$owned_count" =~ ^[0-9]+$ ]]; then
	printf 'unable to query CloudFront ownership inventory\n' >&2
	exit 2
fi
if (( owned_count != 0 )); then
	printf 'refusing CloudFront acceptance: %s distribution(s) already carry magelift ownership comments\n' "$owned_count" >&2
	exit 2
fi
if [[ "$waf_enabled" == 1 ]]; then
	waf_count="$(cloudfront_owned_waf_count)"
	if [[ ! "$waf_count" =~ ^[0-9]+$ ]]; then
		printf 'unable to query WAFv2 ownership inventory\n' >&2
		exit 2
	fi
	if (( waf_count != 0 )); then
		printf 'refusing CloudFront acceptance: %s magelift-cf-waf WebACL(s) already exist\n' "$waf_count" >&2
		exit 2
	fi
fi

cloudflare_acceptance_dns_init

export MAGELIFT_CLEANUP_LEDGER_DIR="${MAGELIFT_CLEANUP_LEDGER_DIR:-$ROOT/.magelift/cleanup}"
export MAGELIFT_CLEANUP_RUN_ID="$run_id"
export MAGELIFT_CLEANUP_MARKER="$marker"
export MAGELIFT_CLEANUP_PROVIDER=aws
export MAGELIFT_CLEANUP_REGION=us-east-1
export MAGELIFT_CLEANUP_PROFILE="$profile"
ledger_path="$(acceptance_cleanup_ledger_path "$run_id")"

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-2700}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-cloudfront-${run_id}-ttl-expired-$$"

cert_arn=""
waf_name=""
waf_id=""
waf_arn=""
state_path="${TMPDIR:-/tmp}/magelift-aws-cloudfront-${run_id}-state-$$.json"
distribution_domain=""
origin_group_args=()
web_acl_args=()
cleanup_status=0

cleanup() {
	local exit_status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS CloudFront acceptance TTL expired; forced cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if [[ -n "${acm_validation_name:-}" ]]; then
		if ! cloudfront_acceptance_dns_cleanup_validation "$acm_validation_name" "$acm_dns_marker"; then
			cleanup_status=1
		fi
	fi
	if [[ "$alias_traffic" == 1 ]]; then
		if ! cloudfront_acceptance_dns_cleanup_validation "$domain" "$alias_dns_marker"; then
			cleanup_status=1
		fi
	fi
	if [[ -f "$state_path" && -n "$cert_arn" ]]; then
		if ! go run ./cmd/aws-cloudfront-acceptance \
			--profile "$profile" \
			--marker "$marker" \
			--domain "$domain" \
			--certificate-arn "$cert_arn" \
			--primary-origin "$primary_origin" \
			--secondary-origin "$secondary_origin" \
			"${origin_group_args[@]}" \
			"${web_acl_args[@]}" \
			--phase destroy \
			--state-path "$state_path" >/dev/null 2>&1; then
			printf 'CloudFront destroy from leftover state failed path=%s\n' "$state_path" >&2
			cleanup_status=1
		fi
		rm -f "$state_path"
	fi
	if [[ -n "$cert_arn" ]]; then
		if ! aws acm delete-certificate --region us-east-1 --certificate-arn "$cert_arn" >/dev/null 2>&1; then
			if aws acm describe-certificate --region us-east-1 --certificate-arn "$cert_arn" --query 'Certificate.Status' --output text 2>/dev/null | grep -qv 'None'; then
				printf 'ACM certificate cleanup failed arn=%s\n' "$cert_arn" >&2
				cleanup_status=1
			fi
		fi
	fi
	if [[ -n "$waf_name" && -n "$waf_id" ]]; then
		if ! cloudfront_delete_waf "$waf_name" "$waf_id"; then
			cleanup_status=1
		fi
	fi
	if (( exit_status != 0 || cleanup_status != 0 )); then
		printf 'AWS CloudFront acceptance failed marker=%s; ACM and DNS cleanup attempted trafficImpact=not-run\n' "$marker" >&2
	fi
	exit "$(( exit_status != 0 ? exit_status : cleanup_status ))"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

printf '+ cloudfront: requesting ACM certificate domain=%s region=us-east-1\n' "$domain" >&2
acceptance_cleanup_ledger_claim "$ledger_path" acm-certificate source "$domain" 20
cert_arn="$(aws acm request-certificate --region us-east-1 --domain-name "$domain" --validation-method DNS --query CertificateArn --output text)"
acceptance_cleanup_ledger_record "$ledger_path" acm-certificate "$domain" "$cert_arn"

validation_json=""
for _ in {1..30}; do
	validation_json="$(aws acm describe-certificate --region us-east-1 --certificate-arn "$cert_arn" --query 'Certificate.DomainValidationOptions[0].ResourceRecord' --output json 2>/dev/null || true)"
	if [[ -n "$validation_json" && "$validation_json" != "null" ]]; then
		break
	fi
	sleep 2
done
acm_validation_name="$(jq -r '.Name // empty' <<<"$validation_json" | sed 's/\.$//')"
acm_validation_target="$(jq -r '.Value // empty' <<<"$validation_json" | sed 's/\.$//')"
if [[ -z "$acm_validation_name" || -z "$acm_validation_target" ]]; then
	printf 'ACM did not publish DNS validation records for %s; refusing to leave a pending certificate\n' "$cert_arn" >&2
	exit 2
fi

printf '+ cloudfront: preparing ACM validation DNS record=%s\n' "$acm_validation_name" >&2
acceptance_cleanup_ledger_claim "$ledger_path" cloudflare-dns validation "$acm_validation_name" 10
cloudfront_acceptance_dns_create_validation_cname "$acm_validation_name" "$acm_validation_target" "$acm_dns_marker"
acceptance_cleanup_ledger_record "$ledger_path" cloudflare-dns "$acm_validation_name" "$acm_validation_name"

if ! cloudfront_acceptance_wait_acm_issued "$cert_arn"; then
	printf 'ACM issuance blocked for %s; stopping before CloudFront mutation\n' "$domain" >&2
	exit 2
fi
printf '+ cloudfront: ACM certificate issued arn=%s\n' "$cert_arn" >&2

web_acl_args=()
if [[ "$waf_enabled" == 1 ]]; then
	waf_name="magelift-cf-waf-${run_id}"
	waf_rules_file="${TMPDIR:-/tmp}/magelift-aws-cloudfront-${run_id}-waf-rules-$$.json"
	waf_assoc_file="${TMPDIR:-/tmp}/magelift-aws-cloudfront-${run_id}-waf-assoc-$$.json"
	go run ./cmd/magento-waf-rules --document rules --metric-prefix "$waf_name" >"$waf_rules_file"
	go run ./cmd/magento-waf-rules --document association >"$waf_assoc_file"
	printf '+ cloudfront: creating CLOUDFRONT-scope Magento-safe WAFv2 WebACL name=%s\n' "$waf_name" >&2
	acceptance_cleanup_ledger_claim "$ledger_path" waf-web-acl source "$waf_name" 25
	waf_json="$(aws wafv2 create-web-acl \
		--region us-east-1 \
		--scope CLOUDFRONT \
		--name "$waf_name" \
		--description "magelift ownership=${marker}" \
		--default-action Allow={} \
		--visibility-config "SampledRequestsEnabled=true,CloudWatchMetricsEnabled=true,MetricName=${waf_name}" \
		--association-config "file://${waf_assoc_file}" \
		--rules "file://${waf_rules_file}" \
		--tags "Key=magelift:ownership,Value=${marker}")"
	rm -f "$waf_rules_file" "$waf_assoc_file"
	waf_id="$(printf '%s' "$waf_json" | jq -r '.Summary.Id // empty')"
	waf_arn="$(printf '%s' "$waf_json" | jq -r '.Summary.ARN // empty')"
	if [[ -z "$waf_id" || -z "$waf_arn" ]]; then
		printf 'WAFv2 create-web-acl did not return Id and ARN\n' >&2
		exit 2
	fi
	acceptance_cleanup_ledger_record "$ledger_path" waf-web-acl "$waf_name" "$waf_arn"
	if ! cloudfront_verify_magento_waf "$waf_name" "$waf_id"; then
		exit 2
	fi
	web_acl_args=(--web-acl-id "$waf_arn")
	printf '+ cloudfront: WAFv2 Magento-safe WebACL created id=%s bodyInspection=KB_64\n' "$waf_id" >&2
fi

origin_group_args=()
if [[ "$origin_group" == 1 ]]; then
	origin_group_args=(--origin-group)
else
	origin_group_args=(--origin-group=false)
fi

go_cloudfront() {
	go run ./cmd/aws-cloudfront-acceptance \
		--profile "$profile" \
		--marker "$marker" \
		--domain "$domain" \
		--certificate-arn "$cert_arn" \
		--primary-origin "$primary_origin" \
		--secondary-origin "$secondary_origin" \
		"${origin_group_args[@]}" \
		"${web_acl_args[@]}" \
		"$@"
}

primary_title=""
secondary_title=""
if [[ "$failover" == 1 ]]; then
	primary_title="$(cloudfront_html_title "https://${primary_origin}/")"
	secondary_title="$(cloudfront_html_title "https://${secondary_origin}/")"
	if [[ -z "$primary_title" || -z "$secondary_title" || "$primary_title" == "$secondary_title" ]]; then
		printf 'CloudFront failover requires distinct origin HTML titles primary=%q secondary=%q\n' "$primary_title" "$secondary_title" >&2
		exit 2
	fi
	printf '+ cloudfront: origin HTML titles primary=%q secondary=%q\n' "$primary_title" "$secondary_title" >&2
fi

if [[ "$alias_traffic" == 1 || "$failover" == 1 ]]; then
	cell_output="$(go_cloudfront --phase apply --state-path "$state_path")"
	printf '%s\n' "$cell_output"
	distribution_domain="$(printf '%s\n' "$cell_output" | sed -n 's/^+ cloudfront: deployed distributionDomainName=//p' | tail -n1)"
	if [[ -z "$distribution_domain" ]]; then
		printf 'CloudFront apply did not report distributionDomainName\n' >&2
		exit 2
	fi
	viewer_url="https://${distribution_domain}/"
	traffic_impact="not-run"
	if [[ "$alias_traffic" == 1 ]]; then
		printf '+ cloudfront: preparing alias CNAME domain=%s target=%s\n' "$domain" "$distribution_domain" >&2
		acceptance_cleanup_ledger_claim "$ledger_path" cloudflare-dns alias "$domain" 5
		cloudfront_acceptance_dns_create_validation_cname "$domain" "$distribution_domain" "$alias_dns_marker"
		acceptance_cleanup_ledger_record "$ledger_path" cloudflare-dns "$domain" "$domain"
		alias_status="$(cloudfront_wait_alias_https "$domain" "$distribution_domain")"
		printf '+ cloudfront: alias HTTPS status=%s domain=%s\n' "$alias_status" "$domain" >&2
		viewer_url="https://${domain}/"
		traffic_impact="alias-https"
	fi
	if [[ "$failover" == 1 ]]; then
		got_title="$(cloudfront_wait_html_title "$viewer_url" "$primary_title")"
		printf '+ cloudfront: pre-failover HTML title=%q url=%s\n' "$got_title" "$viewer_url" >&2
		failover_started="$(date +%s)"
		go_cloudfront --phase failover --state-path "$state_path"
		got_title="$(cloudfront_wait_html_title "$viewer_url" "$secondary_title")"
		failover_rto="$(( $(date +%s) - failover_started ))"
		printf '+ cloudfront: post-failover HTML title=%q rtoSeconds=%s url=%s\n' "$got_title" "$failover_rto" "$viewer_url" >&2
		traffic_impact="failover-https"
	fi
	go_cloudfront --phase destroy --state-path "$state_path"
	rm -f "$state_path"
	if [[ "$failover" == 1 ]]; then
		printf 'AWS CloudFront acceptance complete; ownership inventory empty for marker=%s domain=%s distributionDomainName=%s trafficImpact=%s rtoSeconds=%s\n' "$marker" "$domain" "$distribution_domain" "$traffic_impact" "$failover_rto"
	else
		printf 'AWS CloudFront acceptance complete; ownership inventory empty for marker=%s domain=%s distributionDomainName=%s trafficImpact=%s\n' "$marker" "$domain" "$distribution_domain" "$traffic_impact"
	fi
else
	go_cloudfront
	printf 'AWS CloudFront acceptance complete; ownership inventory empty for marker=%s domain=%s trafficImpact=not-run\n' "$marker" "$domain"
fi

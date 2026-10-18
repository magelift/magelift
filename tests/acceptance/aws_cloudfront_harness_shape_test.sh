#!/usr/bin/env bash
# Offline contract for the disposable AWS CloudFront native edge cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/aws-cloudfront-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"' "$SCRIPT"
grep -Fq 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT"
grep -Fq 'MAGELIFT_AWS_CLOUDFRONT_ACCEPTANCE' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_claim' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_record' "$SCRIPT"
grep -Fq 'trap cleanup EXIT' "$SCRIPT"
grep -Fq 'acceptance_start_ttl_watchdog' "$SCRIPT"
grep -Fq 'acceptance_stop_ttl_watchdog' "$SCRIPT"
grep -Fq 'cloudfront_owned_distribution_count' "$SCRIPT"
grep -Fq 'carry magelift ownership comments' "$SCRIPT"
grep -Fq 'aws acm delete-certificate' "$SCRIPT"
grep -Fq 'cloudfront_acceptance_dns_cleanup_validation' "$SCRIPT"
grep -Fq 'go run ./cmd/aws-cloudfront-acceptance' "$SCRIPT"
grep -Fq 'go run ./cmd/magento-waf-rules' "$SCRIPT"
grep -Fq 'cloudfront_verify_magento_waf' "$SCRIPT"
grep -Fq 'bodyInspection=KB_64' "$SCRIPT"
grep -Fq 'SizeRestrictions_BODY' "$ROOT/internal/edge/waf/policy.go"
grep -Fq 'trafficImpact=not-run' "$SCRIPT"
grep -Fq 'MAGELIFT_AWS_CLOUDFRONT_FAILOVER' "$SCRIPT"
grep -Fq -- '--phase failover' "$SCRIPT"
grep -Fq 'failover-https' "$SCRIPT"
grep -Fq 'LC_ALL=C' "$SCRIPT"
grep -Fq 'NativeProvider:  "cloudfront"' "$ROOT/cmd/aws-cloudfront-acceptance/main.go"
if grep -Fq 'cloudfront-waf' "$ROOT/cmd/aws-cloudfront-acceptance/main.go"; then
	printf 'CloudFront acceptance must not use cloudfront-waf\n' >&2
	exit 1
fi

output="$(MAGELIFT_AWS_CLOUDFRONT_RUN_ID=shape-test MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT")"
grep -Fq 'no AWS mutation invoked' <<<"$output"
grep -Fq 'domain=ml-cf-shape-test.acourtiol.com' <<<"$output"
grep -Fq 'trafficImpact=not-run' <<<"$output"
n="$(grep -F 'domain=ml-cf-shape-test.acourtiol.com' <<<"$output" | wc -l | tr -d ' ')"
[[ "$n" == 1 ]]

printf 'aws_cloudfront_harness_shape_test OK\n'

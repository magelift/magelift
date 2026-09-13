#!/usr/bin/env bash
# Offline contract for the disposable GCP native edge live cell.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-edge-acceptance-local.sh"

bash -n "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-dependencies.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"' "$SCRIPT"
grep -Fq 'source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"' "$SCRIPT"
grep -Fq 'MAGELIFT_ACCEPTANCE_DRY_RUN' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_ACCEPTANCE' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_claim' "$SCRIPT"
grep -Fq 'acceptance_cleanup_ledger_record' "$SCRIPT"
grep -Fq 'trap cleanup EXIT' "$SCRIPT"
grep -Fq 'acceptance_start_ttl_watchdog' "$SCRIPT"
grep -Fq 'acceptance_stop_ttl_watchdog' "$SCRIPT"
grep -Fq 'gcp_edge_owned_edge_resource_count' "$SCRIPT"
grep -Fq 'carry magelift ownership descriptions' "$SCRIPT"
grep -Fq 'gcloud compute ssl-certificates delete' "$SCRIPT"
grep -Fq 'gcloud compute backend-services delete' "$SCRIPT"
grep -Fq 'internet-fqdn-port' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_CERT_WAIT_SECONDS' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_ALIAS_WAIT_SECONDS' "$SCRIPT"
grep -Fq 'custom-request-header' "$SCRIPT"
grep -Fq 'www.google.com' "$SCRIPT"
grep -Fq 'destroy from leftover state' "$SCRIPT"
grep -Fq 'go run ./cmd/gcp-edge-acceptance' "$SCRIPT"
grep -Fq -- '--phase apply' "$SCRIPT"
grep -Fq -- '--phase destroy' "$SCRIPT"
grep -Fq -- '--phase failover' "$SCRIPT"
grep -Fq -- '--origin-group' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_ORIGIN_GROUP' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_FAILOVER' "$SCRIPT"
grep -Fq 'MAGELIFT_GCP_EDGE_SECONDARY_ORIGIN' "$SCRIPT"
grep -Fq 'traffic_impact=failover-https' "$SCRIPT"
grep -Fq 'gcp_edge_invalidate_url_map' "$SCRIPT"
grep -Fq 'secondary Internet FQDN NEG' "$SCRIPT"
grep -Fq 'go_gcp_edge' "$SCRIPT"
grep -Fq 'trafficImpact=not-run' "$SCRIPT"
grep -Fq -- '--security-policy' "$SCRIPT"
grep -Fq '"security-policy"' "$ROOT/cmd/gcp-edge-acceptance/main.go"
grep -Fq 'MAGELIFT_GCP_EDGE_WAF' "$SCRIPT"
grep -Fq 'go run ./cmd/magento-waf-rules' "$SCRIPT"
grep -Fq 'gcp_edge_verify_magento_armor' "$SCRIPT"
grep -Fq 'bodyInspection=64KB' "$SCRIPT"
grep -Fq 'security-policies import' "$SCRIPT"
grep -Fq 'gcp_edge_enable_armor_test_rule' "$SCRIPT"
grep -Fq 'gcp_edge_restore_armor_test_rule' "$SCRIPT"
grep -Fq 'armor_test_rule_priority=100' "$SCRIPT"
grep -Fq "armor_test_rule_original_description='allow Magento static and media'" "$SCRIPT"
grep -Fq 'armor_test_rule_original_expression=' "$SCRIPT"
grep -Fq 'match.expr.expression' "$SCRIPT"
grep -Fq 'rules update' "$SCRIPT"
grep -Fq 'no new rule' "$SCRIPT"
grep -Fq 'gcp-armor-beta-body-exclusions.py' "$SCRIPT"
grep -Fq 'patchRule' "$ROOT/scripts/acceptance/gcp-armor-beta-body-exclusions.py"
grep -Fq 'compute/beta' "$ROOT/scripts/acceptance/gcp-armor-beta-body-exclusions.py"
grep -Fq '.fingerprint=$fingerprint' "$SCRIPT"
grep -Fq 'security-policies create' "$SCRIPT"
payload="$(cd "$ROOT" && go run ./cmd/magento-waf-rules --document armor --description 'magelift ownership=shape')"
jq -e '
	(.advancedOptionsConfig.jsonParsing == "STANDARD_WITH_GRAPHQL")
	and ((.advancedOptionsConfig.requestBodyInspectionSize // "" | ascii_upcase) == "64KB")
	and ([.rules[]? | select(.match.expr.expression != null and (.match.expr.expression | contains("sensitivity")))] | length > 0)
	and ([.rules[]? | select(.description == "scannerdetection" and .preview != true)] | length > 0)
	and ([.rules[]? | .preconfiguredWafConfig.exclusions[]? | select(.targetRuleSet == "sqli-v33-stable" and ((.requestBodiesToExclude // []) | length) > 0)] | length > 0)
' <<<"$payload" >/dev/null
grep -Fq 'NativeProvider:  "cloud-cdn"' "$ROOT/cmd/gcp-edge-acceptance/main.go"
if grep -Fq 'cloud-armor-allow-all' "$ROOT/cmd/gcp-edge-acceptance/main.go"; then
	printf 'GCP edge acceptance must not use an allow-all Armor policy\n' >&2
	exit 1
fi

output="$(MAGELIFT_GCP_EDGE_RUN_ID=shape-test MAGELIFT_GCP_PROJECT=example-gcp-project MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT")"
grep -Fq 'no GCP mutation invoked' <<<"$output"
grep -Fq 'domain=ml-gcp-edge-shape-test.acourtiol.com' <<<"$output"
grep -Fq 'trafficImpact=not-run' <<<"$output"
grep -Fq 'originGroup=0' <<<"$output"
grep -Fq 'failover=0' <<<"$output"
n="$(grep -F 'domain=ml-gcp-edge-shape-test.acourtiol.com' <<<"$output" | wc -l | tr -d ' ')"
[[ "$n" == 1 ]]

output="$(MAGELIFT_GCP_EDGE_RUN_ID=shape-failover MAGELIFT_GCP_PROJECT=example-gcp-project MAGELIFT_GCP_EDGE_ORIGIN_GROUP=1 MAGELIFT_GCP_EDGE_FAILOVER=1 MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC=1 MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash "$SCRIPT")"
grep -Fq 'originGroup=1' <<<"$output"
grep -Fq 'failover=1' <<<"$output"
grep -Fq 'trafficImpact=not-run' <<<"$output"

printf 'gcp_edge_harness_shape_test OK\n'

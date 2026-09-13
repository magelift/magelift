#!/usr/bin/env bash
# Offline ACCEPT-05: GCP harness shares checkpoint/evidence; expanded catalog;
# live_cell_loop + force_clean/assert_clean shape present; dry-run never ups.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-acceptance-local.sh"
CATALOG="$ROOT/scripts/acceptance/cells-gcp-preview.txt"
HA_CATALOG="$ROOT/scripts/acceptance/cells-gcp-high-availability.txt"
INFRA_CATALOG="$ROOT/scripts/acceptance/cells-gcp-infra-only.txt"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/gcp-checkpoint.json"
export ACCEPTANCE_EVIDENCE="$TMP/gcp-matrix-results.md"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$CATALOG"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_PROVIDER=gcp
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-gcp

REQUIRED_CELLS=(
	bootstrap:wif
	composer:sm-write
	composer:sm-read
	day2:secrets
	day2:state
	day2:logs
	day2:exec
	day2:health
	search:health
	deploy:candidate
	deploy:repeat
	migrate:dump
	cost:estimate
)

for cell in "${REQUIRED_CELLS[@]}"; do
	if ! grep -qxF "$cell" "$CATALOG"; then
		printf 'catalog missing required cell: %s\n' "$cell" >&2
		exit 1
	fi
done

for cell in bootstrap:wif day2:secrets day2:state search:health cost:estimate; do
	if ! grep -qxF "$cell" "$INFRA_CATALOG"; then
		printf 'infra-only catalog missing required cell: %s\n' "$cell" >&2
		exit 1
	fi
done
if ! grep -qxF 'resilience:pod-loss' "$HA_CATALOG"; then
	printf 'high-availability catalog missing resilience:pod-loss\n' >&2
	exit 1
fi
if ! grep -qxF 'resilience:node-loss' "$HA_CATALOG"; then
	printf 'high-availability catalog missing resilience:node-loss\n' >&2
	exit 1
fi
if ! grep -qxF 'resilience:zone-loss' "$HA_CATALOG"; then
	printf 'high-availability catalog missing resilience:zone-loss\n' >&2
	exit 1
fi
if grep -Eq '^(day2:(logs|exec|health)|deploy:(candidate|repeat)|migrate:dump|cutover:dns)$' "$INFRA_CATALOG"; then
	printf 'infra-only catalog contains an application cell\n' >&2
	exit 1
fi

# Structural: live EXIT contract + live_cell_loop symbols
for sym in force_clean_orphans assert_clean live_cell_loop run_gcp_cell run_gcp_pod_loss_cell run_gcp_node_loss_cell run_gcp_zone_loss_cell gcp_acceptance_resolve_zones prove_wif_token_exchange request_psa_peering_delete destroy_with_psa_nudge verify_acceptance_digest expected_gcp_database_version verify_gcp_database_version run_runtime_health_with_retry gcp_apply_magento_storefront_base_url capture_acceptance_wif_ownership restore_acceptance_wif_ownership assert_wif_pool_name_fresh validate_generated_names validate_infra_only_catalog acceptance_network_endpoint_group_rows cleanup_acceptance_network_endpoint_groups wait_for_acceptance_network_endpoint_groups; do
	if ! grep -Eq "^${sym}\\(\\)" "$SCRIPT"; then
		printf 'missing harness symbol: %s\n' "$sym" >&2
		exit 1
	fi
done
for required in 'MAGELIFT_GCP_ACCEPTANCE_NATIVE_EDGE' 'MAGELIFT_GCP_ACCEPTANCE_ORIGIN_DOMAIN' 'gcp_native_edge_prepare_origin' 'gcp_native_edge_verify_origin_https' 'gcp_native_edge_set_home_page_title' 'cms_page' 'MAGELIFT_EDGE_TITLE_B64' 'MAGENTO_DC__OVERRIDE' 'run_gcp_edge_traffic_cell' 'web/unsecure/base_url' 'web/secure/base_url' 'cloudflare_acceptance_dns_prepare_a' 'ManagedCertificate' 'edge:traffic' 'staticContent:' 'themes: \[Magento/blank, Magento/luma\]'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP native edge acceptance contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -Eq "waiting for HTTPS origin domain=.*>&2" "$SCRIPT"; then
	printf 'GCP native edge HTTPS waiter must keep command-substitution stdout clean\n' >&2
	exit 1
fi
for required in 'Keep progress output off' 'if ! cloudflare_acceptance_dns_prepare_a' 'if ! cloudflare_acceptance_dns_wait_for_a' 'Cloud Armor security policies' 'force_clean: deleting Cloud Armor security policy'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP native edge failure-boundary contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
for required in 'network-endpoint-groups list' 'run-VPC network endpoint groups' 'wait_for_acceptance_network_endpoint_groups'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP run-VPC NEG cleanup contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'clear_env.*no' "$SCRIPT"; then
	printf 'GCP image contract must reject PHP-FPM clear_env=yes runtime images\n' >&2
	exit 1
fi
if ! grep -q 'pub/static/deployed_version.txt' "$SCRIPT"; then
	printf 'GCP image contract must reject Magento images without immutable static content\n' >&2
	exit 1
fi
if ! grep -q 'php-fpm --test' "$SCRIPT"; then
	printf 'GCP image contract must validate PHP-FPM configuration syntax\n' >&2
	exit 1
fi
if ! grep -qxF 'edge:traffic' "$ROOT/scripts/acceptance/cells-gcp-edge.txt"; then
	printf 'GCP edge catalog missing edge:traffic\n' >&2
	exit 1
fi
for required in 'WIF_OWNERSHIP_MARKER_FILE' 'cleanup_acceptance_wif'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP harness missing WIF lifecycle marker: %s\n' "$required" >&2
		exit 1
	fi
done
resume_branch_line=$(grep -n 'if should_skip_create_once; then' "$SCRIPT" | tail -1 | cut -d: -f1)
resume_restore_line=$(grep -n '^[[:space:]]*restore_acceptance_wif_ownership$' "$SCRIPT" | tail -1 | cut -d: -f1)
create_capture_line=$(grep -n '^[[:space:]]*if ! capture_acceptance_wif_ownership; then' "$SCRIPT" | tail -1 | cut -d: -f1)
if [[ -z "$resume_branch_line" || -z "$resume_restore_line" || -z "$create_capture_line" ]] || (( resume_branch_line >= resume_restore_line || resume_restore_line >= create_capture_line )); then
	printf 'GCP WIF ownership must restore only on the resume branch before create-once capture\n' >&2
	exit 1
fi
if ! grep -q 'resilience:pod-loss)' "$SCRIPT"; then
	printf 'GCP harness missing resilience:pod-loss dispatch\n' >&2
	exit 1
fi
if ! grep -q 'resilience:node-loss)' "$SCRIPT"; then
	printf 'GCP harness missing resilience:node-loss dispatch\n' >&2
	exit 1
fi
if ! grep -q 'resilience:zone-loss)' "$SCRIPT"; then
	printf 'GCP harness missing resilience:zone-loss dispatch\n' >&2
	exit 1
fi
if ! grep -q 'run_gcp_magento_known_content_check' "$SCRIPT"; then
	printf 'GCP harness missing Magento known-content HA probe\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_HA_MAGENTO_CONTENT_URL' "$SCRIPT" || ! grep -q 'known-content=not-run' "$SCRIPT"; then
	printf 'GCP harness must keep HTTP Magento known-content optional until URL and expect are set\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_HA_MAGENTO_CONTENT_SKU' "$SCRIPT" || ! grep -q './cmd/magento-catalog-sku-probe' "$SCRIPT"; then
	printf 'GCP harness must support Magento catalog SKU known-content without public HTTP\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_HA_MAGENTO_SEED_PROBE' "$SCRIPT" || ! grep -q './cmd/magento-seed-probe' "$SCRIPT" || ! grep -q 'magelift_seed_probe' "$SCRIPT" || ! grep -q 'tiny-fixture' "$SCRIPT" || ! grep -q 'gcp_magento_fetch_one' "$SCRIPT" || ! grep -q 'gcp_apply_magento_seed_probe' "$SCRIPT" || ! grep -q 'magelift-seed-probe.sql' "$SCRIPT" || ! grep -q '/^warning:/' "$SCRIPT" || ! grep -q '/^Defaulted /' "$SCRIPT"; then
	printf 'GCP harness must plant magelift_seed_probe after dump import, probe it on HA cells, and skip exec warning noise\n' >&2
	exit 1
fi
if ! grep -q './cmd/magento-known-content-probe' "$SCRIPT"; then
	printf 'GCP harness must invoke magento-known-content-probe for HA application integrity\n' >&2
	exit 1
fi
for required in 'MAGELIFT_GCP_FAILED_DEPLOY_DIGEST' 'run_gcp_failed_deployment_check' 'failed-deployment=not-run' './cmd/failed-deployment-probe' 'failed-deployment PASS restored='; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP failed-deployment drill contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
for required in 'MAGELIFT_GCP_DESTROY_BACKUPS' 'run_magelift_destroy' 'destroy --yes --destroy-backups' 'leftover Cloud SQL backups remaining=0' 'gcp_cloudsql_instance_name' 'cleanup_leftover_cloudsql_backups'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP Magento destroy-backups contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'deploy:repeat)' "$SCRIPT"; then
	printf 'GCP harness missing deploy:repeat dispatch\n' >&2
	exit 1
fi
for required in 'verify_gcp_magento_version' 'cell-deploy-repeat.json' 'cell-deploy-repeat-health.json' 'cell-deploy-repeat-version.log'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP repeat-deploy verification contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_NAME:-mlacc.*composer-auth' "$SCRIPT"; then
	printf 'GCP harness must default Composer credentials to the marker-owned secret\n' >&2
	exit 1
fi
if ! grep -q 'must use the marker-owned' "$SCRIPT"; then
	printf 'GCP harness must refuse an unowned Composer secret on live runs\n' >&2
	exit 1
fi
for required in 'gcloud compute instances delete' 'spec.providerID' 'instanceIDBefore' 'instanceIDAfter' 'MAGELIFT_GCP_NODE_LOSS_TIMEOUT_SECS'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP node-loss injection contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'GKE managed instance group can recreate the VM under the same' "$SCRIPT" || ! grep -Eq 'replacement="\$node_name"' "$SCRIPT"; then
	printf 'GCP node-loss verifier must accept same-name node reuse after VM identity changes\n' >&2
	exit 1
fi
for required in 'MAGELIFT_GCP_ZONE_LOSS_TIMEOUT_SECS' 'gce-instance-delete-all-ready-nodes-in-zone' 'zoneReadyGapObserved' 'fencing:"not-injected-single-runtime"'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP zone-loss injection contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
for required in 'gcloud compute regions describe' 'MAGELIFT_GCP_ACCEPTANCE_ZONES' 'selected GCP zones'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP zone-selection preflight contract missing: %s\n' "$required" >&2
		exit 1
	fi
done
if grep -q 'GCP_ZONES="\${REGION}-b, \${REGION}-c' "$SCRIPT"; then
	printf 'GCP harness must not hard-code region-b/c/d zone suffixes\n' >&2
	exit 1
fi
for required in 'MAGELIFT_GCP_ACCEPTANCE_BACKEND_URL' 'AUTO_STATE_BUCKET=1' 'PULUMI_BACKEND_URL="gs://'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP harness missing explicit backend ownership marker: %s\n' "$required" >&2
		exit 1
	fi
done
if grep -q 'if \[\[ -z "\${PULUMI_BACKEND_URL:-}" \]\]' "$SCRIPT"; then
	printf 'GCP harness must not trust an ambient PULUMI_BACKEND_URL\n' >&2
	exit 1
fi
if ! grep -q 'PSA\|psa_soak\|PSA_SOAK\|soak' "$SCRIPT"; then
	printf 'missing PSA soak messaging in gcp harness\n' >&2
	exit 1
fi
if ! grep -q 'destroy producers are clear; handing off to exact orphan cleanup' "$SCRIPT"; then
	printf 'GCP destroy must avoid replaying Pulumi after producer inventories are clear\n' >&2
	exit 1
fi
if ! grep -q 'lib-checkpoint.sh' "$SCRIPT" || ! grep -q 'lib-evidence.sh' "$SCRIPT"; then
	printf 'gcp script must source shared checkpoint/evidence libs\n' >&2
	exit 1
fi
if ! grep -q 'acceptance_checkpoint_bind_run_id' "$SCRIPT"; then
	printf 'GCP KEEP resume must bind JSONL run id from the checkpoint\n' >&2
	exit 1
fi
if ! grep -q 'append_row' "$SCRIPT"; then
	printf 'gcp script must call append_row for evidence\n' >&2
	exit 1
fi
# bootstrap:wif must refuse Ensure-only
if ! grep -q 'ENSURE_ONLY\|Ensure-only\|not Ensure-only\|Act/STS\|token exchange' "$SCRIPT"; then
	printf 'bootstrap:wif must require Act/STS proof (not Ensure-only)\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_DIGEST' "$SCRIPT"; then
	printf 'DIGEST pullable documentation gate missing\n' >&2
	exit 1
fi
for required in 'lib-cosign.sh' 'acceptance_require_certificate_identity' 'acceptance_maybe_sign_digest' 'acceptance_promote_digest' 'Cosign journal before Magento deploy:candidate'; do
	if ! grep -Fq -- "$required" "$SCRIPT"; then
		printf 'GCP Magento create-once must promote a Cosign-verifiable digest: %s\n' "$required" >&2
		exit 1
	fi
done
for required in '--certificate-identity' '--certificate-oidc-issuer' 'MAGELIFT_CERTIFICATE_IDENTITY'; do
	if ! grep -Fq -- "$required" "$ROOT/scripts/acceptance/lib-cosign.sh"; then
		printf 'shared Cosign acceptance lib missing %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -Fq -- 'MAGELIFT_GCP_CERTIFICATE_IDENTITY' "$SCRIPT"; then
	printf 'GCP harness must keep MAGELIFT_GCP_CERTIFICATE_IDENTITY as a Cosign alias\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY' "$SCRIPT" || ! grep -q 'infra-only catalog contains application cell' "$SCRIPT"; then
	printf 'explicit fail-closed infra-only catalog gate missing\n' >&2
	exit 1
fi
if ! grep -q 'cells-gcp-\${PROFILE}\.txt' "$SCRIPT"; then
	printf 'profile-specific default cell catalog missing\n' >&2
	exit 1
fi
if ! grep -q 'gcloud artifacts docker images describe' "$SCRIPT"; then
	printf 'Artifact Registry digest existence check missing\n' >&2
	exit 1
fi
if ! grep -q 'databaseVersion' "$SCRIPT"; then
	printf 'Cloud SQL database version assertion missing\n' >&2
	exit 1
fi
if ! grep -q 'private-2 public-0 public-1 public-2' "$SCRIPT"; then
	printf 'HA force-clean must delete all three private/public subnet pairs\n' >&2
	exit 1
fi
if ! grep -q 'run_gcp_exec_cell' "$SCRIPT" || ! grep -q 'No agent available' "$SCRIPT"; then
	printf 'day2:exec must retry only the GKE Konnectivity agent error\n' >&2
	exit 1
fi
# cleanup path: destroy then force_clean then assert_clean (live EXIT contract)
cleanup_block=$(awk '/^cleanup\(\)/,/^}/' "$SCRIPT")
if ! printf '%s\n' "$cleanup_block" | grep -q 'destroy'; then
	printf 'EXIT cleanup must call destroy when created\n' >&2
	exit 1
fi
if ! printf '%s\n' "$cleanup_block" | grep -q 'force_clean_orphans'; then
	printf 'EXIT cleanup must call force_clean_orphans\n' >&2
	exit 1
fi
if ! printf '%s\n' "$cleanup_block" | grep -q 'assert_clean'; then
	printf 'EXIT cleanup must call assert_clean\n' >&2
	exit 1
fi
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE_KEEP' "$SCRIPT"; then
	printf 'KEEP gate missing from gcp harness\n' >&2
	exit 1
fi
if ! grep -q 'ACCEPTANCE_CHECKPOINT_FINGERPRINT' "$SCRIPT" || ! grep -q 'provider=gcp' "$SCRIPT"; then
	printf 'configuration fingerprint missing from GCP resume path\n' >&2
	exit 1
fi
for required in 'acceptance_export_reuse_boundary' 'gcp-cloudsql-automated:' 'gke-native-workloads'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'GCP harness missing reuse-boundary export: %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'Keep Pulumi.*dependency order intact' "$SCRIPT"; then
	printf 'GCP cleanup must preserve Pulumi dependency order\n' >&2
	exit 1
fi
if ! grep -q 'force_clean_orphans remains the final' "$SCRIPT"; then
	printf 'GCP cleanup must retain force_clean_orphans as the final authoritative cleanup path\n' >&2
	exit 1
fi
# Refuse live without gate (T-07-11)
if ! grep -q 'MAGELIFT_GCP_ACCEPTANCE' "$SCRIPT"; then
	printf 'MAGELIFT_GCP_ACCEPTANCE refuse gate missing\n' >&2
	exit 1
fi

# A full GCP run must reject a nearly-full local volume before it enables APIs
# or starts the serial provider-specific Go build. This keeps a local ENOSPC
# from becoming the first failure after a paid-scope mutation begins.
free_space_line=$(grep -n 'acceptance_require_local_free_space' "$SCRIPT" | head -1 | cut -d: -f1)
api_enable_line=$(grep -n 'gcloud services enable' "$SCRIPT" | head -1 | cut -d: -f1)
build_line=$(grep -n 'go build -trimpath -ldflags' "$SCRIPT" | head -1 | cut -d: -f1)
if [[ -z "$free_space_line" || -z "$api_enable_line" || -z "$build_line" ]] || (( free_space_line >= api_enable_line || free_space_line >= build_line )); then
	printf 'GCP local free-space admission must precede API enablement and the provider build\n' >&2
	exit 1
fi

LOG="$TMP/gcp-dry-run.log"
set +e
bash "$SCRIPT" >"$LOG" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
	printf 'gcp dry-run exited %s\n' "$rc" >&2
	cat "$LOG" >&2
	exit 1
fi

if ! grep -q 'gcp acceptance dry-run ok' "$LOG"; then
	printf 'expected dry-run ok marker\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q 'created=0' "$LOG"; then
	printf 'dry-run must report created=0\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q 'live_cell_loop' "$LOG"; then
	printf 'dry-run should mention live_cell_loop shape marker\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if grep -Eq '\+ magelift (up|destroy)|gcloud .+ create|created=1' "$LOG"; then
	printf 'dry-run must not invoke spending mutate or set created=1\n' >&2
	cat "$LOG" >&2
	exit 1
fi

if [[ ! -f "$ACCEPTANCE_CHECKPOINT" ]]; then
	printf 'checkpoint not written: %s\n' "$ACCEPTANCE_CHECKPOINT" >&2
	exit 1
fi
for cell in "${REQUIRED_CELLS[@]}"; do
	if ! jq -e --arg c "$cell" '.cells[$c]' "$ACCEPTANCE_CHECKPOINT" >/dev/null; then
		printf 'checkpoint missing cell: %s\n' "$cell" >&2
		cat "$ACCEPTANCE_CHECKPOINT" >&2
		exit 1
	fi
done
if [[ ! -f "$ACCEPTANCE_EVIDENCE" ]]; then
	printf 'evidence not written\n' >&2
	exit 1
fi
for cell in bootstrap:wif deploy:candidate deploy:repeat migrate:dump; do
	if ! grep -q "| ${cell} | PASS |" "$ACCEPTANCE_EVIDENCE"; then
		printf 'evidence row missing for %s\n' "$cell" >&2
		cat "$ACCEPTANCE_EVIDENCE" >&2
		exit 1
	fi
done
if ! grep -q '| gcp |' "$ACCEPTANCE_EVIDENCE"; then
	printf 'evidence provider must be gcp\n' >&2
	cat "$ACCEPTANCE_EVIDENCE" >&2
	exit 1
fi

# Resume: second dry-run must skip completed cells
LOG2="$TMP/gcp-dry-run-resume.log"
set +e
bash "$SCRIPT" >"$LOG2" 2>&1
rc2=$?
set -e
if [[ "$rc2" -ne 0 ]]; then
	printf 'gcp dry-run resume exited %s\n' "$rc2" >&2
	cat "$LOG2" >&2
	exit 1
fi
if ! grep -q 'acceptance skip cell=bootstrap:wif (checkpoint)' "$LOG2"; then
	printf 'resume must skip first complete cell via checkpoint\n' >&2
	cat "$LOG2" >&2
	exit 1
fi

printf 'gcp_harness_shape_test OK\n'

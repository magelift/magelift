#!/usr/bin/env bash
# Local, credit-efficient GCP acceptance: preview by default, destroy on EXIT.
# GKE Autopilot/Standard acceptance path. Never leave GKE/SQL/Valkey running.
# Dry-run (MAGELIFT_ACCEPTANCE_DRY_RUN=1): fixture path; no Pulumi/gcloud create.
# Live up: one create-once, then live_cell_loop catalog updates; never recreate between cells.
# Evidence: .magelift/gcp-matrix/matrix-results.md (six-column append_row only; no hand edits).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck source=acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-dependencies.sh
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-cloudflare-dns.sh
source "$ROOT/scripts/acceptance/lib-cloudflare-dns.sh"
# shellcheck source=acceptance/lib-cosign.sh
source "$ROOT/scripts/acceptance/lib-cosign.sh"
# shellcheck source=acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

json_dependency_status=0
acceptance_campaign_go_memlimit
acceptance_require_jq || json_dependency_status=1

DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}"
case "${MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY:-0}" in
1|true) INFRA_ONLY=1 ;;
0|false|"") INFRA_ONLY=0 ;;
*)
	printf 'MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY must be 1/true or 0/false\n' >&2
	exit 2
	;;
esac
PROFILE="${MAGELIFT_GCP_ACCEPTANCE_PROFILE:-preview}"
case "$PROFILE" in
preview|standard|high-availability)
	DEFAULT_CELL_CATALOG="$ROOT/scripts/acceptance/cells-gcp-${PROFILE}.txt"
	;;
*)
DEFAULT_CELL_CATALOG="$ROOT/scripts/acceptance/cells-gcp-preview.txt"
	;;
esac
if [[ "$INFRA_ONLY" == 1 && -z "${MAGELIFT_GCP_ACCEPTANCE_CELL_CATALOG:-}" && -z "${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-}" ]]; then
	DEFAULT_CELL_CATALOG="$ROOT/scripts/acceptance/cells-gcp-infra-only.txt"
fi
CELL_CATALOG="${MAGELIFT_GCP_ACCEPTANCE_CELL_CATALOG:-${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$DEFAULT_CELL_CATALOG}}"
CELLS=()

# GCP-scoped paths so AWS/GCP evidence do not clobber (ACCEPT-05).
# lib-checkpoint.sh / lib-evidence.sh assign AWS defaults (.magelift/acceptance-*)
# at source time; ${VAR:-gcp} would keep those and make should_skip_create_once
# see AWS PASS cells → skip GCP create-once. Force GCP matrix unless the caller
# set MAGELIFT_* or an absolute/tmp path (shape test).
gcp_acceptance_paths() {
	local state_root="${MAGELIFT_GCP_ACCEPTANCE_DIR:-.magelift}"
	local checkpoint_scope="${MAGELIFT_GCP_ACCEPTANCE_NAME:-gcp}"
	if [[ "$DRY_RUN" == 1 || "$DRY_RUN" == true ]]; then
		# A fixture run must never make a later live run skip its first cell.
		checkpoint_scope="${checkpoint_scope}-dry-run"
	fi
	if [[ -n "${MAGELIFT_ACCEPTANCE_CHECKPOINT:-}" ]]; then
		export ACCEPTANCE_CHECKPOINT="$MAGELIFT_ACCEPTANCE_CHECKPOINT"
	elif [[ "${ACCEPTANCE_CHECKPOINT:-}" == /* || "${ACCEPTANCE_CHECKPOINT:-}" == *gcp-matrix* ]]; then
		export ACCEPTANCE_CHECKPOINT="$ACCEPTANCE_CHECKPOINT"
	else
		export ACCEPTANCE_CHECKPOINT="${state_root%/}/gcp-matrix/${checkpoint_scope}/acceptance-checkpoint.json"
	fi
	if [[ -n "${MAGELIFT_ACCEPTANCE_EVIDENCE:-}" ]]; then
		export ACCEPTANCE_EVIDENCE="$MAGELIFT_ACCEPTANCE_EVIDENCE"
	elif [[ "${ACCEPTANCE_EVIDENCE:-}" == /* || "${ACCEPTANCE_EVIDENCE:-}" == *gcp-matrix* ]]; then
		export ACCEPTANCE_EVIDENCE="$ACCEPTANCE_EVIDENCE"
	else
		export ACCEPTANCE_EVIDENCE="${state_root%/}/gcp-matrix/matrix-results.md"
	fi
	acceptance_checkpoint_bind_run_id
}

load_cells() {
	local line
	CELLS=()
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
		CELLS+=("$line")
	done <"$CELL_CATALOG"
	if [[ ${#CELLS[@]} -eq 0 ]]; then
		printf 'no cells in catalog: %s\n' "$CELL_CATALOG" >&2
		exit 2
	fi
}

gcp_acceptance_resolve_zones() {
	local requested zone zone_uri zones_raw required_zones already_zone already
	local -a selected_zones=()
	requested="${MAGELIFT_GCP_ACCEPTANCE_ZONES:-}"
	required_zones=2
	if [[ "$PROFILE" == high-availability ]]; then
		required_zones=3
	fi
	if [[ -n "$requested" ]]; then
		# A caller-supplied list is useful when the provider has capacity in only
		# a subset of the region. Validate it against the configured region before
		# it reaches the YAML config; never treat arbitrary text as a zone.
		requested="${requested//,/ }"
		for zone in $requested; do
			if [[ ! "$zone" =~ ^[a-z][a-z0-9-]+[a-z0-9]$ || "$zone" != "$REGION"-* ]]; then
				printf 'MAGELIFT_GCP_ACCEPTANCE_ZONES contains an invalid zone for region %s: %s\n' "$REGION" "$zone" >&2
				return 2
			fi
			already=0
			for already_zone in "${selected_zones[@]}"; do
				if [[ "$already_zone" == "$zone" ]]; then
					already=1
					break
				fi
			done
			if [[ "$already" == 0 ]]; then
				selected_zones+=("$zone")
			fi
		done
	else
		if ! zones_raw="$(gcloud compute regions describe "$REGION" --project="$PROJECT" --format='value(zones)' 2>/dev/null)"; then
			printf 'could not resolve the available GCP zones for region %s before config generation\n' "$REGION" >&2
			return 1
		fi
		while IFS= read -r zone_uri; do
			zone="${zone_uri##*/}"
			if [[ ! "$zone" =~ ^[a-z][a-z0-9-]+[a-z0-9]$ || "$zone" != "$REGION"-* ]]; then
				continue
			fi
			already=0
			for already_zone in "${selected_zones[@]}"; do
				if [[ "$already_zone" == "$zone" ]]; then
					already=1
					break
				fi
			done
			if [[ "$already" == 0 ]]; then
				selected_zones+=("$zone")
			fi
		done < <(printf '%s' "$zones_raw" | tr ';' '\n' | sort)
	fi
	if [[ "${#selected_zones[@]}" -lt "$required_zones" ]]; then
		printf 'GCP region %s provides %s validated zone(s), but profile %s requires at least %s\n' "$REGION" "${#selected_zones[@]}" "$PROFILE" "$required_zones" >&2
		return 2
	fi
	GCP_ZONES="${selected_zones[0]}"
	local index
	for ((index = 1; index < required_zones; index++)); do
		GCP_ZONES+=", ${selected_zones[index]}"
	done
	printf '+ selected GCP zones region=%s profile=%s zones=%s\n' "$REGION" "$PROFILE" "$GCP_ZONES"
}

validate_infra_only_catalog() {
	[[ "$INFRA_ONLY" == 1 ]] || return 0
	load_cells
	local cell
	for cell in "${CELLS[@]}"; do
		case "$cell" in
		bootstrap:wif|day2:secrets|day2:state|search:health|queue:health|cost:estimate)
			;;
		composer:sm-write|composer:sm-read)
			if [[ -z "${MAGELIFT_GCP_COMPOSER_SECRET_ID:-}" || "$COMPOSER_SECRET_ID" != "${NAME}-"* ]]; then
				printf 'infra-only Composer cell %s requires MAGELIFT_GCP_COMPOSER_SECRET_ID with the run-owned %s- prefix; refusing to mutate an unmarked secret\n' "$cell" "$NAME" >&2
				return 2
			fi
			;;
		*)
			printf 'infra-only catalog contains application cell %s; use a catalog containing only bootstrap, managed-service health, state/secret, and cost cells\n' "$cell" >&2
			return 2
			;;
		esac
	done
	printf '+ infra-only catalog validated; Magento image, dump, deploy, exec, logs, and DNS cells are excluded\n'
}

gcp_dry_run_cell_loop() {
	gcp_acceptance_paths
	local cell provider account date_s duration started created_once=0
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-gcp}"
	account="${MAGELIFT_ACCEPTANCE_ACCOUNT:-dry-run}"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure
	printf 'gcp acceptance dry-run start cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2
	# Structural markers: EXIT contract symbols remain defined in live path below.
	printf 'gcp harness shape: live_cell_loop + force_clean_orphans + assert_clean + PSA soak (live Phase 7)\n' >&2
	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi
		if [[ "$created_once" -eq 0 ]]; then
			printf 'acceptance create-once\n' >&2
			created_once=1
		fi
		printf 'acceptance cell-update cell=%s\n' "$cell" >&2
		started=$(date +%s)
		duration="$(( $(date +%s) - started ))s"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "PASS" "$duration" "$provider" "$account" "$date_s"
		append_shared_row "$cell" "PASS" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "PASS"
		printf 'acceptance cell-done cell=%s result=PASS\n' "$cell" >&2
	done
	# Dry-run never sets created=1 / never calls up / never invokes gcloud mutate.
	printf 'gcp acceptance dry-run ok; created=0; no GCP up invoked\n' >&2
}

if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
	if (( json_dependency_status != 0 )); then
		exit 2
	fi
	gcp_dry_run_cell_loop
	exit 0
fi

if [[ "${MAGELIFT_GCP_ACCEPTANCE:-}" != "1" ]]; then
	printf 'refusing to run without MAGELIFT_GCP_ACCEPTANCE=1 (or MAGELIFT_ACCEPTANCE_DRY_RUN=1)\n' >&2
	exit 2
fi

dependency_status="$json_dependency_status"
acceptance_require_commands gcloud docker pulumi go kubectl gke-gcloud-auth-plugin curl openssl shasum || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

WORKDIR="${MAGELIFT_GCP_ACCEPTANCE_DIR:-/tmp/magelift-gcp-wt}"
# Go/Pulumi builds can use substantially more temporary space than the final
# binary. Refuse the run before ADC/API/state setup when the local volume is
# too full; a later build failure must not be the first signal that a paid run
# cannot start. The threshold is deliberately configurable for operators who
# have measured a smaller build footprint for a warm binary.
GCP_LOCAL_FREE_SPACE_MB="${MAGELIFT_GCP_ACCEPTANCE_MIN_FREE_MB:-12288}"
if ! acceptance_require_local_free_space "$GCP_LOCAL_FREE_SPACE_MB" "$ROOT"; then
	exit 2
fi

# Live runs require an operator-owned disposable project; never commit real IDs.
if [[ -z "${MAGELIFT_GCP_PROJECT:-}" ]]; then
	if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-}" == "1" ]]; then
		PROJECT="example-gcp-project"
	else
		printf 'set MAGELIFT_GCP_PROJECT to your disposable GCP project (no default for live runs)\n' >&2
		exit 2
	fi
else
	PROJECT="${MAGELIFT_GCP_PROJECT}"
fi
REGION="${MAGELIFT_GCP_REGION:-europe-west1}"
MODE="${1:-preview}"
# A pullable OCI digest is required for Magento day-2 / deploy:candidate /
# health cells. Explicit infra-only runs intentionally retain the placeholder
# because they never schedule the Magento image and validate their catalog
# before creating any cloud resource.
DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-ghcr.io/example/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
MAGENTO_VERSION="${MAGELIFT_GCP_ACCEPTANCE_VERSION:-2.4.9}"
PHP_VERSION="${MAGELIFT_GCP_ACCEPTANCE_PHP:-8.5}"
COMPOSER_VERSION="${MAGELIFT_GCP_ACCEPTANCE_COMPOSER_VERSION:-2.10}"
PHP_EXTENSIONS="${MAGELIFT_GCP_ACCEPTANCE_EXTENSIONS:-intl,pdo_mysql}"
MEMORYSTORE_ENGINE_VERSION="${MAGELIFT_GCP_ACCEPTANCE_MEMORYSTORE_ENGINE_VERSION:-}"
OPENSEARCH_IMAGE="${MAGELIFT_GCP_OPENSEARCH_IMAGE:-opensearchproject/opensearch:3@sha256:bcc1797519726ceb6d651d4a3e60b7c30da91793914a8dfe75fd441d4f641509}"
RABBITMQ_IMAGE="${MAGELIFT_GCP_RABBITMQ_IMAGE:-rabbitmq:4.3-management-alpine@sha256:cd624335f752f704e768239ea21501e5771ca13b3b278520da5ad1076eb86e55}"
ENABLE_CLOUD_ARMOR="${MAGELIFT_GCP_ACCEPTANCE_ENABLE_CLOUD_ARMOR:-false}"
AUTOPILOT_MEMORY_REQUEST="${MAGELIFT_GCP_ACCEPTANCE_MEMORY_REQUEST:-2Gi}"
case "${MAGELIFT_GCP_ACCEPTANCE_NATIVE_EDGE:-0}" in
1|true) NATIVE_EDGE_ENABLED=1 ;;
0|false|"") NATIVE_EDGE_ENABLED=0 ;;
*)
	printf 'MAGELIFT_GCP_ACCEPTANCE_NATIVE_EDGE must be 1/true or 0/false\n' >&2
	exit 2
	;;
esac
ORIGIN_DOMAIN="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_DOMAIN:-}"
ORIGIN_TITLE="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_TITLE:-}"
GITHUB_OWNER="${MAGELIFT_GCP_ACCEPTANCE_GITHUB_OWNER:-${GITHUB_REPOSITORY_OWNER:-magelift}}"
GITHUB_REPO="${MAGELIFT_GCP_ACCEPTANCE_GITHUB_REPO:-magelift}"
COMPOSER_SECRET_ID="${MAGELIFT_GCP_COMPOSER_SECRET_ID:-${MAGELIFT_GCP_ACCEPTANCE_NAME:-mlacc}-${PROFILE}-composer-auth}"
SEED_DUMP="${MAGELIFT_GCP_ACCEPTANCE_SEED_DUMP:-$ROOT/testdata/fixtures/migrate/tiny.sql}"
CUTOVER_HOST="${MAGELIFT_CUTOVER_HOST:-magelift-preview.example.com}"

# Magento up records deploy journal rows without Cosign fields. Rollback copies
# promote signatures by digest, so create-once must promote a signed immutable
# digest before deploy:candidate (same contract as AWS acceptance).
# MAGELIFT_GCP_CERTIFICATE_IDENTITY remains a provider alias for
# MAGELIFT_CERTIFICATE_IDENTITY.
if [[ "$MODE" == up && "$INFRA_ONLY" != 1 ]]; then
	acceptance_require_certificate_identity "${MAGELIFT_GCP_CERTIFICATE_IDENTITY:-}" "${MAGELIFT_GCP_CERTIFICATE_OIDC_ISSUER:-}"
fi

# A live run must fail before any Pulumi state bucket, WIF identity, or paid
# provider resource is created when the configured dump is only an offline
# synthetic fixture. The deploy:candidate cell runs setup:upgrade against the
# imported database and therefore requires an installed Magento schema.
if [[ "$MODE" == up && "$INFRA_ONLY" != 1 ]] && ! acceptance_validate_magento_seed_dump "$SEED_DUMP"; then
	exit 2
fi

# Multi-node OpenSearch requires node-level sysctl access, which GKE
# Autopilot intentionally does not provide. Keep Autopilot for the smaller
# cells and use the explicit Standard runtime for HA cells.
GKE_RUNTIME="${MAGELIFT_GCP_ACCEPTANCE_RUNTIME:-gke-autopilot}"
if [[ "$PROFILE" == high-availability && -z "${MAGELIFT_GCP_ACCEPTANCE_RUNTIME:-}" ]]; then
	GKE_RUNTIME="gke-standard"
fi
if [[ "$PROFILE" == high-availability && "$GKE_RUNTIME" == gke-autopilot ]]; then
	printf 'high-availability requires gke-standard (OpenSearch vm.max_map_count). Unset MAGELIFT_GCP_ACCEPTANCE_RUNTIME or set MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-standard.\n' >&2
	exit 2
fi
# Do not inherit MAGELIFT_KUBECONFIG from a previous KEEP. GCP can reuse the
# GKE public endpoint IP; the stale CA then fails day2:logs with x509
# unknown authority.
if [[ -n "${MAGELIFT_KUBECONFIG:-}" ]]; then
	printf '+ dropping inherited MAGELIFT_KUBECONFIG; will refresh after create-once\n'
	unset MAGELIFT_KUBECONFIG
fi
# Isolation prefix for this worktree; override per operator; do not reuse shared prefixes.
NAME="${MAGELIFT_GCP_ACCEPTANCE_NAME:-mlacc}"
if [[ -z "${MAGELIFT_GCP_ACCEPTANCE_NAME:-}" ]]; then
	printf 'set MAGELIFT_GCP_ACCEPTANCE_NAME to a unique disposable project name for live runs\n' >&2
	exit 2
fi
if ! acceptance_campaign_require_prefix gcp "$NAME"; then
	exit 2
fi
if ! acceptance_campaign_require_isolated_backend "${MAGELIFT_GCP_ACCEPTANCE_BACKEND_URL:-}"; then
	exit 2
fi
if [[ "$NATIVE_EDGE_ENABLED" == 1 ]]; then
	if [[ -z "$ORIGIN_DOMAIN" ]]; then
		printf 'MAGELIFT_GCP_ACCEPTANCE_ORIGIN_DOMAIN is required when MAGELIFT_GCP_ACCEPTANCE_NATIVE_EDGE=1\n' >&2
		exit 2
	fi
	if [[ -z "$ORIGIN_TITLE" ]]; then
		ORIGIN_TITLE="MageLift Acceptance ${NAME} Magento ${MAGENTO_VERSION}"
	fi
	if [[ "$ORIGIN_TITLE" == *$'\r'* || "$ORIGIN_TITLE" == *$'\n'* ]]; then
		printf 'MAGELIFT_GCP_ACCEPTANCE_ORIGIN_TITLE must be single-line\n' >&2
		exit 2
	fi
	acceptance_require_commands cf dig || dependency_status=1
	if (( dependency_status == 0 )); then
		cloudflare_acceptance_dns_init
		cloudflare_acceptance_dns_validate_domain "$ORIGIN_DOMAIN"
	fi
	if [[ "$ENABLE_CLOUD_ARMOR" != true && "$ENABLE_CLOUD_ARMOR" != 1 ]]; then
		printf 'native GCP edge acceptance requires MAGELIFT_GCP_ACCEPTANCE_ENABLE_CLOUD_ARMOR=true\n' >&2
		exit 2
	fi
fi
if (( dependency_status != 0 )); then
	exit 2
fi
if [[ "$MODE" == up && "$COMPOSER_SECRET_ID" != "${NAME}-"* ]]; then
	printf 'MAGELIFT_GCP_COMPOSER_SECRET_ID must use the marker-owned %s- prefix; refusing to mutate an unowned Composer secret\n' "$NAME" >&2
	exit 2
fi
ENCRYPTION_KEY_SECRET_ID="${MAGELIFT_GCP_ENCRYPTION_KEY_SECRET_ID:-${NAME}-${PROFILE}-magento-crypt-key}"
ENCRYPTION_KEY_ACCEPTANCE_OWNED=0
# Magelift DIY stack identity is project-env-provider-runtime (see platform.FormatStackName).
STACK_NAME="${NAME}-${PROFILE}-gcp-${GKE_RUNTIME}"
CLUSTER_NAME="${NAME}-${PROFILE}-gke"
if [[ "$GKE_RUNTIME" == gke-standard ]]; then
	CLUSTER_NAME="${CLUSTER_NAME}-standard"
fi
PULUMI_PROJECT="magelift"
LOG_DIR="${WORKDIR}/logs"
BIN="${MAGELIFT_GCP_ACCEPTANCE_BIN:-${WORKDIR}/magelift}"
CONFIG="${WORKDIR}/magelift.yaml"
COMPOSER_SM_URI="gcp-secret-manager://projects/${PROJECT}/secrets/${COMPOSER_SECRET_ID}/versions/latest"
WIF_POOL_ID="ml-${NAME}-${PROFILE}"
WIF_PROVIDER_ID="github"
WIF_SERVICE_ACCOUNT_EMAIL="ml-${NAME}-${PROFILE}-ci@${PROJECT}.iam.gserviceaccount.com"
WIF_OWNERSHIP_MARKER_FILE="${WORKDIR}/wif-acceptance-owned"
WIF_ACCEPTANCE_OWNED=0

if [[ ! "$PHP_EXTENSIONS" =~ ^[a-z][a-z0-9_-]*(,[a-z][a-z0-9_-]*)*$ ]]; then
	printf 'MAGELIFT_GCP_ACCEPTANCE_EXTENSIONS must be a comma-separated lowercase PHP extension list\n' >&2
	exit 2
fi
case "$MEMORYSTORE_ENGINE_VERSION" in
""|VALKEY_8_0|VALKEY_9_0|VALKEY_9_1) ;;
*)
	printf 'MAGELIFT_GCP_ACCEPTANCE_MEMORYSTORE_ENGINE_VERSION must be VALKEY_8_0, VALKEY_9_0, or VALKEY_9_1\n' >&2
	exit 2
;;
esac
if [[ "$MEMORYSTORE_ENGINE_VERSION" == VALKEY_9_1 && "$PROFILE" != preview ]]; then
	printf 'VALKEY_9_1 is a Preview-only GCP acceptance profile; use MAGELIFT_GCP_ACCEPTANCE_PROFILE=preview\n' >&2
	exit 2
fi
PHP_EXTENSIONS_YAML="${PHP_EXTENSIONS//,/, }"

generated_state_bucket_name() {
	local state_environment="${PROFILE}"
	local base digest prefix_length prefix tail
	if [[ "$PROFILE" == high-availability ]]; then
		state_environment="ha"
	fi
	base="magelift-${PROJECT}-${REGION}-${NAME}-${state_environment}-state"
	if (( ${#base} <= 63 )); then
		printf '%s' "$base"
		return 0
	fi
	if ! digest="$(printf '%s' "$base" | shasum -a 256 | awk '{print substr($1, 1, 10)}')"; then
		printf 'unable to hash generated GCS state bucket name\n' >&2
		return 1
	fi
	tail="${state_environment}-state"
	prefix_length=$((63 - ${#digest} - ${#tail} - 2))
	prefix="${base:0:prefix_length}"
	prefix="${prefix%-}"
	printf '%s-%s-%s' "$prefix" "$digest" "$tail"
}

validate_generated_names() {
	local state_bucket
	if ! state_bucket="$(generated_state_bucket_name)"; then
		return 1
	fi
	if (( ${#CLUSTER_NAME} > 40 )); then
		printf 'generated GKE cluster name is %d characters; GKE allows at most 40: %s\n' \
			"${#CLUSTER_NAME}" "$CLUSTER_NAME" >&2
		return 1
	fi
	if (( ${#WIF_POOL_ID} > 31 )); then
		printf 'generated Workload Identity pool ID is %d characters; GCP allows at most 31: %s\n' \
			"${#WIF_POOL_ID}" "$WIF_POOL_ID" >&2
		return 1
	fi
	local service_account_id="ml-${NAME}-${PROFILE}-ci"
	if (( ${#service_account_id} > 30 )); then
		printf 'generated service account ID is %d characters; GCP allows at most 30: %s\n' \
			"${#service_account_id}" "$service_account_id" >&2
		return 1
	fi
}

if ! validate_generated_names; then
	exit 2
fi

case "$MODE" in
preview|up) ;;
*)
	printf 'usage: %s [preview|up]\n' "$(basename "$0")" >&2
	exit 2
;;
esac
case "$PROFILE" in
preview) ;;
standard|high-availability)
	if [[ "${MAGELIFT_GCP_ACCEPTANCE_ALLOW_COSTLY:-}" != true ]]; then
		printf 'refusing profile %s without MAGELIFT_GCP_ACCEPTANCE_ALLOW_COSTLY=true\n' "$PROFILE" >&2
		exit 2
	fi
	;;
*)
	printf 'unsupported profile %s\n' "$PROFILE" >&2
	exit 2
	;;
	esac

if ! validate_infra_only_catalog; then
	exit 2
fi

mkdir -p "$WORKDIR" "$LOG_DIR"
PULUMI_ACCEPTANCE_PASSPHRASE_OWNED=0
if [[ -z "${PULUMI_CONFIG_PASSPHRASE:-}" && -z "${PULUMI_CONFIG_PASSPHRASE_FILE:-}" ]]; then
	PULUMI_CONFIG_PASSPHRASE_FILE="${WORKDIR}/pulumi-passphrase"
	if [[ ! -s "$PULUMI_CONFIG_PASSPHRASE_FILE" ]]; then
		umask 077
		if ! openssl rand -hex 32 >"$PULUMI_CONFIG_PASSPHRASE_FILE"; then
			printf 'unable to create the temporary Pulumi passphrase file\n' >&2
			exit 2
		fi
	fi
	chmod 600 "$PULUMI_CONFIG_PASSPHRASE_FILE"
	export PULUMI_CONFIG_PASSPHRASE_FILE
	PULUMI_ACCEPTANCE_PASSPHRASE_OWNED=1
	printf '+ using a temporary Pulumi passphrase file (value not logged)\n'
fi
exec > >(tee -a "${LOG_DIR}/acceptance.log") 2>&1
GCP_ACCEPTANCE_RUN_LABEL="$(printf '%s' "$ACCEPTANCE_EVIDENCE_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ORIGIN_INGRESS_NAME="${NAME}-${PROFILE}-edge-ingress"
ORIGIN_CERTIFICATE_NAME="${NAME}-${PROFILE}-edge-certificate"
ORIGIN_DNS_MARKER="magelift/acceptance/gcp-origin-dns/${NAME}-${PROFILE}"

printf 'gcp acceptance start mode=%s profile=%s project=%s region=%s at=%s\n' \
	"$MODE" "$PROFILE" "$PROJECT" "$REGION" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Prefer Application Default Credentials (refreshable). A static
# GOOGLE_OAUTH_ACCESS_TOKEN expires mid-create (~40m) and surfaces as
# ACCESS_TOKEN_TYPE_UNSUPPORTED on GKE/Memorystore operation polls.
ensure_gcp_adc() {
	local adc="${GOOGLE_APPLICATION_CREDENTIALS:-$HOME/.config/gcloud/application_default_credentials.json}"
	local account
	if [[ -f "$adc" ]]; then
		export GOOGLE_APPLICATION_CREDENTIALS="$adc"
		export CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE="$GOOGLE_APPLICATION_CREDENTIALS"
		unset GOOGLE_OAUTH_ACCESS_TOKEN || true
		printf '+ using existing ADC at %s\n' "$adc"
		return 0
	fi
	account="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -n 1 | tr -d '[:space:]')"
	if [[ -z "$account" ]]; then
		printf 'no active gcloud account; run gcloud auth login\n' >&2
		return 1
	fi
	mkdir -p "$(dirname "$adc")"
	if ! python3 - "$account" "$adc" <<'PY'
import json, sqlite3, sys
from pathlib import Path
account, dest = sys.argv[1], Path(sys.argv[2])
db = Path.home() / ".config/gcloud/credentials.db"
con = sqlite3.connect(db)
row = con.execute("select value from credentials where account_id = ?", (account,)).fetchone()
if not row:
    # fall back to any authorized_user row
    row = con.execute("select value from credentials limit 1").fetchone()
if not row:
    raise SystemExit(f"no gcloud credentials for {account}")
data = json.loads(row[0])
if data.get("type") != "authorized_user" or "refresh_token" not in data:
    raise SystemExit("gcloud credentials are not refreshable authorized_user")
adc = {
    "type": "authorized_user",
    "client_id": data["client_id"],
    "client_secret": data["client_secret"],
    "refresh_token": data["refresh_token"],
}
if "universe_domain" in data:
    adc["universe_domain"] = data["universe_domain"]
dest.write_text(json.dumps(adc))
dest.chmod(0o600)
print(account)
PY
	then
		printf 'unable to materialize ADC from gcloud credentials.db\n' >&2
		return 1
	fi
	export GOOGLE_APPLICATION_CREDENTIALS="$adc"
	export CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE="$GOOGLE_APPLICATION_CREDENTIALS"
	unset GOOGLE_OAUTH_ACCESS_TOKEN || true
	printf '+ materialized ADC for %s at %s (GOOGLE_OAUTH_ACCESS_TOKEN unset)\n' "$account" "$adc"
	return 0
}

if ! ensure_gcp_adc; then
	# Last resort: short-lived user access token (will fail long Ups).
	if [[ -z "${GOOGLE_OAUTH_ACCESS_TOKEN:-}" ]]; then
		if ! GOOGLE_OAUTH_ACCESS_TOKEN="$(gcloud auth print-access-token --project="$PROJECT" 2>/dev/null)"; then
			printf 'unable to obtain GCP credentials; run gcloud auth login and preferably application-default login\n' >&2
			exit 2
		fi
		export GOOGLE_OAUTH_ACCESS_TOKEN
		unset CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE || true
		printf 'WARNING: using static GOOGLE_OAUTH_ACCESS_TOKEN; long Ups may fail when it expires\n' >&2
	fi
fi
export CLOUDSDK_CORE_PROJECT="$PROJECT"
export GOOGLE_PROJECT="$PROJECT"

digest_is_placeholder() {
	[[ "$DIGEST" == *sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef* ]]
}

verify_acceptance_digest() {
	if [[ "$INFRA_ONLY" == 1 ]]; then
		printf '+ infra-only acceptance: skipping Magento OCI digest and container contract checks\n'
		return 0
	fi
	if digest_is_placeholder; then
		printf 'MAGELIFT_GCP_ACCEPTANCE_DIGEST must be a pullable OCI digest for live runs\n' >&2
		return 1
	fi
	if [[ "$DIGEST" == *.pkg.dev/*@sha256:* ]] && ! gcloud artifacts docker images describe "$DIGEST" \
		--project="$PROJECT" >/dev/null 2>&1; then
		printf 'GCP Artifact Registry image digest does not exist: %s\n' "$DIGEST" >&2
		return 1
	fi
	if ! command -v docker >/dev/null 2>&1; then
		printf 'Docker is required to validate the Magento runtime image contract before provisioning\n' >&2
		return 1
	fi
	local platform="${MAGELIFT_GCP_ACCEPTANCE_DOCKER_PLATFORM:-linux/amd64}"
	if ! docker run --rm --platform "$platform" --entrypoint /bin/sh "$DIGEST" -c \
		'test -f /app/app/etc/env.php && test -f /app/pub/static/deployed_version.txt && grep -q "MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST" /app/app/etc/env.php && grep -Eq "user.*=>.*MAGENTO_DC_QUEUE__AMQP__USERNAME" /app/app/etc/env.php && ! grep -Eq "username.*=>.*MAGENTO_DC_QUEUE__AMQP__USERNAME" /app/app/etc/env.php && grep -q "MAGENTO_DC_QUEUE__AMQP__PASSWORD" /app/app/etc/env.php && php -i 2>/dev/null | grep -q "auto_prepend_file => /usr/local/lib/magelift/deployment-config.php" && grep -Eq "^[[:space:]]*clear_env[[:space:]]*=[[:space:]]*no[[:space:]]*$" /usr/local/etc/php-fpm.d/zz-magelift.conf && php-fpm --test >/dev/null 2>&1' >/dev/null 2>&1; then
		printf 'Magento image does not contain the MageLift runtime, static-content, and deployment-config contract: %s\n' "$DIGEST" >&2
		return 1
	fi
}

if ! verify_acceptance_digest; then
	exit 2
fi

expected_gcp_database_version() {
	case "$MAGENTO_VERSION" in
	2.4.6|2.4.6-p*)
		printf 'MYSQL_8_0'
		;;
	2.4.7|2.4.7-p*|2.4.8|2.4.8-p*|2.4.9|2.4.9-p*)
		printf 'MYSQL_8_4'
		;;
	*)
		printf 'no GCP Cloud SQL version mapping for Magento %s\n' "$MAGENTO_VERSION" >&2
		return 1
		;;
	esac
}

verify_gcp_database_version() {
	local expected actual instance
	expected="$(expected_gcp_database_version)" || return 1
	instance="${NAME}-${PROFILE}-sql"
	if ! actual="$(gcloud sql instances describe "$instance" --project="$PROJECT" --format='value(databaseVersion)' 2>/dev/null)"; then
		printf 'could not read Cloud SQL database version for %s\n' "$instance" >&2
		return 1
	fi
	if [[ "$actual" != "$expected" ]]; then
		printf 'Cloud SQL database version mismatch for %s: got %s, want %s for Magento %s\n' \
			"$instance" "${actual:-<empty>}" "$expected" "$MAGENTO_VERSION" >&2
		return 1
	fi
	printf '+ Cloud SQL database version verified instance=%s version=%s Magento=%s\n' "$instance" "$actual" "$MAGENTO_VERSION"
}

# MageLift GCP DIY state is GCS. A leftover global `pulumi login` or ambient
# PULUMI_BACKEND_URL can point at a deleted/wrong-region backend and falsely
# block create-once. Only an explicit acceptance override opts out of the
# disposable bucket owned by this harness. Keep this name identical to
# gcp/bootstrap.BuildPlan.
if [[ -n "${MAGELIFT_GCP_ACCEPTANCE_BACKEND_URL:-}" ]]; then
	export PULUMI_BACKEND_URL="$MAGELIFT_GCP_ACCEPTANCE_BACKEND_URL"
	printf '+ using explicitly configured GCP Pulumi backend %s\n' "$PULUMI_BACKEND_URL"
	AUTO_STATE_BUCKET=0
	STATE_BUCKET_NAME=""
else
	if ! STATE_BUCKET_NAME="$(generated_state_bucket_name)"; then
		exit 2
	fi
	export PULUMI_BACKEND_URL="gs://${STATE_BUCKET_NAME}"
	printf '+ defaulting PULUMI_BACKEND_URL=%s\n' "$PULUMI_BACKEND_URL"
	AUTO_STATE_BUCKET=1
fi

# Producer deletes (SQL/Memorystore/GKE/SCP) are async; poll before PSA peering/VPC teardown.
FORCE_CLEAN_POLL_INTERVAL_SECS="${MAGELIFT_GCP_FORCE_CLEAN_POLL_INTERVAL_SECS:-30}"
FORCE_CLEAN_TIMEOUT_SECS="${MAGELIFT_GCP_FORCE_CLEAN_TIMEOUT_SECS:-1200}"
PSA_NUDGE_DELAY_SECS="${MAGELIFT_GCP_PSA_NUDGE_DELAY_SECS:-5}"
PSA_NUDGE_TIMEOUT_SECS="${MAGELIFT_GCP_PSA_NUDGE_TIMEOUT_SECS:-1200}"

refresh_gcp_access_token() {
	if GOOGLE_OAUTH_ACCESS_TOKEN="$(gcloud auth print-access-token --project="$PROJECT" 2>/dev/null)"; then
		export GOOGLE_OAUTH_ACCESS_TOKEN
		printf '+ refreshed GOOGLE_OAUTH_ACCESS_TOKEN\n'
		return 0
	fi
	printf 'unable to refresh GCP access token\n' >&2
	return 1
}

APIS=(
	compute.googleapis.com
	container.googleapis.com
	cloudbilling.googleapis.com
	sqladmin.googleapis.com
	secretmanager.googleapis.com
	servicenetworking.googleapis.com
	networkconnectivity.googleapis.com
	serviceconsumermanagement.googleapis.com
	memorystore.googleapis.com
)

ensure_acceptance_state_bucket() {
	if [[ "$AUTO_STATE_BUCKET" != 1 ]]; then
		return 0
	fi
	if gcloud storage buckets describe "gs://${STATE_BUCKET_NAME}" --project="$PROJECT" >/dev/null 2>&1; then
		printf '+ using existing acceptance Pulumi state bucket gs://%s\n' "$STATE_BUCKET_NAME"
		return 0
	fi
	printf '+ creating acceptance Pulumi state bucket gs://%s\n' "$STATE_BUCKET_NAME"
	gcloud storage buckets create "gs://${STATE_BUCKET_NAME}" \
		--project="$PROJECT" \
		--location="$REGION" \
		--uniform-bucket-level-access >/dev/null
}

if [[ ! -x "$BIN" || "${MAGELIFT_GCP_ACCEPTANCE_REBUILD:-false}" == true ]]; then
	printf '+ building GCP acceptance CLI -> %s GOMEMLIMIT=%s\n' "$BIN" "${GOMEMLIMIT:-}"
	(cd "$ROOT" && go build -trimpath -ldflags='-s -w' -o "$BIN" ./cmd/magelift)
else
	printf '+ using existing GCP acceptance CLI -> %s\n' "$BIN"
fi

acceptance_prepare_lifecycle
EXPIRES="$ACCEPTANCE_EXPIRES_AT"
gcp_acceptance_resolve_zones
ORIGIN_ENV_DOMAIN_YAML=""
if [[ "$NATIVE_EDGE_ENABLED" == 1 ]]; then
	ORIGIN_ENV_DOMAIN_YAML="    domain: ${ORIGIN_DOMAIN}"
fi
cat >"$CONFIG" <<EOF
schemaVersion: 1
project:
  name: ${NAME}
application:
  edition: open-source
  version: "${MAGENTO_VERSION}"
  mode: integrated
  webRuntime: nginx-fpm
build:
  php: "${PHP_VERSION}"
  extensions: [${PHP_EXTENSIONS_YAML}]
  staticContent:
    locales: [en_US]
    themes: [Magento/blank, Magento/luma]
  composer:
    version: "${COMPOSER_VERSION}"
    credentials: ${COMPOSER_SM_URI}
target:
  provider: gcp
  runtime: ${GKE_RUNTIME}
  gcp:
    project: ${PROJECT}
    region: ${REGION}
    networkCidr: 10.40.0.0/16
    zones: [${GCP_ZONES}]
    imageDigest: ${DIGEST}
    databaseName: magento
    masterUsername: magento
    encryptionKeySecret: ${ENCRYPTION_KEY_SECRET_ID}
    memorystoreEngineVersion: ${MEMORYSTORE_ENGINE_VERSION:-null}
    openSearchImage: ${OPENSEARCH_IMAGE}
    rabbitMqImage: ${RABBITMQ_IMAGE}
    enableCloudArmor: ${ENABLE_CLOUD_ARMOR}
    autopilotMemoryRequest: ${AUTOPILOT_MEMORY_REQUEST}
    labels:
      magelift-project: acceptance-gcp
      magelift-environment: ${PROFILE}
      magelift-acceptance-run: ${GCP_ACCEPTANCE_RUN_LABEL}
defaults:
  region: ${REGION}
  preset: ${PROFILE}
environments:
  ${PROFILE}:
    account: "${PROJECT}"
    class: preview
    expiresAt: "${EXPIRES}"
    monthlyBudgetCents: 50000
    seedDump: ${SEED_DUMP}
${ORIGIN_ENV_DOMAIN_YAML}
extensions: {}
EOF

if [[ "$NATIVE_EDGE_ENABLED" == 1 ]]; then
	cat >>"$CONFIG" <<EOF
edge:
  mode: native
  nativeProvider: cloud-cdn
  domains: [${ORIGIN_DOMAIN}]
  tls: true
  tlsMode: gke-managed
  dnsMode: customer-managed
  purgeOnDeploy: true
  originHealthRef: health/magento
  ownershipMarker: magelift/acceptance/gcp-native-edge/${NAME}-${PROFILE}
EOF
fi

# Reusing a retained stack is safe only when the checkpoint belongs to this
# exact release, runtime, artifact, and cell catalog. This lets one expensive
# GKE/Cloud SQL/Valkey stack serve several warm application runs without
# allowing an old PASS to skip a materially different certification cell.
GCP_FINGERPRINT_EDITION="${MAGELIFT_GCP_ACCEPTANCE_EDITION:-open-source}"
GCP_FINGERPRINT_DATABASE=cloud-sql-mysql
GCP_FINGERPRINT_SEARCH=opensearch
GCP_FINGERPRINT_CACHE=valkey
GCP_FINGERPRINT_WEB_CACHE=none
GCP_FINGERPRINT_EDGE=none
if [[ "$NATIVE_EDGE_ENABLED" == 1 ]]; then
	GCP_FINGERPRINT_EDGE="gke-cloud-cdn:${ORIGIN_DOMAIN}:cloud-armor"
fi
if [[ "$PROFILE" == preview ]]; then
	GCP_FINGERPRINT_QUEUE=database
else
	GCP_FINGERPRINT_QUEUE=rabbitmq
fi
ACCEPTANCE_CHECKPOINT_FINGERPRINT="$({
	printf 'provider=gcp\nproject=%s\nregion=%s\nname=%s\nprofile=%s\nruntime=%s\n' \
		"$PROJECT" "$REGION" "$NAME" "$PROFILE" "$GKE_RUNTIME"
	printf 'release=%s\nphp=%s\ncomposer=%s\ndigest=%s\ninfraOnly=%s\nedition=%s\nextensions=%s\ndatabase=%s\nsearch=%s\nqueue=%s\ncache=%s\ncacheVersion=%s\nwebCache=%s\nedge=%s\nopenSearch=%s\nrabbitMQ=%s\ncloudArmor=%s\nmemory=%s\nseed=%s\nencryption=%s\nzones=%s\n' \
		"$MAGENTO_VERSION" "$PHP_VERSION" "$COMPOSER_VERSION" "$DIGEST" \
		"$INFRA_ONLY" \
		"$GCP_FINGERPRINT_EDITION" "$PHP_EXTENSIONS" "$GCP_FINGERPRINT_DATABASE" "$GCP_FINGERPRINT_SEARCH" \
		"$GCP_FINGERPRINT_QUEUE" "$GCP_FINGERPRINT_CACHE" "$MEMORYSTORE_ENGINE_VERSION" "$GCP_FINGERPRINT_WEB_CACHE" "$GCP_FINGERPRINT_EDGE" \
		"$OPENSEARCH_IMAGE" "$RABBITMQ_IMAGE" "$ENABLE_CLOUD_ARMOR" "$AUTOPILOT_MEMORY_REQUEST" "$SEED_DUMP" "$ENCRYPTION_KEY_SECRET_ID"
	printf 'nativeEdge=%s\noriginDomain=%s\noriginTitle=%s\n' "$NATIVE_EDGE_ENABLED" "$ORIGIN_DOMAIN" "$ORIGIN_TITLE"
	printf '\ncell-catalog:\n'
	cat "$CELL_CATALOG"
} | shasum -a 256 | awk '{print $1}')"
export ACCEPTANCE_CHECKPOINT_FINGERPRINT
export MAGELIFT_ACCEPTANCE_RELEASE="$MAGENTO_VERSION"
export MAGELIFT_ACCEPTANCE_EDITION="$GCP_FINGERPRINT_EDITION"
export MAGELIFT_ACCEPTANCE_RUNTIME="$GKE_RUNTIME"
export MAGELIFT_ACCEPTANCE_PRESET="$PROFILE"
export MAGELIFT_ACCEPTANCE_DIGEST="$DIGEST"
export MAGELIFT_ACCEPTANCE_PHP_VERSION="$PHP_VERSION"
export MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS="$PHP_EXTENSIONS"
export MAGELIFT_ACCEPTANCE_COMPOSER_VERSION="$COMPOSER_VERSION"
export MAGELIFT_ACCEPTANCE_DATABASE="$GCP_FINGERPRINT_DATABASE"
export MAGELIFT_ACCEPTANCE_SEARCH="$GCP_FINGERPRINT_SEARCH"
export MAGELIFT_ACCEPTANCE_QUEUE="$GCP_FINGERPRINT_QUEUE"
export MAGELIFT_ACCEPTANCE_CACHE="$GCP_FINGERPRINT_CACHE"
export MAGELIFT_ACCEPTANCE_WEB_CACHE="$GCP_FINGERPRINT_WEB_CACHE"
export MAGELIFT_ACCEPTANCE_EDGE="$GCP_FINGERPRINT_EDGE"
export MAGELIFT_ACCEPTANCE_STACK_ID="${PULUMI_PROJECT}/${STACK_NAME}"
export MAGELIFT_ACCEPTANCE_COMPUTE_MODE="$(acceptance_shared_compute_mode "$GKE_RUNTIME")"
export MAGELIFT_ACCEPTANCE_KUBERNETES_MODE="$(acceptance_shared_kubernetes_mode "$GKE_RUNTIME")"
gcp_seed_fixture="$(acceptance_seed_fixture_id "$SEED_DUMP")"
acceptance_export_reuse_boundary \
	"$gcp_seed_fixture" \
	"gcp-cloudsql-automated:${NAME}-${PROFILE}-sql" \
	"gke-native-workloads" \
	"$GCP_FINGERPRINT_EDGE" \
	"$DIGEST" \
	"$gcp_seed_fixture" \
	"${PULUMI_BACKEND_URL:?PULUMI_BACKEND_URL is required for reuse-boundary evidence}"

run() {
	printf '+ magelift %s\n' "$*"
	"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json "$@"
}

# Magelift Cloud SQL instance name matches naming.CloudSQLInstance(project, environment).
gcp_cloudsql_instance_name() {
	printf '%s-%s-sql' "$NAME" "$PROFILE"
}

gcp_destroy_backups_requested() {
	case "${MAGELIFT_GCP_DESTROY_BACKUPS:-1}" in
	0|false|no) return 1 ;;
	*) return 0 ;;
	esac
}

run_magelift_destroy() {
	if gcp_destroy_backups_requested; then
		run destroy --yes --destroy-backups
	else
		run destroy --yes
	fi
}

gcp_cloudsql_leftover_backup_names() {
	local instance json
	instance="$(gcp_cloudsql_instance_name)"
	if [[ -z "$instance" || "$instance" == *[/\*\?]* ]]; then
		printf 'Cloud SQL leftover backup inventory requires a single Magelift instance name\n' >&2
		return 2
	fi
	json="$(gcloud sql backups list --project="$PROJECT" --format=json)" || return 1
	jq -r --arg instance "$instance" '.[] | select((.instance // "") == $instance or ((.instance // "") | endswith("/instances/" + $instance))) | (.name // empty)' <<<"$json"
}

assert_no_leftover_cloudsql_backups() {
	local leftovers leftover_count
	if ! gcp_destroy_backups_requested; then
		return 0
	fi
	leftovers="$(gcp_cloudsql_leftover_backup_names)" || return 1
	leftover_count="$(printf '%s\n' "$leftovers" | awk 'NF' | wc -l)"
	leftover_count="${leftover_count//[[:space:]]/}"
	if [[ "${leftover_count:-0}" -gt 0 ]]; then
		printf 'leftover Cloud SQL backups for %s:\n%s\n' "$(gcp_cloudsql_instance_name)" "$leftovers" >&2
		return 1
	fi
	printf '+ leftover Cloud SQL backups remaining=0 instance=%s\n' "$(gcp_cloudsql_instance_name)"
	return 0
}

cleanup_leftover_cloudsql_backups() {
	local backup leftovers leftover_count start elapsed timeout_secs
	if ! gcp_destroy_backups_requested; then
		return 0
	fi
	timeout_secs="${MAGELIFT_GCP_BACKUP_CLEANUP_TIMEOUT_SECS:-180}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ ]]; then
		printf 'Cloud SQL leftover backup cleanup timeout must be a positive integer\n' >&2
		return 2
	fi
	leftovers="$(gcp_cloudsql_leftover_backup_names)" || return 1
	while IFS= read -r backup; do
		[[ -z "$backup" ]] && continue
		printf '+ deleting leftover Cloud SQL backup %s\n' "$backup"
		gcloud sql backups delete "$backup" --project="$PROJECT" --quiet >/dev/null || true
	done <<<"$leftovers"
	start="$(date +%s)"
	while true; do
		leftovers="$(gcp_cloudsql_leftover_backup_names)" || return 1
		leftover_count="$(printf '%s\n' "$leftovers" | awk 'NF' | wc -l)"
		leftover_count="${leftover_count//[[:space:]]/}"
		if [[ "${leftover_count:-0}" -eq 0 ]]; then
			printf '+ leftover Cloud SQL backups remaining=0 instance=%s\n' "$(gcp_cloudsql_instance_name)"
			return 0
		fi
		elapsed=$(( $(date +%s) - start ))
		if [[ "$elapsed" -ge "$timeout_secs" ]]; then
			printf 'leftover Cloud SQL backups still present after %ss for %s:\n%s\n' "$elapsed" "$(gcp_cloudsql_instance_name)" "$leftovers" >&2
			return 1
		fi
		printf '+ waiting for leftover Cloud SQL backup deletes elapsed=%ds remaining=%s\n' "$elapsed" "$leftover_count"
		sleep 5
	done
}

run_runtime_health_with_retry() {
	local log_path="${1:?runtime health log path required}"
	local timeout="${MAGELIFT_GCP_RUNTIME_HEALTH_TIMEOUT_SECS:-600}"
	local interval="${MAGELIFT_GCP_RUNTIME_HEALTH_INTERVAL_SECS:-15}"
	local started now
	: >"$log_path"
	started=$(date +%s)
	while true; do
		if run health --mode runtime 2>&1 | tee -a "$log_path"; then
			return 0
		fi
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf 'runtime health did not become ready after %ss\n' "$timeout" >&2
			return 1
		fi
		printf '+ runtime health is not ready; retrying in %ss\n' "$interval" >&2
		sleep "$interval"
	done
}

# day2:health runs before migrate/deploy:candidate. Infra-only Magento returns
# HTTP 500 on the LoadBalancer, which must not fail the kube replica check.
# Magento HTTP 200 is required after deploy:candidate.
gcp_kube_deployment_healthy() {
	local log_path="${1:?runtime health log path required}"
	python3 - "$log_path" <<'PY'
import json
import sys

text = open(sys.argv[1], encoding="utf-8", errors="replace").read()
decoder = json.JSONDecoder()
objs = []
i = 0
while i < len(text):
    start = text.find("{", i)
    if start < 0:
        break
    try:
        obj, end = decoder.raw_decode(text, start)
    except json.JSONDecodeError:
        i = start + 1
        continue
    if isinstance(obj, dict) and isinstance(obj.get("checks"), list):
        objs.append(obj)
    i = end
if not objs:
    raise SystemExit(1)
for check in objs[-1]["checks"]:
    if check.get("id") == "runtime.kube.deployment" and check.get("status") == "healthy":
        raise SystemExit(0)
raise SystemExit(1)
PY
}

run_kube_runtime_health_with_retry() {
	local log_path="${1:?runtime health log path required}"
	local timeout="${MAGELIFT_GCP_RUNTIME_HEALTH_TIMEOUT_SECS:-600}"
	local interval="${MAGELIFT_GCP_RUNTIME_HEALTH_INTERVAL_SECS:-15}"
	local started now
	: >"$log_path"
	started=$(date +%s)
	while true; do
		run health --mode runtime 2>&1 | tee -a "$log_path" || true
		if gcp_kube_deployment_healthy "$log_path"; then
			printf '+ kube deployment healthy; Magento HTTP waits for deploy:candidate\n'
			return 0
		fi
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf 'kube runtime health did not become ready after %ss\n' "$timeout" >&2
			return 1
		fi
		printf '+ kube runtime health is not ready; retrying in %ss\n' "$interval" >&2
		sleep "$interval"
	done
}

verify_gcp_magento_version() {
	local log_path="${1:?Magento version log path required}"
	if ! run exec --service web -- bin/magento --no-ansi --version 2>&1 | tee "$log_path"; then
		printf 'Magento CLI version check failed\n' >&2
		return 1
	fi
	if ! grep -Fq -- "$MAGENTO_VERSION" "$log_path"; then
		printf 'Magento CLI version mismatch: expected %s\n' "$MAGENTO_VERSION" >&2
		return 1
	fi
	printf '+ Magento CLI version verified release=%s\n' "$MAGENTO_VERSION"
}

# Plant magelift_seed_probe after dump import. Live installed-schema dumps have
# catalog schema but no products and no seed-probe table; tiny.sql already has
# the table (idempotent). Password travels as a base64 first stdin line, matching
# dumpimport's kube runner (never -p on argv).
gcp_apply_magento_seed_probe() {
	local kc_path="${1:?kubeconfig required}"
	local dump_pod="${2:?mysql client pod required}"
	local db_host="${3:?database host required}"
	local db_pass="${4:?database password required}"
	local sql_path="${ROOT}/testdata/fixtures/migrate/magelift-seed-probe.sql"
	local b64 observed
	if [[ ! -r "${sql_path}" ]]; then
		printf 'Magento seed probe SQL is missing: %s\n' "${sql_path}" >&2
		return 1
	fi
	b64="$(printf '%s' "${db_pass}" | python3 -c 'import base64,sys; print(base64.b64encode(sys.stdin.buffer.read()).decode())')"
	if ! {
		printf '%s\n' "${b64}"
		cat "${sql_path}"
	} | kubectl --kubeconfig="${kc_path}" exec -i "${dump_pod}" -- sh -c 'read -r _ml_b64
MYSQL_PWD=$(printf "%s" "$_ml_b64" | base64 -d)
export MYSQL_PWD
unset _ml_b64
exec mysql -h "$1" -P 3306 -u magento magento' sh "${db_host}" \
		>"${LOG_DIR}/cell-migrate-seed-probe.apply.log" 2>&1; then
		printf 'Magento seed probe SQL apply failed\n' >&2
		return 1
	fi
	if ! observed="$(
		{
			printf '%s\n' "${b64}"
		} | kubectl --kubeconfig="${kc_path}" exec -i "${dump_pod}" -- sh -c 'read -r _ml_b64
MYSQL_PWD=$(printf "%s" "$_ml_b64" | base64 -d)
export MYSQL_PWD
unset _ml_b64
exec mysql -N -h "$1" -P 3306 -u magento magento -e "SELECT label FROM magelift_seed_probe WHERE id = 1"' sh "${db_host}"
	)"; then
		printf 'Magento seed probe SQL verify failed\n' >&2
		return 1
	fi
	observed="$(printf '%s' "${observed}" | awk 'NF && $0 !~ /^[+{]/ && $0 !~ /^warning:/ && $0 !~ /^Defaulted / { print; exit }')"
	if [[ "${observed}" != tiny-fixture ]]; then
		printf 'Magento seed probe label mismatch after apply\n' >&2
		return 1
	fi
	printf '+ Magento seed probe planted label=tiny-fixture\n'
}

# Fetch one Magento DB scalar through Magento's resource layer. Keep the PHP
# program on one physical line because the exec transport rejects command
# arguments containing literal newlines. entity is an allowlisted kind.
gcp_magento_fetch_one() {
	local entity="${1:?Magento fetch entity required}"
	local out_log="${2:?Magento fetch log required}"
	local table column order_column empty_msg
	case "${entity}" in
	seed)
		table="magelift_seed_probe"
		column="label"
		order_column="id"
		empty_msg="Magento magelift_seed_probe has no label"
		;;
	sku)
		table="catalog_product_entity"
		column="sku"
		order_column="entity_id"
		empty_msg="Magento catalog_product_entity has no sku"
		;;
	*)
		printf 'Magento fetch entity must be seed or sku\n' >&2
		return 2
		;;
	esac
	run exec --service web -- php -r '$database=["host"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST"),"dbname"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME"),"username"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME"),"password"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD")]; foreach ($database as $key=>$value) { if ($value === "") { fwrite(STDERR, "Magento database environment is incomplete: ".$key."\n"); exit(1); } } $encoded=json_encode(["db"=>["connection"=>["default"=>$database]]], JSON_UNESCAPED_SLASHES); if (!is_string($encoded) || putenv("MAGENTO_DC__OVERRIDE=".$encoded) === false) { fwrite(STDERR, "could not set Magento deployment-config override\n"); exit(1); } require "/app/app/bootstrap.php"; $bootstrap=\Magento\Framework\App\Bootstrap::create(BP, $_SERVER); $objectManager=$bootstrap->getObjectManager(); $resource=$objectManager->get(\Magento\Framework\App\ResourceConnection::class); $connection=$resource->getConnection(); $table=$resource->getTableName("'"${table}"'"); $value=(string) $connection->fetchOne("SELECT ".$connection->quoteIdentifier("'"${column}"'")." FROM ".$connection->quoteIdentifier($table)." ORDER BY ".$connection->quoteIdentifier("'"${order_column}"'")." ASC LIMIT 1"); if ($value === "") { fwrite(STDERR, "'"${empty_msg}"'\n"); exit(1); } fwrite(STDOUT, $value."\n");' 2>&1 | tee "${out_log}"
}

# Optional HA application-integrity probe. URL and HTTP expect must be set
# together. MAGELIFT_HA_MAGENTO_CONTENT_SKU reads catalog_product_entity;
# live installed-schema dumps have that table but no product rows, so do not
# default a sample-data SKU. migrate:dump plants magelift_seed_probe.label=
# tiny-fixture, then HA cells run magento-seed-probe unless
# MAGELIFT_HA_MAGENTO_SEED_PROBE=0.
run_gcp_magento_known_content_check() {
	local log_path="${1:?Magento known-content log path required}"
	local url="${MAGELIFT_HA_MAGENTO_CONTENT_URL:-}"
	local expect="${MAGELIFT_HA_MAGENTO_CONTENT_EXPECT:-}"
	local sku="${MAGELIFT_HA_MAGENTO_CONTENT_SKU:-}"
	local seed_expect="${MAGELIFT_HA_MAGENTO_SEED_EXPECT:-}"
	local seed_probe="${MAGELIFT_HA_MAGENTO_SEED_PROBE:-1}"
	local observed
	case "${seed_probe}" in
	0|false|no)
		seed_probe=0
		;;
	1|true|yes|"")
		seed_probe=1
		;;
	*)
		printf 'MAGELIFT_HA_MAGENTO_SEED_PROBE must be 1/true or 0/false\n' >&2
		return 2
		;;
	esac
	if [[ "${seed_probe}" == 1 && -z "${seed_expect}" ]]; then
		seed_expect="tiny-fixture"
	fi
	if [[ -z "${url}" && -z "${expect}" && -z "${sku}" && "${seed_probe}" == 0 ]]; then
		printf 'known-content=not-run\n' | tee "${log_path}"
		return 0
	fi
	if [[ -z "${url}" && -n "${expect}" ]] || [[ -n "${url}" && -z "${expect}" ]]; then
		printf 'Magento known-content HTTP check requires both MAGELIFT_HA_MAGENTO_CONTENT_URL and MAGELIFT_HA_MAGENTO_CONTENT_EXPECT\n' >&2
		return 2
	fi
	: >"${log_path}"
	if [[ "${seed_probe}" == 1 ]]; then
		if ! gcp_magento_fetch_one seed "${LOG_DIR}/cell-magento-seed-probe.exec.log"; then
			printf 'Magento seed probe exec failed\n' >&2
			return 1
		fi
		observed="$(awk 'NF && $0 !~ /^[+{]/ && $0 !~ /^warning:/ && $0 !~ /^Defaulted / { print; exit }' "${LOG_DIR}/cell-magento-seed-probe.exec.log")"
		if ! (cd "${ROOT}" && go run ./cmd/magento-seed-probe --observed="${observed}" --expect="${seed_expect}") | tee -a "${log_path}"; then
			return 1
		fi
	fi
	if [[ -n "${sku}" ]]; then
		if ! gcp_magento_fetch_one sku "${LOG_DIR}/cell-magento-catalog-sku.exec.log"; then
			printf 'Magento catalog SKU exec failed\n' >&2
			return 1
		fi
		observed="$(awk 'NF && $0 !~ /^[+{]/ && $0 !~ /^warning:/ && $0 !~ /^Defaulted / { print; exit }' "${LOG_DIR}/cell-magento-catalog-sku.exec.log")"
		if ! (cd "${ROOT}" && go run ./cmd/magento-catalog-sku-probe --observed="${observed}" --expect="${sku}") | tee -a "${log_path}"; then
			return 1
		fi
	fi
	if [[ -n "${url}" ]]; then
		if ! (cd "${ROOT}" && go run ./cmd/magento-known-content-probe --url="${url}" --expect="${expect}") | tee -a "${log_path}"; then
			return 1
		fi
	fi
}

# Optional failed-deployment drill. Unset keeps the operator-assisted overlay
# (failed-deployment=not-run). When MAGELIFT_GCP_FAILED_DEPLOY_DIGEST is set,
# patch the live web Deployment to that digest (kube Ready must fail), then
# restore MAGELIFT_GCP_ACCEPTANCE_DIGEST via magelift deploy. Do not use
# magelift deploy for the fault digest: kube.Steps runs migrate Jobs before
# UpdateServices, so a crash-loop image never reaches the web pods.
# Do not mark 3.9 complete from not-run. Do not rewrite InjectFailedDeployment.
run_gcp_failed_deployment_check() {
	local log_path="${1:?failed-deployment log path required}"
	local fault="${MAGELIFT_GCP_FAILED_DEPLOY_DIGEST:-}"
	local restored=0 web kc
	if [[ -z "${fault}" ]]; then
		printf 'failed-deployment=not-run\n' | tee "${log_path}"
		return 0
	fi
	if digest_is_placeholder; then
		printf 'failed-deployment requires a pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST\n' >&2
		return 2
	fi
	if ! (cd "${ROOT}" && go run ./cmd/failed-deployment-probe --current="${DIGEST}" --fault="${fault}") | tee "${log_path}"; then
		return 2
	fi
	web="${NAME}-${PROFILE}-app-web"
	if [[ -z "${MAGELIFT_KUBECONFIG:-}" || ! -f "${MAGELIFT_KUBECONFIG:-}" ]]; then
		gcp_prepare_live_kubeconfig || true
	fi
	kc="${MAGELIFT_KUBECONFIG:-}"
	if [[ -z "$kc" || ! -f "$kc" ]]; then
		printf 'failed-deployment needs MAGELIFT_KUBECONFIG from gcloud get-credentials\n' >&2
		return 2
	fi
	# RollingUpdate keeps the previous Ready Magento pod while pause CrashLoops,
	# so kube health stays 1/1. Recreate + delete forces Ready to drop.
	if ! kubectl --kubeconfig="$kc" patch "deployment/${web}" --type=json \
		-p '[{"op":"replace","path":"/spec/strategy","value":{"type":"Recreate"}}]' \
		2>&1 | tee "${LOG_DIR}/cell-failed-deployment-inject.json"; then
		printf 'failed-deployment kubectl patch Recreate failed\n' >&2
		return 1
	fi
	if ! kubectl --kubeconfig="$kc" set image "deployment/${web}" "php-fpm=${fault}" "web=${fault}" 2>&1 | tee -a "${LOG_DIR}/cell-failed-deployment-inject.json"; then
		printf 'failed-deployment kubectl set-image failed\n' >&2
		return 1
	fi
	kubectl --kubeconfig="$kc" delete pod -l "app=${web}" --wait=false >/dev/null 2>&1 || true
	local waited=0
	while (( waited < 180 )); do
		if ! run health --mode runtime >/dev/null 2>&1; then
			printf 'failed-deployment injected and unhealthy\n' | tee -a "${log_path}"
			break
		fi
		sleep 10
		waited=$((waited + 10))
	done
	if run health --mode runtime >/dev/null 2>&1; then
		printf 'failed-deployment apply failed but runtime is healthy\n' >&2
		kubectl --kubeconfig="$kc" set image "deployment/${web}" "php-fpm=${DIGEST}" "web=${DIGEST}" >/dev/null 2>&1 || true
		if "$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json \
			deploy --yes --digest "$DIGEST" 2>&1 | tee "${LOG_DIR}/cell-failed-deployment-restore.json"; then
			run_runtime_health_with_retry "${LOG_DIR}/cell-failed-deployment-restore-health.json" || true
		fi
		return 1
	fi
	if ! kubectl --kubeconfig="$kc" set image "deployment/${web}" "php-fpm=${DIGEST}" "web=${DIGEST}" 2>&1 | tee "${LOG_DIR}/cell-failed-deployment-restore.json"; then
		printf 'failed-deployment kubectl restore set-image failed\n' >&2
		return 1
	fi
	kubectl --kubeconfig="$kc" delete pod -l "app=${web}" --wait=false >/dev/null 2>&1 || true
	if ! run_runtime_health_with_retry "${LOG_DIR}/cell-failed-deployment-restore-health.json"; then
		printf 'failed-deployment restore health failed\n' >&2
		return 1
	fi
	restored=1
	printf 'failed-deployment PASS restored=%s\n' "$restored" | tee -a "${log_path}"
}

request_psa_peering_delete() {
	local network="$1" token
	token="$(gcloud auth print-access-token --project="$PROJECT" 2>/dev/null || true)"
	if [[ -n "$token" ]]; then
		curl -sS -X POST \
			-H "Authorization: Bearer ${token}" \
			-H 'Content-Type: application/json' \
			"https://compute.googleapis.com/compute/v1/projects/${PROJECT}/global/networks/${network}/removePeering" \
			-d '{"name":"servicenetworking-googleapis-com"}' >/dev/null || true
	fi
	gcloud services vpc-peerings delete \
		--network="$network" \
		--service=servicenetworking.googleapis.com \
		--project="$PROJECT" --quiet --async 2>/dev/null || true
}

destroy_with_psa_nudge() {
	# Keep Pulumi's dependency order intact. Cloud SQL users and databases must
	# be removed before the instance, and the Kubernetes LoadBalancer Service
	# must be removed before GKE. If the provider races the final PSA connection,
	# issue only the exact producer deletes and retry the same Pulumi destroy
	# after the producers are gone. force_clean_orphans remains the final
	# authoritative cleanup path when this bounded retry cannot converge.
	local net="${NAME}-${PROFILE}-net"
	if run_magelift_destroy; then
		assert_no_leftover_cloudsql_backups || return 1
		return 0
	fi

	printf 'destroy encountered a transient provider dependency; waiting for exact producer deletes before retrying\n' >&2
	issue_producer_deletes
	if ! wait_for_producer_deletes; then
		return 1
	fi
	sleep "$PSA_NUDGE_DELAY_SECS"
	request_psa_peering_delete "$net"
	# Once every producer is gone, another full Pulumi destroy only replays the
	# stale Service Networking dependency and can spend many minutes producing
	# the same error. Return to the exact orphan cleanup path, which owns the
	# remaining network, PSA address, and state reconciliation.
	if ! producer_resources_remain; then
		printf 'destroy producers are clear; handing off to exact orphan cleanup instead of replaying Pulumi destroy\n' >&2
		return 1
	fi

	local started now
	started="$(date +%s)"
	while true; do
		if run_magelift_destroy; then
			assert_no_leftover_cloudsql_backups || return 1
			return 0
		fi
		now="$(date +%s)"
		if (( now - started >= PSA_NUDGE_TIMEOUT_SECS )); then
			printf 'destroy PSA retry timed out after %ss\n' "$PSA_NUDGE_TIMEOUT_SECS" >&2
			return 1
		fi
		request_psa_peering_delete "$net"
		sleep "$FORCE_CLEAN_POLL_INTERVAL_SECS"
	done
}

run_gcp_exec_cell() {
	local attempt log rc tee_rc
	local -a pipe_status
	local max_attempts="${MAGELIFT_GCP_EXEC_RETRY_ATTEMPTS:-10}"
	local retry_interval="${MAGELIFT_GCP_EXEC_RETRY_INTERVAL_SECS:-30}"
	case "$max_attempts" in
	''|*[!0-9]*|0)
		printf 'MAGELIFT_GCP_EXEC_RETRY_ATTEMPTS must be a positive integer\n' >&2
		return 2
		;;
	esac
	case "$retry_interval" in
	''|*[!0-9]*)
		printf 'MAGELIFT_GCP_EXEC_RETRY_INTERVAL_SECS must be a non-negative integer\n' >&2
		return 2
		;;
	esac
	for ((attempt = 1; attempt <= max_attempts; attempt++)); do
		log="${LOG_DIR}/cell-day2-exec-attempt-${attempt}.log"
		printf '+ day2:exec attempt=%d/%d\n' "$attempt" "$max_attempts"
		if run exec --service web -- true 2>&1 | tee "$log"; then
			return 0
		else
			pipe_status=("${PIPESTATUS[@]}")
			rc="${pipe_status[0]}"
			tee_rc="${pipe_status[1]}"
		fi
		if [[ "$rc" == 0 && "$tee_rc" != 0 ]]; then
			return "$tee_rc"
		fi
		if ! grep -Fq 'No agent available' "$log"; then
			return "$rc"
		fi
		if [[ "$attempt" == "$max_attempts" ]]; then
			return "$rc"
		fi
		printf '+ day2:exec Konnectivity agent unavailable; retrying in %ss\n' "$retry_interval" >&2
		sleep "$retry_interval"
	done
}

capture_acceptance_wif_ownership() {
	local existing=0 pool_state
	pool_state="$(gcloud iam workload-identity-pools describe "$WIF_POOL_ID" \
		--location=global --project="$PROJECT" --format='value(state)' 2>/dev/null || true)"
	if [[ -n "$pool_state" && "$pool_state" != "DELETED" ]]; then
		existing=1
	fi
	if gcloud iam workload-identity-pools providers describe "$WIF_PROVIDER_ID" \
		--workload-identity-pool="$WIF_POOL_ID" --location=global --project="$PROJECT" \
		>/dev/null 2>&1; then
		existing=1
	fi
	if gcloud iam service-accounts describe "$WIF_SERVICE_ACCOUNT_EMAIL" \
		--project="$PROJECT" >/dev/null 2>&1; then
		existing=1
	fi
	if [[ "$existing" == 1 ]]; then
		printf '+ WIF resources already exist; acceptance cleanup will preserve them\n'
		return 0
	fi
	if ! printf 'project=%s\nname=%s\nprofile=%s\n' \
		"$PROJECT" "$NAME" "$PROFILE" >"$WIF_OWNERSHIP_MARKER_FILE"; then
		printf 'unable to persist acceptance WIF ownership marker: %s\n' "$WIF_OWNERSHIP_MARKER_FILE" >&2
		return 1
	fi
	WIF_ACCEPTANCE_OWNED=1
	printf '+ WIF resources absent before bootstrap; acceptance cleanup owns this identity\n'
}

restore_acceptance_wif_ownership() {
	if [[ -s "$WIF_OWNERSHIP_MARKER_FILE" ]]; then
		WIF_ACCEPTANCE_OWNED=1
		printf '+ restored acceptance WIF ownership from %s\n' "$WIF_OWNERSHIP_MARKER_FILE"
	fi
}

assert_wif_pool_name_fresh() {
	local pool_state
	pool_state="$(gcloud iam workload-identity-pools describe "$WIF_POOL_ID" \
		--location=global --project="$PROJECT" --format='value(state)' 2>/dev/null || true)"
	if [[ "$pool_state" == "DELETED" ]]; then
		printf 'WIF pool ID %s is tombstoned after deletion; choose a fresh MAGELIFT_GCP_ACCEPTANCE_NAME before provisioning\n' "$WIF_POOL_ID" >&2
		return 1
	fi
}

delete_acceptance_wif_resource() {
	local kind="$1"
	case "$kind" in
	pool)
		local pool_state
		pool_state="$(gcloud iam workload-identity-pools describe "$WIF_POOL_ID" \
			--location=global --project="$PROJECT" --format='value(state)' 2>/dev/null || true)"
		if [[ -n "$pool_state" && "$pool_state" != "DELETED" ]]; then
			gcloud iam workload-identity-pools delete "$WIF_POOL_ID" \
				--location=global --project="$PROJECT" --quiet
		fi
		;;
	provider)
		if gcloud iam workload-identity-pools providers describe "$WIF_PROVIDER_ID" \
			--workload-identity-pool="$WIF_POOL_ID" --location=global --project="$PROJECT" \
			>/dev/null 2>&1; then
			gcloud iam workload-identity-pools providers delete "$WIF_PROVIDER_ID" \
				--workload-identity-pool="$WIF_POOL_ID" --location=global --project="$PROJECT" --quiet
		fi
		;;
	service-account)
		if gcloud iam service-accounts describe "$WIF_SERVICE_ACCOUNT_EMAIL" \
			--project="$PROJECT" >/dev/null 2>&1; then
			gcloud iam service-accounts delete "$WIF_SERVICE_ACCOUNT_EMAIL" \
				--project="$PROJECT" --quiet
		fi
		;;
	*)
		printf 'unknown acceptance WIF resource: %s\n' "$kind" >&2
		return 2
		;;
	esac
}

cleanup_acceptance_wif() {
	if [[ "$WIF_ACCEPTANCE_OWNED" != 1 ]]; then
		return 0
	fi
	printf '+ removing acceptance-created WIF resources\n'
	local result=0
	if ! delete_acceptance_wif_resource provider; then
		result=1
	fi
	if ! delete_acceptance_wif_resource pool; then
		result=1
	fi
	if ! delete_acceptance_wif_resource service-account; then
		result=1
	fi
	if [[ "$result" == 0 ]]; then
		rm -f "$WIF_OWNERSHIP_MARKER_FILE"
	fi
	return "$result"
}

capture_kubernetes_failure_diagnostics() {
	local kc_path job_name pod_name endpoint token kubeconfig_owned=0
	local -a kubectl_args
	if ! command -v kubectl >/dev/null 2>&1; then
		return 0
	fi
	job_name="${NAME}-${PROFILE}-app-db-grant"
	kc_path="${LOG_DIR}/failure-diagnostics.kubeconfig"
	if PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path" 2>/dev/null; then
		chmod 600 "$kc_path"
		kubectl_args=(--kubeconfig="$kc_path")
		kubeconfig_owned=1
	else
		endpoint="$(gcloud container clusters describe "$CLUSTER_NAME" --region="$REGION" --project="$PROJECT" --format='value(endpoint)' 2>/dev/null || true)"
		token="$(gcloud auth print-access-token --project="$PROJECT" 2>/dev/null || true)"
		if [[ -z "$endpoint" || -z "$token" ]]; then
			printf '+ Kubernetes diagnostics unavailable: kubeconfig and direct GKE access failed\n' >&2
			return 0
		fi
		kubectl_args=(--server="https://${endpoint}" --token="$token" --insecure-skip-tls-verify)
		printf '+ using direct GKE API diagnostics fallback\n'
	fi
	printf '+ capturing Kubernetes diagnostics for %s\n' "$job_name"
	kubectl "${kubectl_args[@]}" get job "$job_name" -o yaml >"${LOG_DIR}/failure-db-grant-job.yaml" 2>&1 || true
	kubectl "${kubectl_args[@]}" get pods -l "job-name=$job_name" -o wide >"${LOG_DIR}/failure-db-grant-pods.txt" 2>&1 || true
	kubectl "${kubectl_args[@]}" describe job "$job_name" >"${LOG_DIR}/failure-db-grant-job.describe.txt" 2>&1 || true
	while IFS= read -r pod_name; do
		[[ -z "$pod_name" ]] && continue
		kubectl "${kubectl_args[@]}" describe pod "$pod_name" >"${LOG_DIR}/failure-${pod_name}.describe.txt" 2>&1 || true
		kubectl "${kubectl_args[@]}" logs "$pod_name" --all-containers --prefix >"${LOG_DIR}/failure-${pod_name}.logs.txt" 2>&1 || true
	done < <(kubectl "${kubectl_args[@]}" get pods -l "job-name=$job_name" -o name 2>/dev/null | sed 's#^pod/##' || true)
	if [[ "$kubeconfig_owned" == 1 ]]; then
		rm -f "$kc_path"
	fi
}

ensure_acceptance_encryption_key() {
	if [[ -n "${MAGELIFT_GCP_ENCRYPTION_KEY_SECRET_ID:-}" ]]; then
		if ! gcloud secrets describe "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" >/dev/null 2>&1; then
			printf 'configured GCP encryption key secret does not exist: %s\n' "$ENCRYPTION_KEY_SECRET_ID" >&2
			return 1
		fi
		printf '+ using configured GCP encryption key secret %s\n' "$ENCRYPTION_KEY_SECRET_ID"
		return 0
	fi
	ENCRYPTION_KEY_ACCEPTANCE_OWNED=1
	if gcloud secrets describe "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" >/dev/null 2>&1; then
		printf '+ reusing acceptance GCP encryption key secret %s\n' "$ENCRYPTION_KEY_SECRET_ID"
		return 0
	fi
	printf '+ creating acceptance GCP encryption key secret %s (value not logged)\n' "$ENCRYPTION_KEY_SECRET_ID"
	gcloud secrets create "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" --replication-policy=automatic >/dev/null
	openssl rand -hex 32 | gcloud secrets versions add "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" --data-file=- >/dev/null
}

cleanup_acceptance_encryption_key() {
	if [[ "$ENCRYPTION_KEY_ACCEPTANCE_OWNED" != 1 ]]; then
		return 0
	fi
	if gcloud secrets describe "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" >/dev/null 2>&1; then
		printf '+ removing acceptance GCP encryption key secret %s\n' "$ENCRYPTION_KEY_SECRET_ID"
		gcloud secrets delete "$ENCRYPTION_KEY_SECRET_ID" --project="$PROJECT" --quiet
	fi
}

cleanup_acceptance_passphrase() {
	if [[ "$PULUMI_ACCEPTANCE_PASSPHRASE_OWNED" == 1 ]]; then
		rm -f "$PULUMI_CONFIG_PASSPHRASE_FILE"
	fi
}

created=0
ORIGIN_DNS_CLAIMED=0
ORIGIN_DNS_ADDRESS=""
ORIGIN_KUBECONFIG=""
cleanup() {
	local ec=$?
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		MAGELIFT_ACCEPTANCE_CLEANUP_REASON="acceptance TTL expired; forced cleanup"
		MAGELIFT_GCP_ACCEPTANCE_KEEP=false
	fi
	if [[ "${MAGELIFT_GCP_ACCEPTANCE_KEEP:-false}" == true ]]; then
		printf '+ KEEP=true; skipping destroy/force_clean/assert_clean (stack retained for resume)\n'
		MAGELIFT_ACCEPTANCE_CLEANUP_REASON="stack retention was explicitly requested" append_shared_cleanup "SKIP" "$NAME-$PROFILE" "magelift-run=${ACCEPTANCE_EVIDENCE_RUN_ID}" ""
		exit "$ec"
	fi
	local cleanup_result=PASS
	if [[ "$ORIGIN_DNS_CLAIMED" == 1 ]]; then
		if ! cloudflare_acceptance_dns_cleanup_a "$ORIGIN_DOMAIN" "$ORIGIN_DNS_MARKER" "$ORIGIN_DNS_ADDRESS"; then
			cleanup_result=FAIL
		fi
	fi
	if [[ "$created" == 1 ]]; then
		capture_kubernetes_failure_diagnostics
		if gcp_destroy_backups_requested; then
			printf '+ magelift destroy --yes --destroy-backups (EXIT trap)\n'
		else
			printf '+ magelift destroy --yes (EXIT trap)\n'
		fi
		if ! (ensure_gcp_adc || refresh_gcp_access_token); then
			printf 'unable to refresh GCP credentials for cleanup\n' >&2
			cleanup_result=FAIL
		elif ! destroy_with_psa_nudge; then
			printf 'destroy failed; attempting force_clean_orphans\n' >&2
			# The exact provider inventory below is authoritative. GCP can report a
			# transient PSA dependency after producer deletion even when the bounded
			# orphan cleanup removes every owned resource.
			MAGELIFT_ACCEPTANCE_CLEANUP_REASON="Pulumi destroy reported a transient provider error; bounded orphan cleanup later proved the ownership prefix empty"
		fi
		if ! force_clean_orphans; then
			cleanup_result=FAIL
		fi
	fi
	if ! cleanup_acceptance_wif; then
		cleanup_result=FAIL
	fi
	if ! cleanup_acceptance_encryption_key; then
		cleanup_result=FAIL
	fi
	if ! remove_acceptance_state_bucket; then
		cleanup_result=FAIL
	fi
	cleanup_acceptance_passphrase
	if ! assert_clean; then
		ec=1
		cleanup_result=FAIL
	fi
	if [[ -n "$ORIGIN_KUBECONFIG" ]]; then
		rm -f "$ORIGIN_KUBECONFIG"
	fi
	append_shared_cleanup "$cleanup_result" "$NAME-$PROFILE" "magelift-run=${ACCEPTANCE_EVIDENCE_RUN_ID}" ""
	exit "$ec"
}
trap cleanup EXIT

prefix_match() {
	# Resources named from naming.Resource(project, env, ...) → mlgcpwt-preview-...
	local kind="$1"
	shift
	local leftover=0
	local line
	while IFS= read -r line; do
		[[ -z "$line" ]] && continue
		if [[ "$kind" == secret && "$ENCRYPTION_KEY_ACCEPTANCE_OWNED" == 1 && "$line" == "$ENCRYPTION_KEY_SECRET_ID" ]]; then
			continue
		fi
		if [[ "$line" == ${NAME}-* ]] || [[ "$line" == *"${NAME}-${PROFILE}"* ]]; then
			printf 'leftover %s: %s\n' "$kind" "$line" >&2
			leftover=1
		fi
	done
	return "$leftover"
}

matches_acceptance_prefix() {
	local value="$1"
	[[ "$value" == ${NAME}-* ]] || [[ "$value" == *"${NAME}-${PROFILE}"* ]]
}

resource_id() {
	printf '%s' "${1##*/}"
}

acceptance_network_endpoint_group_rows() {
	local network_name="${NAME}-${PROFILE}-net" network_url listing
	network_url="https://www.googleapis.com/compute/v1/projects/${PROJECT}/global/networks/${network_name}"
	if ! listing="$(gcloud compute network-endpoint-groups list \
		--project="$PROJECT" --format='json(name,zone,network)' 2>/dev/null)"; then
		printf 'unable to list GCP network endpoint groups\n' >&2
		return 1
	fi
	if ! printf '%s' "$listing" | jq -r \
		--arg network_name "$network_name" \
		--arg network_url "$network_url" \
		'.[]
		| select((.network // "") == $network_name
			or (.network // "") == $network_url
			or ((.network // "") | endswith("/global/networks/" + $network_name)))
		| [.name, (.zone // "")] | @tsv'; then
		printf 'unable to parse GCP network endpoint group inventory\n' >&2
		return 1
	fi
}

cleanup_acceptance_network_endpoint_groups() {
	local rows row name zone
	if ! rows="$(acceptance_network_endpoint_group_rows)"; then
		return 1
	fi
	while IFS=$'\t' read -r name zone; do
		[[ -z "$name" ]] && continue
		if [[ -n "$zone" ]]; then
			zone="$(resource_id "$zone")"
			printf '+ force_clean: deleting run-VPC network endpoint group %s zone=%s\n' "$name" "$zone"
			gcloud compute network-endpoint-groups delete "$name" \
				--zone="$zone" --project="$PROJECT" --quiet 2>/dev/null || true
		else
			printf '+ force_clean: deleting run-VPC global network endpoint group %s\n' "$name"
			gcloud compute network-endpoint-groups delete "$name" \
				--global --project="$PROJECT" --quiet 2>/dev/null || true
		fi
		done <<<"$rows"
}

wait_for_acceptance_network_endpoint_groups() {
	local attempt rows
	for attempt in $(seq 1 20); do
		if ! cleanup_acceptance_network_endpoint_groups; then
			return 1
		fi
		if ! rows="$(acceptance_network_endpoint_group_rows)"; then
			return 1
		fi
		if [[ -z "$rows" ]]; then
			printf '+ force_clean: run-VPC network endpoint groups cleared\n'
			return 0
		fi
		printf '+ force_clean: waiting for run-VPC network endpoint groups to clear attempt=%s/20\n' "$attempt"
		sleep 15
	done
	printf 'force_clean: run-VPC network endpoint groups remain after bounded wait:\n%s\n' "$rows" >&2
	return 1
}

pulumi_cli() {
	if [[ -n "${PULUMI_BACKEND_URL:-}" ]]; then
		PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" pulumi "$@"
	else
		pulumi "$@"
	fi
}

# Magelift uses automation.NewInlineStackWithBackend(..., stackName, "magelift", backendURL).
# Stack name is project-environment (e.g. mlgcpwt-preview); backend is pulumi login or PULUMI_BACKEND_URL.
pulumi_fq_stack_ref() {
	local ref org
	if ! command -v pulumi >/dev/null 2>&1; then
		printf '%s/%s' "$PULUMI_PROJECT" "$STACK_NAME"
		return 0
	fi
	if command -v jq >/dev/null 2>&1; then
		ref="$(pulumi_cli stack ls --all --json 2>/dev/null | jq -r --arg name "$STACK_NAME" --arg project "$PULUMI_PROJECT" '
			.[] | select(
				.name == $name
				or .name == ($project + "/" + $name)
				or (.name | endswith("/" + $project + "/" + $name))
			) | .name' | head -n 1)"
		if [[ -n "$ref" && "$ref" != "null" ]]; then
			printf '%s' "$ref"
			return 0
		fi
	fi
	org="$(pulumi_cli whoami 2>/dev/null | head -n 1 | tr -d '[:space:]')"
	if [[ -n "$org" ]]; then
		printf '%s/%s/%s' "$org" "$PULUMI_PROJECT" "$STACK_NAME"
		return 0
	fi
	printf '%s/%s' "$PULUMI_PROJECT" "$STACK_NAME"
}

magelift_json_from_output() {
	local raw="$1"
	printf '%s\n' "$raw" | awk '/^\{/{found=1} found'
}

pulumi_stack_has_managed_resources() {
	if ! command -v pulumi >/dev/null 2>&1; then
		return 1
	fi
	local fq_ref export_json count
	fq_ref="$(pulumi_fq_stack_ref)"
	export_json="$(pulumi_cli stack export --stack "$fq_ref" 2>/dev/null)" || return 1
	if ! command -v jq >/dev/null 2>&1; then
		[[ -n "$export_json" ]]
		return
	fi
	if ! count="$(printf '%s' "$export_json" | jq -e '[.deployment.resources[]? | select(.type != "pulumi:pulumi:Stack")] | length' 2>/dev/null)"; then
		printf 'pulumi_stack_has_managed_resources: jq parse failed on stack export; treating as stale\n' >&2
		return 0
	fi
	[[ "${count:-0}" -gt 0 ]]
}

preview_reports_stale_state() {
	local preview_raw preview_json stale
	# Preview failure (wrong backend, missing stack, auth) is not proof of stale
	# managed resources; pulumi_stack_has_managed_resources already covered export.
	preview_raw="$(run preview 2>/dev/null)" || {
		printf 'preview_reports_stale_state: magelift preview failed; treating as clean (no proof of stale)\n' >&2
		return 1
	}
	if ! command -v jq >/dev/null 2>&1; then
		printf 'preview_reports_stale_state: jq missing; treating as clean (no proof of stale)\n' >&2
		return 1
	fi
	preview_json="$(magelift_json_from_output "$preview_raw")"
	if ! stale="$(printf '%s' "$preview_json" | jq -e '[.preview.changes[]? | select(.operation == "same" or .operation == "update" or .operation == "delete") | .count] | add // 0' 2>/dev/null)"; then
		printf 'preview_reports_stale_state: jq parse failed; treating as clean (no proof of stale)\n' >&2
		return 1
	fi
	[[ "${stale:-0}" -gt 0 ]]
}

remove_pulumi_stack_state() {
	local fq_ref
	fq_ref="$(pulumi_fq_stack_ref)"
	printf '+ remove_pulumi_stack_state stack=%s\n' "$fq_ref"
	if command -v pulumi >/dev/null 2>&1; then
		if pulumi_cli stack rm "$fq_ref" --yes --force 2>/dev/null; then
			printf 'pulumi stack rm ok\n'
			return 0
		fi
		if pulumi_cli stack rm "$STACK_NAME" --yes --force 2>/dev/null; then
			printf 'pulumi stack rm ok (short name)\n'
			return 0
		fi
		if pulumi_cli stack rm "${PULUMI_PROJECT}/${STACK_NAME}" --yes --force 2>/dev/null; then
			printf 'pulumi stack rm ok (project-qualified)\n'
			return 0
		fi
	fi
	if [[ "${PULUMI_BACKEND_URL:-}" == file://* ]]; then
		local backend_dir="${PULUMI_BACKEND_URL#file://}"
		rm -rf "${backend_dir}/.pulumi/stacks/${PULUMI_PROJECT}/${STACK_NAME}.json" \
			"${backend_dir}/.pulumi/stacks/${PULUMI_PROJECT}/${STACK_NAME}.json.attrs" \
			"${backend_dir}/.pulumi/stacks/${PULUMI_PROJECT}/${STACK_NAME}.json.bak" 2>/dev/null || true
	fi
	rm -rf "${WORKDIR}/.pulumi" 2>/dev/null || true
}

# After manual force_clean_orphans, GCP can be empty while Pulumi still tracks VPC/secrets.
reconcile_stale_pulumi_state() {
	if ! assert_clean; then
		printf '+ reconcile_stale_pulumi_state skipped (GCP leftovers remain)\n'
		return 0
	fi
	local fq_ref
	fq_ref="$(pulumi_fq_stack_ref)"
	printf '+ reconcile_stale_pulumi_state: GCP clean; resetting Pulumi stack %s\n' "$fq_ref"
	if run_magelift_destroy 2>"${LOG_DIR}/reconcile-destroy.stderr"; then
		printf 'magelift destroy ok\n'
	else
		printf 'magelift destroy failed; removing Pulumi stack state directly\n' >&2
		remove_pulumi_stack_state
	fi
	if pulumi_stack_has_managed_resources; then
		printf 'Pulumi stack still has managed resources after destroy; forcing stack rm\n' >&2
		remove_pulumi_stack_state
	fi
	if pulumi_stack_has_managed_resources; then
		printf 'reconcile_stale_pulumi_state FAILED: Pulumi stack still has resources\n' >&2
		return 1
	fi
	if preview_reports_stale_state; then
		printf 'reconcile_stale_pulumi_state FAILED: preview still reports unchanged resources\n' >&2
		return 1
	fi
	printf '+ reconcile_stale_pulumi_state ok\n'
	return 0
}

assert_clean() {
	printf '+ assert_clean project=%s\n' "$PROJECT"
	local failed=0
	local listing bucket network_endpoint_groups
	if ! listing="$(gcloud compute networks list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list GCP networks\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match network; then
		failed=1
	fi
	if ! network_endpoint_groups="$(acceptance_network_endpoint_group_rows)"; then
		failed=1
	elif [[ -n "$network_endpoint_groups" ]]; then
		printf 'leftover run-VPC network endpoint groups:\n%s\n' "$network_endpoint_groups" >&2
		failed=1
	fi
	if ! listing="$(gcloud container clusters list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list GKE clusters\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match gke; then
		failed=1
	fi
	if ! listing="$(gcloud sql instances list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list Cloud SQL instances\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match sql; then
		failed=1
	fi
	if ! listing="$(gcloud memorystore instances list --location="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list Memorystore instances\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match memorystore; then
		failed=1
	fi
	if ! listing="$(gcloud network-connectivity service-connection-policies list --region="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list service connection policies\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match scp; then
		failed=1
	fi
	if ! listing="$(gcloud secrets list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list Secret Manager secrets\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match secret; then
		failed=1
	fi
	if ! listing="$(gcloud compute addresses list --global --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list global addresses\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match address; then
		failed=1
	fi
	if ! listing="$(gcloud compute security-policies list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list Cloud Armor security policies\n' >&2
		failed=1
	elif ! printf '%s\n' "$listing" | prefix_match security-policy; then
		failed=1
	fi
	if ! listing="$(gcloud storage buckets list --project="$PROJECT" --format='value(name)' 2>/dev/null)"; then
		printf 'unable to list GCS buckets\n' >&2
		failed=1
	else
		while IFS= read -r bucket; do
			[[ -z "$bucket" ]] && continue
			bucket="${bucket#gs://}"
			if [[ "$AUTO_STATE_BUCKET" == 1 && "$bucket" == "$STATE_BUCKET_NAME" ]]; then
				continue
			fi
			if [[ "$bucket" == ${NAME}-* ]] || [[ "$bucket" == *"${NAME}-${PROFILE}"* ]]; then
				printf 'leftover bucket: %s\n' "$bucket" >&2
				failed=1
			fi
		done <<<"$listing"
	fi
	if ! assert_no_leftover_cloudsql_backups; then
		failed=1
	fi
	if [[ "$failed" != 0 ]]; then
		printf 'assert_clean FAILED: leftovers remain in %s\n' "$PROJECT" >&2
		return 1
	fi
	printf 'assert_clean ok\n'
	return 0
}

acceptance_resource_lines() {
	local line
	while IFS= read -r line; do
		[[ -z "$line" ]] && continue
		matches_acceptance_prefix "$line" || continue
		printf '%s\n' "$line"
	done
}

issue_producer_deletes() {
	local resource
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		gcloud container clusters delete "$resource" --region="$REGION" --project="$PROJECT" --quiet --async 2>/dev/null || true
	done < <(gcloud container clusters list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		gcloud sql instances delete "$resource" --project="$PROJECT" --quiet --async 2>/dev/null || true
	done < <(gcloud sql instances list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		gcloud memorystore instances delete "$(resource_id "$resource")" --location="$REGION" --project="$PROJECT" --quiet --async 2>/dev/null || true
	done < <(gcloud memorystore instances list --location="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		gcloud network-connectivity service-connection-policies delete "$(resource_id "$resource")" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	done < <(gcloud network-connectivity service-connection-policies list --region="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		resource="${resource#gs://}"
		[[ "$AUTO_STATE_BUCKET" == 1 && "$resource" == "$STATE_BUCKET_NAME" ]] && continue
		gcloud storage rm -r "gs://${resource}" --project="$PROJECT" 2>/dev/null || true
	done < <(gcloud storage buckets list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
}

remove_acceptance_state_bucket() {
	if [[ "$AUTO_STATE_BUCKET" != 1 ]]; then
		return 0
	fi
	if ! gcloud storage buckets describe "gs://${STATE_BUCKET_NAME}" --project="$PROJECT" >/dev/null 2>&1; then
		return 0
	fi
	printf '+ removing auto-created Pulumi state bucket gs://%s\n' "$STATE_BUCKET_NAME"
	if ! gcloud storage rm -r "gs://${STATE_BUCKET_NAME}" --project="$PROJECT"; then
		printf 'failed to remove auto-created Pulumi state bucket %s\n' "$STATE_BUCKET_NAME" >&2
		return 1
	fi
	if gcloud storage buckets describe "gs://${STATE_BUCKET_NAME}" --project="$PROJECT" >/dev/null 2>&1; then
		printf 'auto-created Pulumi state bucket still exists: %s\n' "$STATE_BUCKET_NAME" >&2
		return 1
	fi
	return 0
}

producer_resources_remain() {
	local kind count
	for kind in gke sql memorystore scp; do
		case "$kind" in
		gke)
			count="$(gcloud container clusters list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines | wc -l)"
			;;
		sql)
			count="$(gcloud sql instances list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines | wc -l)"
			;;
		memorystore)
			count="$(gcloud memorystore instances list --location="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines | wc -l)"
			;;
		scp)
			count="$(gcloud network-connectivity service-connection-policies list --region="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines | wc -l)"
			;;
		esac
		count="${count//[[:space:]]/}"
		if [[ "${count:-0}" -gt 0 ]]; then
			return 0
		fi
	done
	return 1
}

wait_for_producer_deletes() {
	local start elapsed
	start="$(date +%s)"
	while producer_resources_remain; do
		elapsed=$(( $(date +%s) - start ))
		if [[ "$elapsed" -ge "$FORCE_CLEAN_TIMEOUT_SECS" ]]; then
			printf 'force_clean: timeout after %ds waiting for GKE/SQL/Memorystore/SCP deletes\n' "$FORCE_CLEAN_TIMEOUT_SECS" >&2
			gcloud container clusters list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines >&2 || true
			gcloud sql instances list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines >&2 || true
			gcloud memorystore instances list --location="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines >&2 || true
			gcloud network-connectivity service-connection-policies list --region="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines >&2 || true
			return 1
		fi
		printf '+ force_clean: waiting for producer deletes (GKE/SQL/Memorystore/SCP) elapsed=%ds\n' "$elapsed"
		issue_producer_deletes
		sleep "$FORCE_CLEAN_POLL_INTERVAL_SECS"
	done
	printf '+ force_clean: producer resources cleared\n'
	return 0
}

# Cloud SQL release of PSA can lag minutes after instance delete
# (FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION). Prefer compute removePeering
# after a soak; fall back to services vpc-peerings delete.
force_clean_orphans() {
	local net="${NAME}-${PROFILE}-net"
	local resource
	local soak_secs="${MAGELIFT_GCP_PSA_SOAK_SECS:-180}"
	printf '+ force_clean_orphans prefix=%s-%s timeout=%ss psa_soak=%ss\n' \
		"$NAME" "$PROFILE" "$FORCE_CLEAN_TIMEOUT_SECS" "$soak_secs"
	issue_producer_deletes
	wait_for_producer_deletes || return 1
	if ! cleanup_leftover_cloudsql_backups; then
		return 1
	fi
	# GKE can leave zonal/global NEGs behind after the cluster delete operation
	# reports complete. They are still attached to this VPC and block network
	# deletion; scope cleanup by the exact run-owned network, not by NEG name.
	wait_for_acceptance_network_endpoint_groups || return 1
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		matches_acceptance_prefix "$resource" || continue
		gcloud secrets delete "$resource" --project="$PROJECT" --quiet 2>/dev/null || true
	done < <(gcloud secrets list --project="$PROJECT" --format='value(name)' 2>/dev/null || true)
	# Cloud Armor policies are global resources and are not producer resources;
	# delete them after the Pulumi attempt so a provider failure cannot leave
	# run-owned CEL rules consuming the project's global CEVAL quota.
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		matches_acceptance_prefix "$resource" || continue
		printf '+ force_clean: deleting Cloud Armor security policy %s\n' "$resource"
		gcloud compute security-policies delete "$resource" --project="$PROJECT" --quiet 2>/dev/null || true
	done < <(gcloud compute security-policies list --project="$PROJECT" --format='value(name)' 2>/dev/null || true)
	gcloud compute routers nats delete "${NAME}-${PROFILE}-net-nat" --router="${NAME}-${PROFILE}-net-router" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	gcloud compute routers delete "${NAME}-${PROFILE}-net-router" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	local subnet
	for subnet in private-0 private-1 private-2 public-0 public-1 public-2; do
		gcloud compute networks subnets delete "${NAME}-${PROFILE}-net-${subnet}" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	done
	if ! gcloud compute networks describe "$net" --project="$PROJECT" >/dev/null 2>&1; then
		gcloud compute addresses delete "${NAME}-${PROFILE}-sql-psa" --global --project="$PROJECT" --quiet 2>/dev/null || true
		printf '+ force_clean: network %s absent; skipping Cloud SQL PSA soak\n' "$net"
		return 0
	fi
	printf '+ force_clean: soaking %ss for Cloud SQL PSA release\n' "$soak_secs"
	sleep "$soak_secs"
	# Retry PSA peering teardown until the VPC is gone or attempts exhaust.
	for _ in $(seq 1 20); do
		if ! gcloud compute networks describe "$net" --project="$PROJECT" >/dev/null 2>&1; then
			printf '+ force_clean: network %s already gone\n' "$net"
			break
		fi
		if gcloud compute networks peerings list --network="$net" --project="$PROJECT" --format='value(peerings[].name)' 2>/dev/null | grep -qx 'servicenetworking-googleapis-com'; then
			request_psa_peering_delete "$net"
			sleep 30
			continue
		fi
		gcloud compute addresses delete "${NAME}-${PROFILE}-sql-psa" --global --project="$PROJECT" --quiet 2>/dev/null || true
		if gcloud compute networks delete "$net" --project="$PROJECT" --quiet 2>/dev/null; then
			break
		fi
		sleep 30
	done
	gcloud compute addresses delete "${NAME}-${PROFILE}-sql-psa" --global --project="$PROJECT" --quiet 2>/dev/null || true
	gcloud compute networks delete "$net" --project="$PROJECT" --quiet 2>/dev/null || true
}

# Install the cleanup trap before creating the disposable state bucket, then
# wait until every cleanup helper has been defined before invoking it. A
# provider or local build failure after this point must still remove the bucket.
ACCEPTANCE_TTL_MARKER_FILE="$WORKDIR/acceptance-ttl-expired"
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ACCEPTANCE_TTL_MARKER_FILE"
for api in "${APIS[@]}"; do
	gcloud services enable "$api" --project="$PROJECT" >/dev/null
done
ensure_acceptance_state_bucket

# --- live cell helpers (create-once then updates; never recreate) -----------------

checkpoint_has_cells() {
	acceptance_checkpoint_load
	local n
	n=$(jq '.cells | length' "$ACCEPTANCE_CHECKPOINT")
	[[ "${n:-0}" -gt 0 ]]
}

should_skip_create_once() {
	local resume="${MAGELIFT_GCP_ACCEPTANCE_RESUME:-0}"
	if [[ "$resume" == "1" || "$resume" == "true" ]]; then
		return 0
	fi
	load_cells
	if [[ ${#CELLS[@]} -gt 0 ]] && cell_done "${CELLS[0]}"; then
		return 0
	fi
	if checkpoint_has_cells; then
		return 0
	fi
	return 1
}

acceptance_account_id() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_ACCOUNT:-}" ]]; then
		printf '%s' "$MAGELIFT_ACCEPTANCE_ACCOUNT"
		return 0
	fi
	printf '%s' "$PROJECT"
}

application_url_from_outputs() {
	local raw json url svc kc_path
	raw="$(run outputs 2>/dev/null)" || true
	json="$(magelift_json_from_output "$raw")"
	if command -v jq >/dev/null 2>&1 && [[ -n "$json" ]]; then
		url="$(printf '%s' "$json" | jq -r '
			.outputs.applicationURL // .outputs.applicationUrl // .applicationURL // .applicationUrl //
			.outputs.ingressHostname // .outputs.loadBalancerIP // empty
		' 2>/dev/null | head -n 1)"
	fi
	# Autopilot often skips LB await; applicationURL stays empty while the
	# Service eventually gets an external IP. Fall back to kubectl.
	if [[ -z "$url" || "$url" == "null" ]] && command -v kubectl >/dev/null 2>&1; then
		kc_path="${MAGELIFT_KUBECONFIG:-}"
		if [[ -z "$kc_path" || ! -f "$kc_path" ]]; then
			kc_path="${LOG_DIR}/cutover-dns.kubeconfig"
			if PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
				pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path" 2>/dev/null; then
				chmod 600 "$kc_path"
			else
				kc_path=""
			fi
		fi
		if [[ -n "$kc_path" ]]; then
			svc="${NAME}-${PROFILE}-app-web"
			for _ in $(seq 1 30); do
				url="$(kubectl --kubeconfig="$kc_path" get svc "$svc" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
				[[ -z "$url" ]] && url="$(kubectl --kubeconfig="$kc_path" get svc "$svc" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
				[[ -n "$url" ]] && break
				sleep 10
			done
		fi
	fi
	[[ -n "$url" && "$url" != "null" ]] || return 1
	# Strip scheme for DNS TARGET when present.
	url="${url#https://}"
	url="${url#http://}"
	url="${url%%/*}"
	printf '%s' "$url"
}

# Seed dumps keep Magento base_url on localhost. CheckRuntime GETs the Service
# LoadBalancer and treats a 302 to localhost as unhealthy. Native edge later
# overwrites these with https://ORIGIN_DOMAIN/.
gcp_apply_magento_storefront_base_url() {
	local host base_url
	host="$(application_url_from_outputs)" || {
		printf 'Magento storefront base_url needs a LoadBalancer host\n' >&2
		return 1
	}
	base_url="http://${host}/"
	printf '+ setting Magento storefront base_url=%q (public origin URL, not a secret)\n' "$base_url"
	if ! run exec --service web -- bin/magento --no-ansi config:set web/unsecure/base_url "$base_url" 2>&1 | tee "${LOG_DIR}/cell-storefront-base-url-unsecure.log"; then
		printf 'could not set Magento web/unsecure/base_url\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi config:set web/secure/base_url "$base_url" 2>&1 | tee "${LOG_DIR}/cell-storefront-base-url-secure.log"; then
		printf 'could not set Magento web/secure/base_url\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi cache:flush 2>&1 | tee "${LOG_DIR}/cell-storefront-base-url-cache-flush.log"; then
		printf 'could not flush Magento cache after storefront base_url\n' >&2
		return 1
	fi
}

# bootstrap:wif; Ensure WIF on the project PLUS Act or gcloud STS/WIF exchange proof.
# Ensure-only is forbidden (D-05 / GCP-01).
prove_wif_token_exchange() {
	local ci_sa pool_hint proof_log
	if [[ "${MAGELIFT_GCP_WIF_ENSURE_ONLY:-}" == "1" || "${MAGELIFT_GCP_WIF_ENSURE_ONLY:-}" == "true" ]]; then
		printf 'bootstrap:wif refused: MAGELIFT_GCP_WIF_ENSURE_ONLY forbids Ensure-only (need Act or STS exchange)\n' >&2
		return 1
	fi
	proof_log="${MAGELIFT_GCP_WIF_ACT_LOG:-}"
	if [[ -n "$proof_log" && -f "$proof_log" ]] && grep -Eqi 'WIF token exchange succeeded|token exchange' "$proof_log"; then
		printf '+ bootstrap:wif Act proof accepted from %s\n' "$proof_log"
		return 0
	fi
	if [[ "${MAGELIFT_GCP_WIF_ACT_PROOF:-}" == "1" || "${MAGELIFT_GCP_WIF_ACT_PROOF:-}" == "true" ]]; then
		printf '+ bootstrap:wif Act proof flag MAGELIFT_GCP_WIF_ACT_PROOF=1 (operator attested Act smoke)\n'
		return 0
	fi
	# Bootstrap SA ID is ml-<project>-<env>-ci (internal/cloud/gcp/bootstrap/identity.go).
	ci_sa="${MAGELIFT_GCP_WIF_SERVICE_ACCOUNT:-ml-${NAME}-${PROFILE}-ci@${PROJECT}.iam.gserviceaccount.com}"
	pool_hint="${MAGELIFT_GCP_WIF_PROVIDER:-}"
	printf '+ bootstrap:wif attempting gcloud STS/impersonation token exchange for %s\n' "$ci_sa"
	if [[ -n "$pool_hint" ]]; then
		printf '+ bootstrap:wif provider=%s\n' "$pool_hint"
	fi
	if TOKEN="$(gcloud auth print-access-token --impersonate-service-account="$ci_sa" --project="$PROJECT" 2>/dev/null)" \
		&& [[ -n "$TOKEN" ]]; then
		printf '+ bootstrap:wif STS/impersonation token exchange ok (token not logged)\n'
		return 0
	fi
	printf 'bootstrap:wif FAILED: need Act proof (MAGELIFT_GCP_WIF_ACT_LOG or MAGELIFT_GCP_WIF_ACT_PROOF=1) or gcloud STS/WIF exchange; Ensure-only is not enough\n' >&2
	return 1
}

run_gcp_queue_health_cell() {
	local kc_path queue expected timeout_secs poll_interval cluster_status_max_attempts elapsed ready headless_json pods_json pod pod_count cluster_status cluster_status_ok cluster_status_attempts node zone zone_count
	case "$PROFILE" in
	standard) expected=1 ;;
	high-availability) expected=2 ;;
	*)
		printf 'queue:health requires standard or high-availability profile\n' >&2
		return 2
		;;
	esac
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'queue:health requires kubectl and jq on PATH\n' >&2
		return 2
	fi
	queue="${NAME}-${PROFILE}-rabbitmq"
	kc_path="${LOG_DIR}/queue-health.kubeconfig"
	if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
		printf 'queue:health could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	timeout_secs="${MAGELIFT_GCP_QUEUE_HEALTH_TIMEOUT_SECS:-900}"
	poll_interval="${MAGELIFT_GCP_QUEUE_HEALTH_POLL_INTERVAL_SECS:-10}"
	cluster_status_max_attempts="${MAGELIFT_GCP_QUEUE_CLUSTER_STATUS_ATTEMPTS:-12}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ || ! "$cluster_status_max_attempts" =~ ^[1-9][0-9]*$ ]]; then
		printf 'queue health timeout, poll interval, and cluster status attempts must be positive integers\n' >&2
		return 2
	fi
	printf '+ queue:health waiting for %s ready RabbitMQ pods (%s)\n' "$expected" "$queue"
	ready=0
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		ready="$(kubectl --kubeconfig="$kc_path" get statefulset "$queue" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
		ready="${ready:-0}"
		if [[ "$ready" =~ ^[0-9]+$ ]] && (( ready >= expected )); then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf 'queue:health timed out ready=%s expected=%s\n' "$ready" "$expected" >&2
			return 1
		fi
		sleep "$poll_interval"
	done

	if ! headless_json="$(kubectl --kubeconfig="$kc_path" get service "${queue}-headless" -o json 2>/dev/null)"; then
		printf 'queue:health could not read the RabbitMQ headless Service\n' >&2
		return 1
	fi
	if ! printf '%s' "$headless_json" | jq -e '.spec.clusterIP == "None" and .spec.publishNotReadyAddresses == true' >/dev/null; then
		printf 'queue:health headless Service does not publish not-ready endpoints\n' >&2
		return 1
	fi
	pods_json="$(kubectl --kubeconfig="$kc_path" get pods -l "app=$queue" -o json)"
	pod_count="$(printf '%s' "$pods_json" | jq -r '.items | length')"
	if [[ "$pod_count" != "$expected" ]]; then
		printf 'queue:health pod count=%s expected=%s\n' "$pod_count" "$expected" >&2
		return 1
	fi
	: >"${LOG_DIR}/cell-queue-health.log"
	while IFS= read -r pod; do
		[[ -z "$pod" ]] && continue
		cluster_status=""
		cluster_status_ok=0
		cluster_status_attempts=0
		while (( cluster_status_attempts < cluster_status_max_attempts )); do
			cluster_status_attempts=$((cluster_status_attempts + 1))
			if cluster_status="$(kubectl --kubeconfig="$kc_path" exec "$pod" -c rabbitmq -- rabbitmq-diagnostics -q cluster_status 2>&1)"; then
				cluster_status_ok=1
				break
			fi
			if (( cluster_status_attempts < cluster_status_max_attempts )); then
				sleep "$poll_interval"
			fi
		done
		if (( cluster_status_ok != 1 )); then
			printf 'queue:health cluster_status failed for pod=%s\n' "$pod" >&2
			printf '%s\n' "$cluster_status" >>"${LOG_DIR}/cell-queue-health.log"
			return 1
		fi
		printf 'pod=%s\n%s\n' "$pod" "$cluster_status" >>"${LOG_DIR}/cell-queue-health.log"
		while IFS= read -r peer; do
			[[ -z "$peer" ]] && continue
			if ! printf '%s' "$cluster_status" | grep -Fq "rabbit@${peer}"; then
				printf 'queue:health pod=%s does not report cluster member=%s\n' "$pod" "$peer" >&2
				return 1
			fi
		done < <(printf '%s' "$pods_json" | jq -r '.items[] | .metadata.name')
	done < <(printf '%s' "$pods_json" | jq -r '.items[] | .metadata.name')

	if [[ "$PROFILE" == high-availability ]]; then
		: >"${LOG_DIR}/cell-queue-zones.log"
		while IFS= read -r node; do
			[[ -z "$node" ]] && continue
			zone="$(kubectl --kubeconfig="$kc_path" get node "$node" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}' 2>/dev/null || true)"
			printf '%s\t%s\n' "$node" "$zone" >>"${LOG_DIR}/cell-queue-zones.log"
		done < <(printf '%s' "$pods_json" | jq -r '.items[] | .spec.nodeName')
		zone_count="$(awk -F '\t' 'NF > 1 && $2 != "" {seen[$2] = 1} END {count = 0; for (zone in seen) count++; print count}' "${LOG_DIR}/cell-queue-zones.log")"
		if [[ "$zone_count" -lt 2 ]]; then
			printf 'queue:health HA pods are in %s zone(s), expected at least 2\n' "$zone_count" >&2
			return 1
		fi
	fi
	printf 'queue:health ok replicas=%s cluster_membership=verified zones=%s\n' "$expected" "${zone_count:-1}"
}

run_gcp_search_health_cell() {
	local kc_path search expected timeout_secs poll_interval elapsed ready headless_json endpoints_json endpoint_count pods_json pod_count pod cluster_health cluster_attempts node zone zone_count
	case "$PROFILE" in
	preview|standard) expected=1 ;;
	high-availability) expected=3 ;;
	*)
		printf 'search:health does not support profile %s\n' "$PROFILE" >&2
		return 2
		;;
	esac
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'search:health requires kubectl and jq on PATH\n' >&2
		return 2
	fi
	search="${NAME}-${PROFILE}-search"
	kc_path="${LOG_DIR}/search-health.kubeconfig"
	if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
		printf 'search:health could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	timeout_secs="${MAGELIFT_GCP_SEARCH_HEALTH_TIMEOUT_SECS:-900}"
	poll_interval="${MAGELIFT_GCP_SEARCH_HEALTH_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'search health timeout and poll interval must be positive integers\n' >&2
		return 2
	fi

	printf '+ search:health waiting for %s ready OpenSearch pods (%s)\n' "$expected" "$search"
	ready=0
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		ready="$(kubectl --kubeconfig="$kc_path" get statefulset "$search" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
		ready="${ready:-0}"
		if [[ "$ready" =~ ^[0-9]+$ ]] && (( ready >= expected )); then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf 'search:health timed out ready=%s expected=%s\n' "$ready" "$expected" >&2
			kubectl --kubeconfig="$kc_path" describe statefulset "$search" >"${LOG_DIR}/failure-${search}.statefulset.describe.txt" 2>&1 || true
			kubectl --kubeconfig="$kc_path" get pods -l "app=$search" -o wide >"${LOG_DIR}/failure-${search}.pods.txt" 2>&1 || true
			return 1
		fi
		sleep "$poll_interval"
	done

	if ! headless_json="$(kubectl --kubeconfig="$kc_path" get service "${search}-headless" -o json 2>/dev/null)"; then
		printf 'search:health could not read the OpenSearch headless Service\n' >&2
		return 1
	fi
	if ! printf '%s' "$headless_json" | jq -e '.spec.clusterIP == "None" and .spec.publishNotReadyAddresses == true' >/dev/null; then
		printf 'search:health headless Service does not publish not-ready addresses\n' >&2
		return 1
	fi
	if ! endpoints_json="$(kubectl --kubeconfig="$kc_path" get endpoints "${search}-headless" -o json 2>/dev/null)"; then
		printf 'search:health could not read the OpenSearch headless endpoints\n' >&2
		return 1
	fi
	endpoint_count="$(printf '%s' "$endpoints_json" | jq '[.subsets[]?.addresses[]?] | length')"
	if [[ "$endpoint_count" -lt "$expected" ]]; then
		printf 'search:health endpoint count=%s expected=%s\n' "$endpoint_count" "$expected" >&2
		return 1
	fi
	pods_json="$(kubectl --kubeconfig="$kc_path" get pods -l "app=$search" -o json)"
	pod_count="$(printf '%s' "$pods_json" | jq -r '.items | length')"
	if [[ "$pod_count" != "$expected" ]]; then
		printf 'search:health pod count=%s expected=%s\n' "$pod_count" "$expected" >&2
		return 1
	fi
	: >"${LOG_DIR}/cell-search-health.log"
	while IFS= read -r pod; do
		[[ -z "$pod" ]] && continue
		cluster_health=""
		cluster_attempts=0
		while (( cluster_attempts < 12 )); do
			cluster_attempts=$((cluster_attempts + 1))
			if cluster_health="$(kubectl --kubeconfig="$kc_path" exec "$pod" -c opensearch -- sh -ec 'if command -v curl >/dev/null 2>&1; then curl -fsS --max-time 10 "http://127.0.0.1:9200/_cluster/health?wait_for_status=yellow&timeout=5s"; elif command -v wget >/dev/null 2>&1; then wget -qO- "http://127.0.0.1:9200/_cluster/health?wait_for_status=yellow&timeout=5s"; else exit 127; fi' 2>&1)"; then
				break
			fi
			if (( cluster_attempts == 12 )); then
				printf 'search:health cluster health failed for pod=%s\n' "$pod" >&2
				printf 'pod=%s\n%s\n' "$pod" "$cluster_health" >>"${LOG_DIR}/cell-search-health.log"
				kubectl --kubeconfig="$kc_path" describe pod "$pod" >"${LOG_DIR}/failure-${pod}.describe.txt" 2>&1 || true
				kubectl --kubeconfig="$kc_path" logs "$pod" -c opensearch >"${LOG_DIR}/failure-${pod}.logs.txt" 2>&1 || true
				return 1
			fi
			sleep "$poll_interval"
		done
		printf 'pod=%s\n%s\n' "$pod" "$cluster_health" >>"${LOG_DIR}/cell-search-health.log"
		if ! printf '%s' "$cluster_health" | jq -e --argjson expected "$expected" '(.status == "green" or .status == "yellow") and .number_of_nodes >= $expected' >/dev/null; then
			printf 'search:health cluster is not ready for pod=%s: %s\n' "$pod" "$cluster_health" >&2
			return 1
		fi
	done < <(printf '%s' "$pods_json" | jq -r '.items[] | .metadata.name')

	if [[ "$PROFILE" == high-availability ]]; then
		: >"${LOG_DIR}/cell-search-zones.log"
		while IFS= read -r node; do
			[[ -z "$node" ]] && continue
			zone="$(kubectl --kubeconfig="$kc_path" get node "$node" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}' 2>/dev/null || true)"
			printf '%s\t%s\n' "$node" "$zone" >>"${LOG_DIR}/cell-search-zones.log"
		done < <(printf '%s' "$pods_json" | jq -r '.items[] | .spec.nodeName')
		zone_count="$(awk -F '\t' 'NF > 1 && $2 != "" {seen[$2] = 1} END {count = 0; for (zone in seen) count++; print count}' "${LOG_DIR}/cell-search-zones.log")"
		if [[ "$zone_count" -lt 2 ]]; then
			printf 'search:health HA pods are in %s zone(s), expected at least 2\n' "$zone_count" >&2
			return 1
		fi
	fi
	printf 'search:health ok replicas=%s cluster_nodes_verified endpoints=%s zones=%s\n' "$expected" "$endpoint_count" "${zone_count:-1}"
}

run_gcp_pod_loss_cell() {
	local kc_path web deployment_json desired ready_before pod pods_json replacement ready old_present
	local timeout_secs poll_interval elapsed
	if [[ "$PROFILE" != high-availability ]]; then
		printf 'resilience:pod-loss requires the high-availability profile\n' >&2
		return 2
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'resilience:pod-loss requires kubectl and jq on PATH\n' >&2
		return 2
	fi
	web="${NAME}-${PROFILE}-app-web"
	kc_path="${LOG_DIR}/pod-loss.kubeconfig"
	if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
		printf 'resilience:pod-loss could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	if ! deployment_json="$(kubectl --kubeconfig="$kc_path" get deployment "$web" -o json)"; then
		printf 'resilience:pod-loss could not read web Deployment=%s\n' "$web" >&2
		return 1
	fi
	desired="$(printf '%s' "$deployment_json" | jq -r '.spec.replicas // 0')"
	ready_before="$(printf '%s' "$deployment_json" | jq -r '.status.readyReplicas // 0')"
	if [[ ! "$desired" =~ ^[0-9]+$ || "$desired" -lt 2 || ! "$ready_before" =~ ^[0-9]+$ || "$ready_before" -lt 2 ]]; then
		printf 'resilience:pod-loss requires at least two ready web replicas (desired=%s ready=%s)\n' "$desired" "$ready_before" >&2
		return 1
	fi
	if ! pods_json="$(kubectl --kubeconfig="$kc_path" get pods -l "app=$web" -o json)"; then
		printf 'resilience:pod-loss could not list web pods\n' >&2
		return 1
	fi
	pod="$(printf '%s' "$pods_json" | jq -r '.items[] | select(.status.phase == "Running") | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name' | head -n 1)"
	if [[ -z "$pod" ]]; then
		printf 'resilience:pod-loss found no ready web pod to delete\n' >&2
		return 1
	fi
	printf '+ resilience:pod-loss deleting pod=%s deployment=%s ready=%s desired=%s\n' "$pod" "$web" "$ready_before" "$desired"
	kubectl --kubeconfig="$kc_path" delete pod "$pod" --wait=false >/dev/null
	timeout_secs="${MAGELIFT_GCP_POD_LOSS_TIMEOUT_SECS:-600}"
	poll_interval="${MAGELIFT_GCP_POD_LOSS_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:pod-loss timeout and poll interval must be positive integers\n' >&2
		return 2
	fi
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		deployment_json="$(kubectl --kubeconfig="$kc_path" get deployment "$web" -o json 2>/dev/null || true)"
		ready="$(printf '%s' "$deployment_json" | jq -r '.status.readyReplicas // 0' 2>/dev/null || printf '0')"
		pods_json="$(kubectl --kubeconfig="$kc_path" get pods -l "app=$web" -o json 2>/dev/null || printf '{"items":[]}')"
		replacement="$(printf '%s' "$pods_json" | jq -r --arg old "$pod" '.items[] | select(.metadata.name != $old) | select(.status.phase == "Running") | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name' | head -n 1)"
		old_present="$(printf '%s' "$pods_json" | jq -r --arg old "$pod" '[.items[] | select(.metadata.name == $old)] | length')"
		if [[ "$ready" =~ ^[0-9]+$ && "$ready" -ge "$desired" && -n "$replacement" && "$old_present" == 0 ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf '%s\n' "$pods_json" >"${LOG_DIR}/failure-pod-loss-pods.json"
			printf 'resilience:pod-loss timed out replacement=%s ready=%s desired=%s oldPresent=%s\n' "$replacement" "$ready" "$desired" "$old_present" >&2
			return 1
		fi
		sleep "$poll_interval"
	done
	if ! run_runtime_health_with_retry "${LOG_DIR}/cell-pod-loss-health.json"; then
		printf 'resilience:pod-loss runtime health failed after replacement pod became ready\n' >&2
		return 1
	fi
	if ! run_gcp_magento_known_content_check "${LOG_DIR}/cell-pod-loss-known-content.log"; then
		printf 'resilience:pod-loss Magento known-content check failed after replacement pod became ready\n' >&2
		return 1
	fi
	printf 'resilience:pod-loss ok deleted=%s replacement=%s ready=%s desired=%s\n' "$pod" "$replacement" "$ready" "$desired"
}

run_gcp_node_loss_cell() {
	local kc_path nodes_json ready_nodes_json ready_before ready_zones_before
	local node_record node_name node_provider_id node_provider_project node_provider_zone node_instance node_label_zone provider_path provider_extra
	local timeout_secs poll_interval elapsed ready_after ready_zones_after old_ready replacement
	local instance_json instance_listing instance_id_before instance_id_after instance_status_before instance_status_after instance_created_before instance_created_after
	local instance_lookup_ok instance_replaced node_identity_reused fault_started measured_rto
	if [[ "$PROFILE" != high-availability ]]; then
		printf 'resilience:node-loss requires the high-availability profile\n' >&2
		return 2
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1 || ! command -v gcloud >/dev/null 2>&1; then
		printf 'resilience:node-loss requires kubectl, jq, and gcloud on PATH\n' >&2
		return 2
	fi
	timeout_secs="${MAGELIFT_GCP_NODE_LOSS_TIMEOUT_SECS:-1200}"
	poll_interval="${MAGELIFT_GCP_NODE_LOSS_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:node-loss timeout and poll interval must be positive integers\n' >&2
		return 2
	fi
	kc_path="${LOG_DIR}/node-loss.kubeconfig"
	if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
		printf 'resilience:node-loss could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	if ! nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json)"; then
		printf 'resilience:node-loss could not list GKE nodes\n' >&2
		return 1
	fi
	printf '%s\n' "$nodes_json" >"${LOG_DIR}/cell-node-loss-nodes-before.json"
	ready_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length')"
	ready_zones_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length')"
	ready_nodes_json="$(printf '%s' "$nodes_json" | jq -c '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name] | sort')"
	if [[ ! "$ready_before" =~ ^[0-9]+$ || "$ready_before" -lt 3 || ! "$ready_zones_before" =~ ^[0-9]+$ || "$ready_zones_before" -lt 3 ]]; then
		printf 'resilience:node-loss requires at least three Ready nodes across three zones (ready=%s zones=%s)\n' "$ready_before" "$ready_zones_before" >&2
		return 1
	fi
	node_record="$(printf '%s' "$nodes_json" | jq -r '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | [.metadata.name, .spec.providerID, .metadata.labels["topology.kubernetes.io/zone"]]] | sort_by(.[0]) | .[0] | @tsv')"
	IFS=$'\t' read -r node_name node_provider_id node_label_zone <<<"$node_record"
	if [[ -z "$node_name" || -z "$node_provider_id" || -z "$node_label_zone" || "$node_provider_id" != gce://* ]]; then
		printf 'resilience:node-loss selected node has no supported GCE provider identity (node=%s)\n' "$node_name" >&2
		return 1
	fi
	provider_path="${node_provider_id#gce://}"
	IFS=/ read -r node_provider_project node_provider_zone node_instance provider_extra <<<"$provider_path"
	if [[ -n "${provider_extra:-}" || -z "$node_provider_project" || -z "$node_provider_zone" || -z "$node_instance" || "$node_provider_project" != "$PROJECT" || "$node_provider_zone" != "$node_label_zone" ]]; then
		printf 'resilience:node-loss selected node provider identity does not match project and zone (node=%s)\n' "$node_name" >&2
		return 1
	fi
	if [[ ! "$node_provider_zone" =~ ^[a-z][a-z0-9-]+[a-z0-9]$ || ! "$node_instance" =~ ^[a-z]([-a-z0-9]*[a-z0-9])?$ ]]; then
		printf 'resilience:node-loss selected provider identity contains an invalid zone or instance name\n' >&2
		return 1
	fi
	if ! instance_json="$(gcloud compute instances describe "$node_instance" --zone="$node_provider_zone" --project="$PROJECT" --format=json 2>/dev/null)"; then
		printf 'resilience:node-loss could not verify the exact backing VM before injection (node=%s instance=%s)\n' "$node_name" "$node_instance" >&2
		return 1
	fi
	instance_listing="$(printf '%s' "$instance_json" | jq -r '.name // empty')"
	instance_id_before="$(printf '%s' "$instance_json" | jq -r '.id // empty')"
	instance_created_before="$(printf '%s' "$instance_json" | jq -r '.creationTimestamp // empty')"
	instance_status_before="$(printf '%s' "$instance_json" | jq -r '.status // empty')"
	if [[ "$instance_listing" != "$node_instance" || ! "$instance_id_before" =~ ^[0-9]+$ || "$instance_status_before" != RUNNING ]]; then
		printf 'resilience:node-loss backing VM is not an exact running instance (node=%s instance=%s)\n' "$node_name" "$node_instance" >&2
		return 1
	fi
	jq -n \
		--arg name "$instance_listing" \
		--arg id "$instance_id_before" \
		--arg zone "$node_provider_zone" \
		--arg creationTimestamp "$instance_created_before" \
		'{name:$name, id:$id, zone:$zone, creationTimestamp:$creationTimestamp, status:"RUNNING"}' \
		>"${LOG_DIR}/cell-node-loss-instance-before.json"
	fault_started="$(date +%s)"
	printf '+ resilience:node-loss deleting backing VM=%s zone=%s node=%s\n' "$node_instance" "$node_provider_zone" "$node_name"
	if ! gcloud compute instances delete "$node_instance" --zone="$node_provider_zone" --project="$PROJECT" --quiet; then
		printf 'resilience:node-loss exact backing VM deletion failed (node=%s instance=%s)\n' "$node_name" "$node_instance" >&2
		return 1
	fi
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json 2>/dev/null || printf '{"items":[]}')"
		ready_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length' 2>/dev/null || printf '0')"
		ready_zones_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length' 2>/dev/null || printf '0')"
		old_ready="$(printf '%s' "$nodes_json" | jq -r --arg old "$node_name" '[.items[] | select(.metadata.name == $old) | .status.conditions[]? | select(.type == "Ready" and .status == "True")] | length' 2>/dev/null || printf '0')"
		replacement="$(printf '%s' "$nodes_json" | jq -r --arg old "$node_name" --arg old_provider "$node_provider_id" --argjson before "$ready_nodes_json" '
			.items[]
			| select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
			| select(.metadata.name != $old and .spec.providerID != $old_provider)
			| select(.metadata.name as $current | (any($before[]; . == $current) | not))
			| .metadata.name' 2>/dev/null | head -n 1)"
		instance_lookup_ok=1
		instance_json="$(gcloud compute instances describe "$node_instance" --zone="$node_provider_zone" --project="$PROJECT" --format=json 2>/dev/null)" || instance_lookup_ok=0
		instance_id_after=""
		instance_status_after=""
		instance_created_after=""
		if (( instance_lookup_ok == 1 )); then
			instance_id_after="$(printf '%s' "$instance_json" | jq -r '.id // empty' 2>/dev/null || true)"
			instance_status_after="$(printf '%s' "$instance_json" | jq -r '.status // empty' 2>/dev/null || true)"
			instance_created_after="$(printf '%s' "$instance_json" | jq -r '.creationTimestamp // empty' 2>/dev/null || true)"
		fi
		instance_replaced=0
		if (( instance_lookup_ok == 1 )) && [[ "$instance_id_after" =~ ^[0-9]+$ && "$instance_id_after" != "$instance_id_before" && "$instance_status_after" == RUNNING ]]; then
			instance_replaced=1
		fi
		# A GKE managed instance group can recreate the VM under the same
		# Kubernetes node name and provider URI. The Compute Engine numeric ID
		# is the authoritative replacement identity in that case; accept the
		# Ready node only after the exact backing VM has changed.
		if [[ -z "$replacement" && "$instance_replaced" == 1 && "$old_ready" =~ ^[1-9][0-9]*$ ]]; then
			replacement="$node_name"
		fi
		if [[ "$ready_after" =~ ^[0-9]+$ && "$ready_after" -ge "$ready_before" && "$ready_zones_after" =~ ^[0-9]+$ && "$ready_zones_after" -ge "$ready_zones_before" && -n "$replacement" && "$instance_replaced" == 1 ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf '%s\n' "$nodes_json" >"${LOG_DIR}/failure-node-loss-nodes.json"
			printf 'instanceLookup=%s\ninstanceIDBefore=%s\ninstanceIDAfter=%s\ninstanceStatusAfter=%s\ninstanceIDChanged=%s\n' \
				"$([[ "$instance_lookup_ok" == 1 ]] && printf verified || printf failed)" "$instance_id_before" "$instance_id_after" "$instance_status_after" "$instance_replaced" \
				>"${LOG_DIR}/failure-node-loss-instance.log"
			printf 'resilience:node-loss timed out replacement=%s ready=%s zones=%s oldReady=%s instanceIDChanged=%s\n' "$replacement" "$ready_after" "$ready_zones_after" "$old_ready" "$instance_replaced" >&2
			return 1
		fi
		sleep "$poll_interval"
	done
	printf '%s\n' "$nodes_json" >"${LOG_DIR}/cell-node-loss-nodes-after.json"
	jq -n \
		--arg name "$node_instance" \
		--arg id "$instance_id_after" \
		--arg zone "$node_provider_zone" \
		--arg creationTimestamp "$instance_created_after" \
		--arg status "$instance_status_after" \
		'{name:$name, id:$id, zone:$zone, creationTimestamp:$creationTimestamp, status:$status}' \
		>"${LOG_DIR}/cell-node-loss-instance-after.json"
	if ! run_runtime_health_with_retry "${LOG_DIR}/cell-node-loss-health.json"; then
		printf 'resilience:node-loss runtime health failed after node replacement\n' >&2
		return 1
	fi
	if ! run_gcp_queue_health_cell; then
		printf 'resilience:node-loss queue health failed after node replacement\n' >&2
		return 1
	fi
	if ! run_gcp_search_health_cell; then
		printf 'resilience:node-loss search health failed after node replacement\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento setup:db:status --no-ansi 2>&1 | tee "${LOG_DIR}/cell-node-loss-database.log"; then
		printf 'resilience:node-loss database schema read failed after node replacement\n' >&2
		return 1
	fi
	if ! run_gcp_magento_known_content_check "${LOG_DIR}/cell-node-loss-known-content.log"; then
		printf 'resilience:node-loss Magento known-content check failed after node replacement\n' >&2
		return 1
	fi
	measured_rto="$(( $(date +%s) - fault_started ))"
	node_identity_reused=false
	if [[ "$replacement" == "$node_name" ]]; then
		node_identity_reused=true
	fi
	printf 'traffic=verified\nqueue=verified\ndatabase=schema-read-verified\ncacheLoss=classified-not-injected-managed\nmeasuredRTOSeconds=%s\n' "$measured_rto" >"${LOG_DIR}/cell-node-loss-cache-classification.log"
	jq -n \
		--arg deletedNode "$node_name" \
		--arg deletedInstance "$node_instance" \
		--arg instanceIDBefore "$instance_id_before" \
		--arg instanceIDAfter "$instance_id_after" \
		--arg deletedZone "$node_provider_zone" \
		--arg replacementNode "$replacement" \
		--argjson nodeIdentityReused "$node_identity_reused" \
		--argjson readyBefore "$ready_before" \
		--argjson readyAfter "$ready_after" \
		--argjson zonesBefore "$ready_zones_before" \
		--argjson zonesAfter "$ready_zones_after" \
		--argjson measuredRTOSeconds "$measured_rto" \
		'{scenario:"resilience:node-loss", injection:"gce-instance-delete", injectionVerified:true, deletedNode:$deletedNode, deletedInstance:$deletedInstance, instanceIDBefore:$instanceIDBefore, instanceIDAfter:$instanceIDAfter, instanceIDChanged:true, deletedZone:$deletedZone, replacementNode:$replacementNode, nodeIdentityReused:$nodeIdentityReused, readyNodesBefore:$readyBefore, readyNodesAfter:$readyAfter, readyZonesBefore:$zonesBefore, readyZonesAfter:$zonesAfter, traffic:"verified", queue:"verified", database:"schema-read-verified", cacheLoss:"classified-not-injected-managed", measuredRTOSeconds:$measuredRTOSeconds}' \
		>"${LOG_DIR}/cell-node-loss-proof.json"
	printf 'resilience:node-loss PASS deletedNode=%s replacement=%s deletedZone=%s instanceIDChanged=verified nodeIdentityReused=%s measuredRTOSeconds=%s traffic=verified queue=verified database=schema-read-verified cacheLoss=classified-not-injected-managed\n' "$node_name" "$replacement" "$node_provider_zone" "$node_identity_reused" "$measured_rto"
}

run_gcp_zone_loss_cell() {
	local kc_path nodes_json ready_before ready_zones_before target_zone target_records target_count
	local target_record target_node_name target_provider_id target_label_zone target_provider_project target_provider_zone target_instance provider_path provider_extra
	local target_before_json='[]' injected_targets='[]' target_after_json='[]' proof_json
	local instance_json instance_listing instance_id_before instance_created_before instance_status_before
	local instance_id_after instance_created_after instance_status_after instance_lookup_ok
	local timeout_secs poll_interval elapsed fault_started measured_rto
	local ready_after ready_zones_after target_zone_ready_after all_replaced target_before_id
	local zone_degraded_observed=0 zone_degraded_observed_json=false
	if [[ "$PROFILE" != high-availability ]]; then
		printf 'resilience:zone-loss requires the high-availability profile\n' >&2
		return 2
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1 || ! command -v gcloud >/dev/null 2>&1; then
		printf 'resilience:zone-loss requires kubectl, jq, and gcloud on PATH\n' >&2
		return 2
	fi
	timeout_secs="${MAGELIFT_GCP_ZONE_LOSS_TIMEOUT_SECS:-1800}"
	poll_interval="${MAGELIFT_GCP_ZONE_LOSS_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:zone-loss timeout and poll interval must be positive integers\n' >&2
		return 2
	fi
	kc_path="${LOG_DIR}/zone-loss.kubeconfig"
	if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
		printf 'resilience:zone-loss could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	if ! nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json)"; then
		printf 'resilience:zone-loss could not list GKE nodes\n' >&2
		return 1
	fi
	printf '%s\n' "$nodes_json" >"${LOG_DIR}/cell-zone-loss-nodes-before.json"
	ready_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length')"
	ready_zones_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length')"
	if [[ ! "$ready_before" =~ ^[0-9]+$ || "$ready_before" -lt 3 || ! "$ready_zones_before" =~ ^[0-9]+$ || "$ready_zones_before" -lt 3 ]]; then
		printf 'resilience:zone-loss requires at least three Ready nodes across three zones (ready=%s zones=%s)\n' "$ready_before" "$ready_zones_before" >&2
		return 1
	fi
	target_zone="$(printf '%s' "$nodes_json" | jq -r '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | map(select(length > 0)) | unique | sort | .[0]')"
	if [[ -z "$target_zone" || "$target_zone" == null ]]; then
		printf 'resilience:zone-loss could not select a declared Ready zone\n' >&2
		return 1
	fi
	target_records="$(printf '%s' "$nodes_json" | jq -c --arg zone "$target_zone" '
		[.items[]
		 | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
		 | select(.metadata.labels["topology.kubernetes.io/zone"] == $zone)
		 | {nodeName:.metadata.name, providerID:.spec.providerID, zone:.metadata.labels["topology.kubernetes.io/zone"]}
		] | sort_by(.nodeName)')"
	target_count="$(printf '%s' "$target_records" | jq 'length')"
	if [[ ! "$target_count" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:zone-loss selected zone has no Ready nodes (zone=%s)\n' "$target_zone" >&2
		return 1
	fi

	# Verify every mutation target and its exact running GCE identity before
	# deleting any VM. The target list is the zone-loss boundary, not a broad
	# project-wide instance query.
	while IFS= read -r target_record; do
		target_node_name="$(printf '%s' "$target_record" | jq -r '.nodeName')"
		target_provider_id="$(printf '%s' "$target_record" | jq -r '.providerID')"
		target_label_zone="$(printf '%s' "$target_record" | jq -r '.zone')"
		if [[ -z "$target_node_name" || -z "$target_provider_id" || -z "$target_label_zone" || "$target_provider_id" != gce://* ]]; then
			printf 'resilience:zone-loss selected node has no supported GCE provider identity (node=%s)\n' "$target_node_name" >&2
			return 1
		fi
		provider_path="${target_provider_id#gce://}"
		provider_extra=""
		IFS=/ read -r target_provider_project target_provider_zone target_instance provider_extra <<<"$provider_path"
		if [[ -n "${provider_extra:-}" || -z "$target_provider_project" || -z "$target_provider_zone" || -z "$target_instance" || "$target_provider_project" != "$PROJECT" || "$target_provider_zone" != "$target_label_zone" ]]; then
			printf 'resilience:zone-loss selected node provider identity does not match project and zone (node=%s)\n' "$target_node_name" >&2
			return 1
		fi
		if [[ ! "$target_provider_zone" =~ ^[a-z][a-z0-9-]+[a-z0-9]$ || ! "$target_instance" =~ ^[a-z]([-a-z0-9]*[a-z0-9])?$ ]]; then
			printf 'resilience:zone-loss selected provider identity contains an invalid zone or instance name\n' >&2
			return 1
		fi
		if ! instance_json="$(gcloud compute instances describe "$target_instance" --zone="$target_provider_zone" --project="$PROJECT" --format=json 2>/dev/null)"; then
			printf 'resilience:zone-loss could not verify the exact backing VM before injection (node=%s instance=%s)\n' "$target_node_name" "$target_instance" >&2
			return 1
		fi
		instance_listing="$(printf '%s' "$instance_json" | jq -r '.name // empty')"
		instance_id_before="$(printf '%s' "$instance_json" | jq -r '.id // empty')"
		instance_created_before="$(printf '%s' "$instance_json" | jq -r '.creationTimestamp // empty')"
		instance_status_before="$(printf '%s' "$instance_json" | jq -r '.status // empty')"
		if [[ "$instance_listing" != "$target_instance" || ! "$instance_id_before" =~ ^[0-9]+$ || "$instance_status_before" != RUNNING ]]; then
			printf 'resilience:zone-loss backing VM is not an exact running instance (node=%s instance=%s)\n' "$target_node_name" "$target_instance" >&2
			return 1
		fi
		target_before_json="$(jq -cn \
			--argjson targets "$target_before_json" \
			--argjson target "$target_record" \
			--arg instanceName "$instance_listing" \
			--arg instanceIDBefore "$instance_id_before" \
			--arg creationTimestampBefore "$instance_created_before" \
			'$targets + [($target + {instanceName:$instanceName, instanceIDBefore:$instanceIDBefore, creationTimestampBefore:$creationTimestampBefore})]')"
	done < <(printf '%s' "$target_records" | jq -c '.[]')
	printf '%s\n' "$target_before_json" >"${LOG_DIR}/cell-zone-loss-targets-before.json"
	jq -n \
		--arg scenario "resilience:zone-loss" \
		--arg selectedZone "$target_zone" \
		--argjson targets "$target_before_json" \
		'{scenario:$scenario, selectedZone:$selectedZone, injection:"gce-instance-delete-all-ready-nodes-in-zone", state:"prepared", targets:$targets}' \
		>"${LOG_DIR}/cell-zone-loss-injection.json"

	fault_started="$(date +%s)"
	while IFS= read -r target_record; do
		target_instance="$(printf '%s' "$target_record" | jq -r '.instanceName')"
		target_provider_zone="$(printf '%s' "$target_record" | jq -r '.zone')"
		target_node_name="$(printf '%s' "$target_record" | jq -r '.nodeName')"
		printf '+ resilience:zone-loss deleting backing VM=%s zone=%s node=%s\n' "$target_instance" "$target_provider_zone" "$target_node_name"
		if ! gcloud compute instances delete "$target_instance" --zone="$target_provider_zone" --project="$PROJECT" --quiet; then
			jq -n \
				--arg scenario "resilience:zone-loss" \
				--arg selectedZone "$target_zone" \
				--argjson targets "$target_before_json" \
				--argjson injectedTargets "$injected_targets" \
				'{scenario:$scenario, selectedZone:$selectedZone, injection:"gce-instance-delete-all-ready-nodes-in-zone", state:"injection-partial", targets:$targets, injectedTargets:$injectedTargets}' \
				>"${LOG_DIR}/cell-zone-loss-injection.json"
			printf 'resilience:zone-loss exact backing VM deletion failed (node=%s instance=%s)\n' "$target_node_name" "$target_instance" >&2
			return 1
		fi
		injected_targets="$(jq -cn --argjson targets "$injected_targets" --argjson target "$target_record" '$targets + [$target]')"
		jq -n \
			--arg scenario "resilience:zone-loss" \
			--arg selectedZone "$target_zone" \
			--argjson targets "$target_before_json" \
			--argjson injectedTargets "$injected_targets" \
			'{scenario:$scenario, selectedZone:$selectedZone, injection:"gce-instance-delete-all-ready-nodes-in-zone", state:"injected", targets:$targets, injectedTargets:$injectedTargets}' \
			>"${LOG_DIR}/cell-zone-loss-injection.json"
	done < <(printf '%s' "$target_before_json" | jq -c '.[]')

	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json 2>/dev/null || printf '{"items":[]}')"
		ready_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length' 2>/dev/null || printf '0')"
		ready_zones_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length' 2>/dev/null || printf '0')"
		target_zone_ready_after="$(printf '%s' "$nodes_json" | jq --arg zone "$target_zone" '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | select(.metadata.labels["topology.kubernetes.io/zone"] == $zone)] | length' 2>/dev/null || printf '0')"
		if [[ "$target_zone_ready_after" == 0 ]]; then
			zone_degraded_observed=1
		fi
		all_replaced=1
		target_after_json='[]'
		while IFS= read -r target_record; do
			target_instance="$(printf '%s' "$target_record" | jq -r '.instanceName')"
			target_provider_zone="$(printf '%s' "$target_record" | jq -r '.zone')"
			target_before_id="$(printf '%s' "$target_record" | jq -r '.instanceIDBefore')"
			instance_lookup_ok=1
			instance_json="$(gcloud compute instances describe "$target_instance" --zone="$target_provider_zone" --project="$PROJECT" --format=json 2>/dev/null)" || instance_lookup_ok=0
			instance_id_after=""
			instance_created_after=""
			instance_status_after=""
			if (( instance_lookup_ok == 1 )); then
				instance_id_after="$(printf '%s' "$instance_json" | jq -r '.id // empty' 2>/dev/null || true)"
				instance_created_after="$(printf '%s' "$instance_json" | jq -r '.creationTimestamp // empty' 2>/dev/null || true)"
				instance_status_after="$(printf '%s' "$instance_json" | jq -r '.status // empty' 2>/dev/null || true)"
			fi
			if (( instance_lookup_ok != 1 )) || [[ ! "$instance_id_after" =~ ^[0-9]+$ || "$instance_id_after" == "$target_before_id" || "$instance_status_after" != RUNNING ]]; then
				all_replaced=0
			fi
			target_after_json="$(jq -cn \
				--argjson targets "$target_after_json" \
				--argjson target "$target_record" \
				--arg instanceIDAfter "$instance_id_after" \
				--arg creationTimestampAfter "$instance_created_after" \
				--arg statusAfter "$instance_status_after" \
				'$targets + [($target + {instanceIDAfter:$instanceIDAfter, creationTimestampAfter:$creationTimestampAfter, statusAfter:$statusAfter})]')"
		done < <(printf '%s' "$target_before_json" | jq -c '.[]')
		if [[ "$ready_after" =~ ^[0-9]+$ && "$ready_after" -ge "$ready_before" && "$ready_zones_after" =~ ^[0-9]+$ && "$ready_zones_after" -ge "$ready_zones_before" && "$target_zone_ready_after" =~ ^[0-9]+$ && "$target_zone_ready_after" -ge "$target_count" && "$all_replaced" == 1 ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf '%s\n' "$nodes_json" >"${LOG_DIR}/failure-zone-loss-nodes.json"
			printf '%s\n' "$target_after_json" >"${LOG_DIR}/failure-zone-loss-targets.json"
			printf 'resilience:zone-loss timed out selectedZone=%s targets=%s ready=%s zones=%s targetZoneReady=%s allReplaced=%s\n' "$target_zone" "$target_count" "$ready_after" "$ready_zones_after" "$target_zone_ready_after" "$all_replaced" >&2
			return 1
		fi
		sleep "$poll_interval"
	done
	printf '%s\n' "$nodes_json" >"${LOG_DIR}/cell-zone-loss-nodes-after.json"
	printf '%s\n' "$target_after_json" >"${LOG_DIR}/cell-zone-loss-targets-after.json"
	if ! run_runtime_health_with_retry "${LOG_DIR}/cell-zone-loss-health.json"; then
		printf 'resilience:zone-loss runtime health failed after selected-zone recovery\n' >&2
		return 1
	fi
	if ! run_gcp_queue_health_cell; then
		printf 'resilience:zone-loss queue health failed after selected-zone recovery\n' >&2
		return 1
	fi
	if ! run_gcp_search_health_cell; then
		printf 'resilience:zone-loss search health failed after selected-zone recovery\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento setup:db:status --no-ansi 2>&1 | tee "${LOG_DIR}/cell-zone-loss-database.log"; then
		printf 'resilience:zone-loss database schema read failed after selected-zone recovery\n' >&2
		return 1
	fi
	if ! run_gcp_magento_known_content_check "${LOG_DIR}/cell-zone-loss-known-content.log"; then
		printf 'resilience:zone-loss Magento known-content check failed after selected-zone recovery\n' >&2
		return 1
	fi
	measured_rto="$(( $(date +%s) - fault_started ))"
	if [[ "$zone_degraded_observed" == 1 ]]; then
		zone_degraded_observed_json=true
	fi
	jq -n \
		--arg selectedZone "$target_zone" \
		--argjson targetsBefore "$target_before_json" \
		--argjson targetsAfter "$target_after_json" \
		--argjson targetCount "$target_count" \
		--argjson readyBefore "$ready_before" \
		--argjson readyAfter "$ready_after" \
		--argjson zonesBefore "$ready_zones_before" \
		--argjson zonesAfter "$ready_zones_after" \
		--argjson targetZoneReadyAfter "$target_zone_ready_after" \
		--argjson zoneReadyGapObserved "$zone_degraded_observed_json" \
		--argjson measuredRTOSeconds "$measured_rto" \
		'{scenario:"resilience:zone-loss", injection:"gce-instance-delete-all-ready-nodes-in-zone", injectionVerified:true, selectedZone:$selectedZone, zoneReadyGapObserved:$zoneReadyGapObserved, targetsBefore:$targetsBefore, targetsAfter:$targetsAfter, targetCount:$targetCount, readyNodesBefore:$readyBefore, readyNodesAfter:$readyAfter, readyZonesBefore:$zonesBefore, readyZonesAfter:$zonesAfter, targetZoneReadyAfter:$targetZoneReadyAfter, traffic:"verified", queue:"verified", database:"schema-read-verified", cacheLoss:"classified-not-injected-managed", fencing:"not-injected-single-runtime", measuredRTOSeconds:$measuredRTOSeconds}' \
		>"${LOG_DIR}/cell-zone-loss-proof.json"
	proof_json="$(<"${LOG_DIR}/cell-zone-loss-proof.json")"
	jq -n \
		--arg scenario "resilience:zone-loss" \
		--arg selectedZone "$target_zone" \
		--argjson targets "$target_before_json" \
		--argjson injectedTargets "$injected_targets" \
		--argjson proof "$proof_json" \
		'{scenario:$scenario, selectedZone:$selectedZone, injection:"gce-instance-delete-all-ready-nodes-in-zone", state:"observed", targets:$targets, injectedTargets:$injectedTargets, proof:$proof}' \
		>"${LOG_DIR}/cell-zone-loss-injection.json"
	printf 'resilience:zone-loss PASS deletedZone=%s deletedInstances=%s zoneReadyGapObserved=%s recoveredReady=%s/%s recoveredZones=%s/%s measuredRTOSeconds=%s traffic=verified queue=verified database=schema-read-verified cacheLoss=classified-not-injected-managed fencing=not-injected-single-runtime\n' "$target_zone" "$target_count" "$zone_degraded_observed_json" "$ready_after" "$ready_before" "$ready_zones_after" "$ready_zones_before" "$measured_rto"
}

gcp_prepare_live_kubeconfig() {
	local kc="${LOG_DIR}/live.kubeconfig"
	mkdir -p "$LOG_DIR"
	if ! KUBECONFIG="$kc" gcloud container clusters get-credentials "$CLUSTER_NAME" \
		--region="$REGION" --project="$PROJECT" >"${LOG_DIR}/live-kubeconfig.log" 2>&1; then
		printf 'could not refresh GKE kubeconfig for %s (cluster may not exist yet)\n' "$CLUSTER_NAME" >&2
		return 1
	fi
	chmod 600 "$kc"
	export MAGELIFT_KUBECONFIG="$kc"
	printf '+ MAGELIFT_KUBECONFIG refreshed from gcloud get-credentials cluster=%s\n' "$CLUSTER_NAME"
}

gcp_native_edge_prepare_kubeconfig() {
	if [[ "$NATIVE_EDGE_ENABLED" != 1 ]]; then
		printf 'native GCP edge is not enabled\n' >&2
		return 2
	fi
	ORIGIN_KUBECONFIG="${LOG_DIR}/native-edge-kubeconfig"
	# Pulumi's kubeconfig output is a static bearer token. It expires during a
	# long ManagedCertificate propagation wait, so prefer GKE's exec-plugin
	# kubeconfig and keep the Pulumi output only as a compatibility fallback.
	if ! KUBECONFIG="$ORIGIN_KUBECONFIG" gcloud container clusters get-credentials "$CLUSTER_NAME" \
		--region="$REGION" --project="$PROJECT" >"${LOG_DIR}/native-edge-kubeconfig.log" 2>&1; then
		if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
			pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$ORIGIN_KUBECONFIG"; then
			printf 'native GCP edge could not prepare a cluster kubeconfig\n' >&2
			return 1
		fi
	fi
	chmod 600 "$ORIGIN_KUBECONFIG"
}

gcp_native_edge_wait_ingress_ip() {
	local timeout_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_INGRESS_TIMEOUT_SECS:-900}"
	local poll_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_POLL_SECS:-15}" elapsed address
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_secs" =~ ^[1-9][0-9]*$ ]]; then
		printf 'native edge ingress timeout and poll interval must be positive integers\n' >&2
		return 2
	fi
	# This function is used in command substitution. Keep progress output off
	# stdout so the caller receives only the validated IPv4 address.
	printf '+ native GCP edge waiting for Ingress address name=%s\n' "$ORIGIN_INGRESS_NAME" >&2
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_secs)); do
		address="$(kubectl --kubeconfig="$ORIGIN_KUBECONFIG" get ingress "$ORIGIN_INGRESS_NAME" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
		if [[ "$address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
			printf '%s' "$address"
			return 0
		fi
		if (( elapsed >= timeout_secs )); then
			kubectl --kubeconfig="$ORIGIN_KUBECONFIG" describe ingress "$ORIGIN_INGRESS_NAME" >"${LOG_DIR}/failure-native-edge-ingress.describe.txt" 2>&1 || true
			printf 'native GCP edge Ingress did not receive a public IPv4 address within %ss\n' "$timeout_secs" >&2
			return 1
		fi
		sleep "$poll_secs"
	done
}

gcp_native_edge_wait_managed_certificate() {
	local timeout_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_CERT_WAIT_SECONDS:-3600}"
	local poll_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_POLL_SECS:-15}" elapsed status domain_status last_status="" last_domain_status=""
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_secs" =~ ^[1-9][0-9]*$ || "$timeout_secs" -lt 60 ]]; then
		printf 'native edge managed certificate timeout must be an integer >= 60 and poll interval must be positive\n' >&2
		return 2
	fi
	printf '+ native GCP edge waiting up to %ss for ManagedCertificate=%s ACTIVE\n' "$timeout_secs" "$ORIGIN_CERTIFICATE_NAME"
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_secs)); do
		status="$(kubectl --kubeconfig="$ORIGIN_KUBECONFIG" get managedcertificate "$ORIGIN_CERTIFICATE_NAME" -o jsonpath='{.status.certificateStatus}' 2>/dev/null || true)"
		domain_status="$(kubectl --kubeconfig="$ORIGIN_KUBECONFIG" get managedcertificate "$ORIGIN_CERTIFICATE_NAME" -o jsonpath='{.status.domainStatus[0].status}' 2>/dev/null || true)"
		if [[ "$status" != "$last_status" || "$domain_status" != "$last_domain_status" ]]; then
			printf '+ native GCP edge ManagedCertificate status=%s domainStatus=%s\n' "${status:-pending}" "${domain_status:-pending}" >&2
			last_status="$status"
			last_domain_status="$domain_status"
		fi
		case "$status" in
		Active)
			return 0
			;;
		Provisioning|FailedNotVisible|"")
			;;
		*)
			kubectl --kubeconfig="$ORIGIN_KUBECONFIG" describe managedcertificate "$ORIGIN_CERTIFICATE_NAME" >"${LOG_DIR}/failure-native-edge-certificate.describe.txt" 2>&1 || true
			printf 'native GCP edge ManagedCertificate entered terminal status=%s\n' "$status" >&2
			return 1
			;;
		esac
		if (( elapsed >= timeout_secs )); then
			kubectl --kubeconfig="$ORIGIN_KUBECONFIG" describe managedcertificate "$ORIGIN_CERTIFICATE_NAME" >"${LOG_DIR}/failure-native-edge-certificate.describe.txt" 2>&1 || true
			printf 'native GCP edge ManagedCertificate did not become ACTIVE within %ss (last=%s)\n' "$timeout_secs" "${status:-pending}" >&2
			return 1
		fi
		sleep "$poll_secs"
	done
}

gcp_native_edge_wait_https() {
	local address="${1:?origin address required}" timeout_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_HTTPS_TIMEOUT_SECS:-600}"
	local poll_secs="${MAGELIFT_GCP_ACCEPTANCE_ORIGIN_POLL_SECS:-15}" elapsed status
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_secs" =~ ^[1-9][0-9]*$ ]]; then
		printf 'native edge HTTPS timeout and poll interval must be positive integers\n' >&2
		return 2
	fi
	printf '+ native GCP edge waiting for HTTPS origin domain=%s address=%s\n' "$ORIGIN_DOMAIN" "$address" >&2
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_secs)); do
		status="$(curl -sS -o /dev/null -w '%{http_code}' -A 'MageLift-origin-health/1' --max-time 20 --resolve "${ORIGIN_DOMAIN}:443:${address}" "https://${ORIGIN_DOMAIN}/" || true)"
		if [[ "$status" =~ ^[23][0-9][0-9]$ ]]; then
			printf '%s' "$status"
			return 0
		fi
		if (( elapsed >= timeout_secs )); then
			printf 'native GCP edge HTTPS origin did not return 2xx/3xx within %ss (last=%s)\n' "$timeout_secs" "${status:-none}" >&2
			return 1
		fi
		sleep "$poll_secs"
	done
}

gcp_native_edge_origin_title() {
	local address="${1:?origin address required}"
	LC_ALL=C curl -sS -A 'MageLift-origin-health/1' --max-time 20 --resolve "${ORIGIN_DOMAIN}:443:${address}" "https://${ORIGIN_DOMAIN}/" |
		LC_ALL=C tr '\n' ' ' |
		grep -oE '<title[^>]*>[^<]+' |
		head -1 |
		sed -E 's/<title[^>]*>//;s/^[[:space:]]+//;s/[[:space:]]+$//'
}

gcp_native_edge_set_home_page_title() {
	local title_b64
	title_b64="$(printf '%s' "$ORIGIN_TITLE" | base64 | tr -d '\n')"
	# The homepage has an explicit cms_page.title, so design/head/default_title
	# does not control its HTML title. Update only the disposable run's home page
	# through Magento's resource layer; the outer edge then verifies the same
	# rendered title through its public HTTPS path.
	# Keep the PHP program on one physical line because the exec transport
	# rejects command arguments containing literal newlines.
	run exec --service web -- env "MAGELIFT_EDGE_TITLE_B64=$title_b64" php -r '$title=base64_decode((string) getenv("MAGELIFT_EDGE_TITLE_B64"), true); if (!is_string($title) || $title === "") { fwrite(STDERR, "run-owned Magento title is empty after decoding\n"); exit(1); } $database=["host"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST"),"dbname"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME"),"username"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME"),"password"=>(string) getenv("MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD")]; foreach ($database as $key=>$value) { if ($value === "") { fwrite(STDERR, "Magento database environment is incomplete: ".$key."\n"); exit(1); } } $encoded=json_encode(["db"=>["connection"=>["default"=>$database]]], JSON_UNESCAPED_SLASHES); if (!is_string($encoded) || putenv("MAGENTO_DC__OVERRIDE=".$encoded) === false) { fwrite(STDERR, "could not set Magento deployment-config override\n"); exit(1); } require "/app/app/bootstrap.php"; $bootstrap=\Magento\Framework\App\Bootstrap::create(BP, $_SERVER); $objectManager=$bootstrap->getObjectManager(); $resource=$objectManager->get(\Magento\Framework\App\ResourceConnection::class); $connection=$resource->getConnection(); $table=$resource->getTableName("cms_page"); $count=(int) $connection->fetchOne("SELECT COUNT(*) FROM ".$connection->quoteIdentifier($table)." WHERE identifier = ?", ["home"]); if ($count !== 1) { fwrite(STDERR, "Magento home CMS page identifier is not unique\n"); exit(1); } $connection->update($table, ["title"=>$title], ["identifier = ?"=>"home"]);'
}

gcp_native_edge_prepare_origin() {
	local address
	if [[ "$NATIVE_EDGE_ENABLED" != 1 ]]; then
		return 0
	fi
	if ! gcp_native_edge_prepare_kubeconfig; then
		return 1
	fi
	address="$(gcp_native_edge_wait_ingress_ip)" || return 1
	ORIGIN_DNS_ADDRESS="$address"
	# Claim before the provider mutation so an EXIT failure after a partial
	# Cloudflare create still gets an ownership-scoped cleanup attempt.
	ORIGIN_DNS_CLAIMED=1
	if ! cloudflare_acceptance_dns_prepare_a "$ORIGIN_DOMAIN" "$address" "$ORIGIN_DNS_MARKER"; then
		printf 'native GCP edge could not prepare the run-owned Cloudflare A record\n' >&2
		return 1
	fi
	if ! cloudflare_acceptance_dns_wait_for_a "$ORIGIN_DOMAIN" "$address"; then
		printf 'native GCP edge Cloudflare A record did not converge\n' >&2
		return 1
	fi
	printf '+ native GCP edge origin control plane ready domain=%s address=%s\n' "$ORIGIN_DOMAIN" "$address"
}

gcp_native_edge_verify_origin_https() {
	if [[ "$NATIVE_EDGE_ENABLED" != 1 ]]; then
		return 0
	fi
	if [[ -z "$ORIGIN_DNS_ADDRESS" ]]; then
		printf 'native GCP edge HTTPS verification requires a prepared origin address\n' >&2
		return 1
	fi
	if ! gcp_native_edge_wait_managed_certificate; then
		return 1
	fi
	local https_status
	https_status="$(gcp_native_edge_wait_https "$ORIGIN_DNS_ADDRESS")" || return 1
	printf '+ native GCP edge origin HTTPS status=%s domain=%s address=%s\n' "$https_status" "$ORIGIN_DOMAIN" "$ORIGIN_DNS_ADDRESS"
}

run_gcp_edge_traffic_cell() {
	local edge_run_id edge_domain
	if [[ "$NATIVE_EDGE_ENABLED" != 1 ]]; then
		printf 'edge:traffic requires MAGELIFT_GCP_ACCEPTANCE_NATIVE_EDGE=1\n' >&2
		return 2
	fi
	if [[ -z "$ORIGIN_TITLE" ]]; then
		printf 'edge:traffic requires a non-empty Magento origin title\n' >&2
		return 2
	fi
	if ! gcp_native_edge_verify_origin_https; then
		printf 'edge:traffic requires a healthy HTTPS Magento origin\n' >&2
		return 1
	fi
	printf '+ edge:traffic setting a run-owned Magento title=%q\n' "$ORIGIN_TITLE"
	local origin_base_url="https://${ORIGIN_DOMAIN}/"
	printf '+ edge:traffic setting Magento canonical HTTPS URLs=%q\n' "$origin_base_url"
	if ! run exec --service web -- bin/magento --no-ansi config:set web/unsecure/base_url "$origin_base_url" 2>&1 | tee "${LOG_DIR}/cell-edge-origin-base-url-unsecure.log"; then
		printf 'edge:traffic could not set Magento web/unsecure/base_url\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi config:set web/secure/base_url "$origin_base_url" 2>&1 | tee "${LOG_DIR}/cell-edge-origin-base-url-secure.log"; then
		printf 'edge:traffic could not set Magento web/secure/base_url\n' >&2
		return 1
	fi
	if ! gcp_native_edge_set_home_page_title 2>&1 | tee "${LOG_DIR}/cell-edge-origin-title-set.log"; then
		printf 'edge:traffic could not set the Magento home-page title\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi cache:flush 2>&1 | tee "${LOG_DIR}/cell-edge-origin-cache-flush.log"; then
		printf 'edge:traffic could not flush Magento cache after setting origin title\n' >&2
		return 1
	fi
	local origin_title
	origin_title="$(gcp_native_edge_origin_title "$ORIGIN_DNS_ADDRESS" || true)"
	if [[ "$origin_title" != "$ORIGIN_TITLE" ]]; then
		printf 'edge:traffic direct origin title mismatch expected=%q got=%q\n' "$ORIGIN_TITLE" "${origin_title:-none}" >&2
		return 1
	fi
	printf '+ edge:traffic direct Magento origin identity verified title=%q domain=%s\n' "$origin_title" "$ORIGIN_DOMAIN"
	edge_run_id="$(printf '%s-edge' "${NAME}-${PROFILE}" | tr -cd 'A-Za-z0-9._-' | cut -c1-48)"
	edge_domain="${MAGELIFT_GCP_EDGE_DOMAIN:-ml-gcp-edge-${edge_run_id}.acourtiol.com}"
	if [[ "$edge_domain" == "$ORIGIN_DOMAIN" ]]; then
		printf 'edge:traffic outer edge domain must differ from the Magento origin domain\n' >&2
		return 2
	fi
	if ! MAGELIFT_GCP_EDGE_ACCEPTANCE=1 \
		MAGELIFT_GCP_PROJECT="$PROJECT" \
		MAGELIFT_GCP_EDGE_RUN_ID="$edge_run_id" \
		MAGELIFT_GCP_EDGE_DOMAIN="$edge_domain" \
		MAGELIFT_GCP_EDGE_ORIGIN_HOST="$ORIGIN_DOMAIN" \
		MAGELIFT_GCP_EDGE_ORIGIN_TITLE="$ORIGIN_TITLE" \
		MAGELIFT_GCP_EDGE_ALIAS_TRAFFIC=1 \
		MAGELIFT_GCP_EDGE_WAF=1 \
		MAGELIFT_GCP_EDGE_ARMOR_TRAFFIC=1 \
		bash "$ROOT/scripts/gcp-edge-acceptance-local.sh" 2>&1 | tee "${LOG_DIR}/cell-edge-traffic.log"; then
		printf 'edge:traffic outer GCP edge acceptance failed\n' >&2
		return 1
	fi
	printf 'edge:traffic PASS origin=%s title=%q publicEdge=%s traffic=alias-https-origin armor=blocked-403 cleanup=verified\n' "$ORIGIN_DOMAIN" "$ORIGIN_TITLE" "$edge_domain"
}

run_gcp_cell() {
	local cell="${1:?cell id required}"
	local url composer_val
	case "$cell" in
	edge:traffic)
		run_gcp_edge_traffic_cell
		;;
	bootstrap:wif)
		printf '+ cell bootstrap:wif Ensure + Act/STS proof\n'
		if ! run bootstrap --yes --github-owner "$GITHUB_OWNER" --github-repo "$GITHUB_REPO" \
			2>&1 | tee "${LOG_DIR}/cell-bootstrap-wif.json"; then
			printf 'bootstrap:wif Ensure failed\n' >&2
			return 1
		fi
		prove_wif_token_exchange
		;;
	composer:sm-write)
		printf '+ cell composer:sm-write Secret Manager write\n'
		composer_val="${MAGELIFT_GCP_COMPOSER_AUTH_FIXTURE:-{\"http-basic\":{\"repo.magento.com\":{\"username\":\"acceptance\",\"password\":\"acceptance\"}}}}"
		printf '%s' "$composer_val" | run secret set "$COMPOSER_SECRET_ID" --value-stdin \
			| tee "${LOG_DIR}/cell-composer-sm-write.json"
		;;
	composer:sm-read)
		printf '+ cell composer:sm-read gcp-secret-manager:// AccessSecretVersion\n'
		# Loud failure if SM cannot resolve; value never logged (T-07-12 / T-07-16).
		if ! run secret list 2>&1 | tee "${LOG_DIR}/cell-composer-sm-read-list.json"; then
			printf 'composer:sm-read secret list failed\n' >&2
			return 1
		fi
		gcloud secrets versions access latest --secret="$COMPOSER_SECRET_ID" --project="$PROJECT" >/dev/null
		printf '+ composer:sm-read ok uri=%s (value not logged)\n' "$COMPOSER_SM_URI"
		;;
	day2:secrets)
		run secret list | tee "${LOG_DIR}/cell-day2-secrets.json"
		;;
	day2:state)
		run state status | tee "${LOG_DIR}/cell-day2-state.json"
		;;
	day2:logs)
		if digest_is_placeholder; then
			printf 'day2:logs requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST\n' >&2
			return 1
		fi
		run logs --service web | tee "${LOG_DIR}/cell-day2-logs.json"
		;;
	day2:exec)
		if digest_is_placeholder; then
			printf 'day2:exec requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST\n' >&2
			return 1
		fi
		run_gcp_exec_cell
		;;
	day2:health)
		if digest_is_placeholder; then
			printf 'day2:health requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST\n' >&2
			return 1
		fi
		run_kube_runtime_health_with_retry "${LOG_DIR}/cell-day2-health.json"
		;;
	search:health)
		run_gcp_search_health_cell
		;;
	deploy:candidate)
		if digest_is_placeholder; then
			printf 'deploy:candidate requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST (migrate→cutover→health→record)\n' >&2
			return 1
		fi
		# Run the exact CLI inventory in a ready web pod before creating the
		# paid migration candidate. This catches an image/runtime contract
		# mismatch, such as a missing installed-state marker that hides
		# cache:flush, without spending another candidate retry cycle.
		# Resume after ~1h cannot exec: stack kubeconfig is a static OAuth
		# token. deploy below refreshes it. Native edge already uses
		# gcloud get-credentials for the same reason.
		if [[ "${MAGELIFT_GCP_ACCEPTANCE_RESUME:-0}" == "1" || "${MAGELIFT_GCP_ACCEPTANCE_RESUME:-0}" == "true" || "${MAGELIFT_GCP_SKIP_MAGENTO_CLI_PREFLIGHT:-0}" == "1" ]]; then
			printf '+ skipping Magento CLI preflight on resume (stack kubeconfig may be stale)\n' | tee "${LOG_DIR}/cell-magento-cli-preflight.log"
		else
			if ! run exec --service web -- bin/magento list --no-ansi 2>&1 | tee "${LOG_DIR}/cell-magento-cli-preflight.log"; then
				printf 'deploy:candidate Magento CLI preflight failed\n' >&2
				return 1
			fi
			if ! grep -Eq '(^|[[:space:]])cache:(clean|flush|status)([[:space:]]|$)' "${LOG_DIR}/cell-magento-cli-preflight.log"; then
				printf 'deploy:candidate Magento CLI preflight did not expose cache commands\n' >&2
				return 1
			fi
		fi
		# kube.Steps: migrate → cutover → health → record (shared Steps from Phase 6).
		if ! "$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json \
			deploy --yes --digest "$DIGEST" 2>&1 | tee "${LOG_DIR}/cell-deploy-candidate.json"; then
			printf 'deploy:candidate failed\n' >&2
			return 1
		fi
		if ! gcp_apply_magento_storefront_base_url; then
			printf 'deploy:candidate Magento storefront base_url failed\n' >&2
			return 1
		fi
		if ! run_runtime_health_with_retry "${LOG_DIR}/cell-deploy-health.json"; then
			printf 'deploy:candidate health failed\n' >&2
			return 1
		fi
		if ! verify_gcp_magento_version "${LOG_DIR}/cell-deploy-candidate-version.log"; then
			printf 'deploy:candidate Magento version check failed\n' >&2
			return 1
		fi
		if ! run_gcp_failed_deployment_check "${LOG_DIR}/cell-failed-deployment.log"; then
			printf 'deploy:candidate failed-deployment drill failed\n' >&2
			return 1
		fi
		if ! gcp_native_edge_verify_origin_https; then
			printf 'deploy:candidate native GCP edge origin HTTPS verification failed\n' >&2
			return 1
		fi
		;;
	deploy:repeat)
		if digest_is_placeholder; then
			printf 'deploy:repeat requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST (existing-stack redeploy→health→version)\n' >&2
			return 1
		fi
		# This is deliberately a normal deployment against the already-created
		# lifecycle. It proves the update path and must not recreate the stack.
		if ! "$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json \
			deploy --yes --digest "$DIGEST" 2>&1 | tee "${LOG_DIR}/cell-deploy-repeat.json"; then
			printf 'deploy:repeat failed\n' >&2
			return 1
		fi
		if ! run_runtime_health_with_retry "${LOG_DIR}/cell-deploy-repeat-health.json"; then
			printf 'deploy:repeat health failed\n' >&2
			return 1
		fi
		if ! verify_gcp_magento_version "${LOG_DIR}/cell-deploy-repeat-version.log"; then
			printf 'deploy:repeat Magento version check failed\n' >&2
			return 1
		fi
		;;
	queue:health)
		run_gcp_queue_health_cell
		;;
	resilience:pod-loss)
		run_gcp_pod_loss_cell
		;;
	resilience:node-loss)
		run_gcp_node_loss_cell
		;;
	resilience:zone-loss)
		run_gcp_zone_loss_cell
		;;
	migrate:dump)
		# D-03: after infra-only creation and before the Magento candidate; the
		# kube dumpimport runner reaches private Cloud SQL through a VPC-adjacent pod.
		# Config already has seedDump, but journal must be StatusRecorded (ADR 0010); # env create --dump can't re-run when the overlay already exists.
		# The application image has no mysql client; schedule a short-lived
		# mysql:8 client pod on the cluster VPC, then pipe the configured dump via RunnerKube.
		printf '+ cell migrate:dump init journal + kube mysql-client pod + env import-dump\n'
		mkdir -p "${WORKDIR}/.magelift/seed-dumps"
		python3 - "$WORKDIR" "$PROFILE" "$SEED_DUMP" <<'PY'
import json, pathlib, sys, datetime
root, env, dump = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3]
path = root / ".magelift" / "seed-dumps" / f"{env}.json"
path.write_text(json.dumps({
    "status": "recorded",
    "dumpPath": dump,
    "updatedAt": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
}, indent=2) + "\n")
print(f"seed dump journal recorded at {path}")
PY
		local kc_path db_host db_pass dump_pod
		kc_path="${LOG_DIR}/migrate-dump.kubeconfig"
		dump_pod="mlgcpwt-dumpimport-mysql"
		# Pulumi DIY backend holds decrypted secret outputs (CLI outputs redacts).
		if ! PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
			pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"; then
			printf 'migrate:dump could not read kubeconfig output\n' >&2
			return 1
		fi
		chmod 600 "$kc_path"
		if ! db_host="$(pulumi_cli stack output databaseWriter --stack "$(pulumi_fq_stack_ref)")"; then
			printf 'migrate:dump could not read database writer output\n' >&2
			return 1
		fi
		if ! db_pass="$(gcloud secrets versions access latest --secret="${NAME}-${PROFILE}-sql-db" --project="$PROJECT")"; then
			printf 'migrate:dump could not read database password secret\n' >&2
			return 1
		fi
		if ! command -v kubectl >/dev/null 2>&1; then
			printf 'migrate:dump requires kubectl on PATH\n' >&2
			return 1
		fi
		kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
		if ! kubectl --kubeconfig="$kc_path" run "$dump_pod" \
			--image=mysql:8.4 \
			--restart=Never \
			--command -- sleep 900; then
			printf 'migrate:dump could not start mysql client pod\n' >&2
			return 1
		fi
		if ! kubectl --kubeconfig="$kc_path" wait --for=condition=Ready "pod/$dump_pod" --timeout=180s; then
			printf 'migrate:dump mysql client pod did not become ready\n' >&2
			return 1
		fi
		if ! MAGELIFT_DUMPIMPORT_RUNNER=kube \
		MAGELIFT_DUMPIMPORT_HOST="$db_host" \
		MAGELIFT_DUMPIMPORT_USER=magento \
		MAGELIFT_DUMPIMPORT_PASSWORD="$db_pass" \
		MAGELIFT_DUMPIMPORT_DATABASE=magento \
		MAGELIFT_DUMPIMPORT_POD="$dump_pod" \
		MAGELIFT_DUMPIMPORT_KUBECONFIG="$kc_path" \
		"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --yes --output json \
			env import-dump "$PROFILE" 2>&1 | tee "${LOG_DIR}/cell-migrate-dump.json"; then
			printf 'migrate:dump import failed\n' >&2
			return 1
		fi
	if ! gcp_apply_magento_seed_probe "$kc_path" "$dump_pod" "$db_host" "$db_pass"; then
		kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
		printf 'migrate:dump Magento seed probe apply failed\n' >&2
		return 1
	fi
	kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
		;;
	cost:estimate)
		run cost | tee "${LOG_DIR}/cell-cost-estimate.json"
		;;
	cutover:dns)
		# D-04: DNS after applicationURL exists; Cloudflare Zone.DNS Edit token required.
		url="$(application_url_from_outputs)" || {
			printf 'cutover:dns requires applicationURL/ingress from stack outputs\n' >&2
			return 1
		}
		printf '+ cell cutover:dns host=%s target=%s\n' "$CUTOVER_HOST" "$url"
		TARGET="$url" MAGELIFT_CUTOVER_HOST="$CUTOVER_HOST" \
			"$ROOT/scripts/cutover-dns-cloudflare.sh" | tee "${LOG_DIR}/cell-cutover-dns.log"
		;;
	*)
		printf 'unsupported gcp acceptance cell: %s\n' "$cell" >&2
		return 2
		;;
	esac
}

live_cell_loop() {
	local cell provider account date_s duration started result rc session_mode cell_count
	gcp_acceptance_paths
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-gcp}"
	account="$(acceptance_account_id)"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure
	cell_count="$(jq '.cells | length' "$ACCEPTANCE_CHECKPOINT")"

	printf 'gcp acceptance live_cell_loop cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi

		printf 'acceptance cell-update cell=%s\n' "$cell" >&2
		if [[ "$cell_count" == 0 ]]; then
			session_mode=baseline
		else
			session_mode=reused
		fi
		export MAGELIFT_ACCEPTANCE_SESSION_MODE="$session_mode"
		started=$(date +%s)
		result=PASS
		rc=0
		# Stack update / day-2 only; never destroy+recreate between cells (D-02).
		if ! run_gcp_cell "$cell"; then
			result=FAIL
			rc=1
		fi

		duration="$(( $(date +%s) - started ))s"
		export MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS="${duration%s}"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		append_shared_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "$result"
		cell_count=$((cell_count + 1))
		printf 'acceptance cell-done cell=%s result=%s\n' "$cell" "$result" >&2
		if [[ "$rc" -ne 0 ]]; then
			if [[ "${MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE:-false}" == true ]]; then
				# Keep the paid stack only when resume was explicitly requested.
				export MAGELIFT_GCP_ACCEPTANCE_KEEP=true
				printf 'acceptance cell failed; KEEP=true; re-run with MAGELIFT_GCP_ACCEPTANCE_RESUME=1 (no destroy)\n' >&2
			else
				printf 'acceptance cell failed; cleanup will run on EXIT; set MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE=true to retain the stack for resume\n' >&2
			fi
			return 1
		fi
	done
}

# --- live main -------------------------------------------------------------------

gcp_acceptance_paths
if ! assert_wif_pool_name_fresh; then
	exit 2
fi
if ! ensure_acceptance_encryption_key; then
	exit 2
fi
run config validate | tee "${LOG_DIR}/validate.json"
reconcile_stale_pulumi_state 2>&1 | tee "${LOG_DIR}/reconcile.log"
reconcile_ec="${PIPESTATUS[0]}"
if [[ "$reconcile_ec" != 0 ]]; then
	exit "$reconcile_ec"
fi
run preview | tee "${LOG_DIR}/preview.json"

if [[ "$MODE" == up ]]; then
	printf 'WARNING: up creates GKE %s + Cloud SQL + Memorystore; destroy + force_clean run on EXIT\n' "$GKE_RUNTIME"
	if [[ "$INFRA_ONLY" == 1 ]]; then
		printf 'WARNING: INFRA_ONLY=1 proves infrastructure and managed-service cells only; it is not Magento application certification\n'
	else
		printf 'WARNING: MAGELIFT_GCP_ACCEPTANCE_DIGEST must be pullable for Magento cells (placeholder fails ImagePull)\n'
	fi
	created=1
	if ! ensure_gcp_adc && ! refresh_gcp_access_token; then
		printf 'ERROR: GCP credentials are unavailable; refusing to create acceptance resources\n' >&2
		exit 1
	fi

	if should_skip_create_once; then
		restore_acceptance_wif_ownership
		printf 'acceptance resume: skipping create-once (stack assumed present; KEEP/RESUME/checkpoint)\n'
		gcp_prepare_live_kubeconfig || true
	else
		printf 'acceptance create-once\n'
		printf '+ magelift bootstrap (GCS DIY state bucket + WIF)\n'
		if ! capture_acceptance_wif_ownership; then
			exit 1
		fi
		run bootstrap --yes --github-owner "$GITHUB_OWNER" --github-repo "$GITHUB_REPO" \
			2>&1 | tee "${LOG_DIR}/bootstrap.json"
		if [[ "$INFRA_ONLY" != 1 ]] && ! digest_is_placeholder; then
			printf '+ magelift promote (Cosign journal before Magento deploy:candidate)\n'
			acceptance_maybe_sign_digest "$DIGEST"
			acceptance_promote_digest "$DIGEST" "${MAGELIFT_GCP_CERTIFICATE_IDENTITY:-}" "${MAGELIFT_GCP_CERTIFICATE_OIDC_ISSUER:-}" \
				| tee "${LOG_DIR}/promote.json"
		fi
		# Infra-only create-once: Magento migrate/cutover live in deploy:candidate (and
		# day2:*) cells. A full deploy --digest here fails closed on health-image migrate
		# or redacted kubeconfig and the EXIT trap destroys the paid Autopilot stack.
		if digest_is_placeholder; then
			printf '+ magelift deploy --yes --infra-only (placeholder digest; Magento cells need pullable DIGEST)\n'
		else
			printf '+ magelift deploy --yes --infra-only (create-once; Magento via deploy:candidate cell)\n'
		fi
		"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json \
			deploy --yes --infra-only | tee "${LOG_DIR}/deploy.json"
		run outputs | tee "${LOG_DIR}/outputs.json"
		if [[ "$INFRA_ONLY" != 1 ]] && ! digest_is_placeholder; then
			if ! gcp_prepare_live_kubeconfig; then
				printf 'create-once needs MAGELIFT_KUBECONFIG for this cluster before day2:logs\n' >&2
				exit 1
			fi
		fi
		if ! gcp_native_edge_prepare_origin; then
			exit 2
		fi
		if ! verify_gcp_database_version; then
			exit 2
		fi
	fi

	live_cell_loop
fi

printf 'gcp acceptance ok mode=%s; destroy + force_clean + assert_clean on EXIT unless MAGELIFT_GCP_ACCEPTANCE_KEEP=true\n' "$MODE"

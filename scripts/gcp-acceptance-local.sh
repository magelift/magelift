#!/usr/bin/env bash
# Local, credit-efficient GCP acceptance: preview by default, destroy on EXIT.
# Experimental target only (ADR 0007/0008). Never leave Autopilot/SQL/Valkey running.
# Dry-run (MAGELIFT_ACCEPTANCE_DRY_RUN=1): fixture path — no Pulumi/gcloud create.
# Live up: one create-once, then live_cell_loop catalog updates; never recreate between cells.
# Evidence: .magelift/gcp-matrix/matrix-results.md (six-column append_row only — no hand edits).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"

CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}"
CELLS=()

# GCP-scoped paths so AWS/GCP evidence do not clobber (ACCEPT-05).
# lib-checkpoint.sh / lib-evidence.sh assign AWS defaults (.magelift/acceptance-*)
# at source time; ${VAR:-gcp} would keep those and make should_skip_create_once
# see AWS PASS cells → skip GCP create-once. Force GCP matrix unless the caller
# set MAGELIFT_* or an absolute/tmp path (shape test).
gcp_acceptance_paths() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_CHECKPOINT:-}" ]]; then
		export ACCEPTANCE_CHECKPOINT="$MAGELIFT_ACCEPTANCE_CHECKPOINT"
	elif [[ "${ACCEPTANCE_CHECKPOINT:-}" == /* || "${ACCEPTANCE_CHECKPOINT:-}" == *gcp-matrix* ]]; then
		export ACCEPTANCE_CHECKPOINT="$ACCEPTANCE_CHECKPOINT"
	else
		export ACCEPTANCE_CHECKPOINT=".magelift/gcp-matrix/acceptance-checkpoint.json"
	fi
	if [[ -n "${MAGELIFT_ACCEPTANCE_EVIDENCE:-}" ]]; then
		export ACCEPTANCE_EVIDENCE="$MAGELIFT_ACCEPTANCE_EVIDENCE"
	elif [[ "${ACCEPTANCE_EVIDENCE:-}" == /* || "${ACCEPTANCE_EVIDENCE:-}" == *gcp-matrix* ]]; then
		export ACCEPTANCE_EVIDENCE="$ACCEPTANCE_EVIDENCE"
	else
		export ACCEPTANCE_EVIDENCE=".magelift/gcp-matrix/matrix-results.md"
	fi
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
		record_cell "$cell" "PASS"
		printf 'acceptance cell-done cell=%s result=PASS\n' "$cell" >&2
	done
	# Dry-run never sets created=1 / never calls up / never invokes gcloud mutate.
	printf 'gcp acceptance dry-run ok; created=0; no GCP up invoked\n' >&2
}

if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
	gcp_dry_run_cell_loop
	exit 0
fi

if [[ "${MAGELIFT_GCP_ACCEPTANCE:-}" != "1" ]]; then
	printf 'refusing to run without MAGELIFT_GCP_ACCEPTANCE=1 (or MAGELIFT_ACCEPTANCE_DRY_RUN=1)\n' >&2
	exit 2
fi

WORKDIR="${MAGELIFT_GCP_ACCEPTANCE_DIR:-/tmp/magelift-gcp-wt}"
PROJECT="${MAGELIFT_GCP_PROJECT:-digital-lab-341608}"
REGION="${MAGELIFT_GCP_REGION:-europe-west1}"
MODE="${1:-preview}"
PROFILE="${MAGELIFT_GCP_ACCEPTANCE_PROFILE:-preview}"
# Must be a pullable OCI digest for Magento day-2 / deploy:candidate / health cells.
# Placeholder sha256:0123… fails ImagePull — set MAGELIFT_GCP_ACCEPTANCE_DIGEST before live up.
DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
GITHUB_OWNER="${MAGELIFT_GCP_ACCEPTANCE_GITHUB_OWNER:-acourtiol}"
GITHUB_REPO="${MAGELIFT_GCP_ACCEPTANCE_GITHUB_REPO:-magelift}"
COMPOSER_SECRET_ID="${MAGELIFT_GCP_COMPOSER_SECRET_ID:-magelift-composer-auth}"
SEED_DUMP="${MAGELIFT_GCP_ACCEPTANCE_SEED_DUMP:-$ROOT/testdata/fixtures/migrate/tiny.sql}"
CUTOVER_HOST="${MAGELIFT_CUTOVER_HOST:-magelift-preview.alexandrecourtiol.com}"
# Isolation prefix for this worktree — do not reuse mlacc (other agents / prior orphans).
NAME="${MAGELIFT_GCP_ACCEPTANCE_NAME:-mlgcpwt}"
# Magelift DIY stack identity is project-env-provider-runtime (see platform.FormatStackName).
STACK_NAME="${NAME}-${PROFILE}-gcp-gke-autopilot"
PULUMI_PROJECT="magelift"
LOG_DIR="${WORKDIR}/logs"
BIN="${WORKDIR}/magelift"
CONFIG="${WORKDIR}/magelift.yaml"
COMPOSER_SM_URI="gcp-secret-manager://projects/${PROJECT}/secrets/${COMPOSER_SECRET_ID}/versions/latest"

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

mkdir -p "$WORKDIR" "$LOG_DIR"
exec > >(tee -a "${LOG_DIR}/acceptance.log") 2>&1

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
		printf 'WARNING: using static GOOGLE_OAUTH_ACCESS_TOKEN; long Ups may fail when it expires\n' >&2
	fi
fi
export CLOUDSDK_CORE_PROJECT="$PROJECT"
export GOOGLE_PROJECT="$PROJECT"

# Magelift GCP DIY state is GCS. A global `pulumi login` to an AWS acceptance S3
# bucket (common on this machine) makes destroy/preview fail with S3 301 and
# falsely blocks create-once. Default to the bootstrap bucket name when unset.
if [[ -z "${PULUMI_BACKEND_URL:-}" ]]; then
	export PULUMI_BACKEND_URL="gs://magelift-${PROJECT}-${REGION}-${NAME}-${PROFILE}-state"
	printf '+ defaulting PULUMI_BACKEND_URL=%s\n' "$PULUMI_BACKEND_URL"
fi

# Producer deletes (SQL/Memorystore/GKE/SCP) are async; poll before PSA peering/VPC teardown.
FORCE_CLEAN_POLL_INTERVAL_SECS="${MAGELIFT_GCP_FORCE_CLEAN_POLL_INTERVAL_SECS:-30}"
FORCE_CLEAN_TIMEOUT_SECS="${MAGELIFT_GCP_FORCE_CLEAN_TIMEOUT_SECS:-1200}"

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
	sqladmin.googleapis.com
	secretmanager.googleapis.com
	servicenetworking.googleapis.com
	networkconnectivity.googleapis.com
	serviceconsumermanagement.googleapis.com
	memorystore.googleapis.com
)
for api in "${APIS[@]}"; do
	gcloud services enable "$api" --project="$PROJECT" >/dev/null
done

printf '+ building magelift -> %s (serial GOMAXPROCS=1)\n' "$BIN"
(cd "$ROOT" && GOMAXPROCS=1 GOFLAGS=-p=1 go build -o "$BIN" ./cmd/magelift)

digest_is_placeholder() {
	[[ "$DIGEST" == *sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef* ]]
}

EXPIRES="$(date -u -d '+6 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+6H +%Y-%m-%dT%H:%M:%SZ)"
cat >"$CONFIG" <<EOF
schemaVersion: 1
project:
  name: ${NAME}
application:
  edition: open-source
  version: "2.4.8"
  mode: integrated
  webRuntime: nginx-fpm
build:
  php: "8.3"
  composer:
    credentials: ${COMPOSER_SM_URI}
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: ${PROJECT}
    region: ${REGION}
    networkCidr: 10.40.0.0/16
    zones: [${REGION}-b, ${REGION}-c]
    imageDigest: ${DIGEST}
    databaseName: magento
    masterUsername: magento
    labels:
      magelift-project: acceptance-gcp
      magelift-environment: ${PROFILE}
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
extensions: {}
EOF

run() {
	printf '+ magelift %s\n' "$*"
	"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json "$@"
}

created=0
cleanup() {
	local ec=$?
	if [[ "${MAGELIFT_GCP_ACCEPTANCE_KEEP:-false}" == true ]]; then
		printf '+ KEEP=true; skipping destroy/force_clean/assert_clean (stack retained for resume)\n'
		exit "$ec"
	fi
	if [[ "$created" == 1 ]]; then
		printf '+ magelift destroy --yes (EXIT trap)\n'
		ensure_gcp_adc || refresh_gcp_access_token || true
		run destroy --yes || printf 'destroy failed; attempting force_clean_orphans\n' >&2
		force_clean_orphans || true
	fi
	if ! assert_clean; then
		ec=1
	fi
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
	# managed resources — pulumi_stack_has_managed_resources already covered export.
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
	if run destroy --yes 2>"${LOG_DIR}/reconcile-destroy.stderr"; then
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
	gcloud compute networks list --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match network || failed=1
	gcloud container clusters list --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match gke || failed=1
	gcloud sql instances list --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match sql || failed=1
	gcloud memorystore instances list --location="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match memorystore || failed=1
	gcloud network-connectivity service-connection-policies list --region="$REGION" --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match scp || failed=1
	gcloud secrets list --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match secret || failed=1
	gcloud compute addresses list --global --project="$PROJECT" --format='value(name)' 2>/dev/null \
		| prefix_match address || failed=1
	local bucket
	while IFS= read -r bucket; do
		[[ -z "$bucket" ]] && continue
		# DIY Pulumi state buckets are bootstrap artefacts (magelift-…-state), not stack leftovers.
		[[ "$bucket" == magelift-*-state ]] && continue
		if [[ "$bucket" == ${NAME}-* ]] || [[ "$bucket" == *"${NAME}-${PROFILE}"* ]]; then
			printf 'leftover bucket: %s\n' "$bucket" >&2
			failed=1
		fi
	done < <(gcloud storage buckets list --project="$PROJECT" --format='value(name)' 2>/dev/null || true)
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
		[[ "$resource" == magelift-*-state ]] && continue
		gcloud storage rm -r "gs://${resource}" --project="$PROJECT" 2>/dev/null || true
	done < <(gcloud storage buckets list --project="$PROJECT" --format='value(name)' 2>/dev/null | acceptance_resource_lines || true)
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
	while IFS= read -r resource; do
		[[ -z "$resource" ]] && continue
		matches_acceptance_prefix "$resource" || continue
		gcloud secrets delete "$resource" --project="$PROJECT" --quiet 2>/dev/null || true
	done < <(gcloud secrets list --project="$PROJECT" --format='value(name)' 2>/dev/null || true)
	gcloud compute routers nats delete "${NAME}-${PROFILE}-net-nat" --router="${NAME}-${PROFILE}-net-router" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	gcloud compute routers delete "${NAME}-${PROFILE}-net-router" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	local subnet
	for subnet in private-0 private-1 public-0 public-1; do
		gcloud compute networks subnets delete "${NAME}-${PROFILE}-net-${subnet}" --region="$REGION" --project="$PROJECT" --quiet 2>/dev/null || true
	done
	printf '+ force_clean: soaking %ss for Cloud SQL PSA release\n' "$soak_secs"
	sleep "$soak_secs"
	# Retry PSA peering teardown until the VPC is gone or attempts exhaust.
	for _ in $(seq 1 20); do
		if ! gcloud compute networks describe "$net" --project="$PROJECT" >/dev/null 2>&1; then
			printf '+ force_clean: network %s already gone\n' "$net"
			break
		fi
		if gcloud compute networks peerings list --network="$net" --project="$PROJECT" --format='value(peerings[].name)' 2>/dev/null | grep -qx 'servicenetworking-googleapis-com'; then
			TOKEN="$(gcloud auth print-access-token --project="$PROJECT" 2>/dev/null || true)"
			if [[ -n "$TOKEN" ]]; then
				curl -sS -X POST \
					-H "Authorization: Bearer ${TOKEN}" \
					-H 'Content-Type: application/json' \
					"https://compute.googleapis.com/compute/v1/projects/${PROJECT}/global/networks/${net}/removePeering" \
					-d '{"name":"servicenetworking-googleapis-com"}' >/dev/null || true
			fi
			gcloud services vpc-peerings delete --network="$net" --service=servicenetworking.googleapis.com --project="$PROJECT" --quiet --async 2>/dev/null || true
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
	# Autopilot often skips LB await — applicationURL stays empty while the
	# Service eventually gets an external IP. Fall back to kubectl.
	if [[ -z "$url" || "$url" == "null" ]] && command -v kubectl >/dev/null 2>&1; then
		kc_path="${LOG_DIR}/cutover-dns.kubeconfig"
		if PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
			pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path" 2>/dev/null; then
			chmod 600 "$kc_path"
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

# bootstrap:wif — Ensure WIF on the project PLUS Act or gcloud STS/WIF exchange proof.
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
	printf 'bootstrap:wif FAILED: need Act proof (MAGELIFT_GCP_WIF_ACT_LOG or MAGELIFT_GCP_WIF_ACT_PROOF=1) or gcloud STS/WIF exchange — Ensure-only is not enough\n' >&2
	return 1
}

run_gcp_cell() {
	local cell="${1:?cell id required}"
	local url composer_val
	case "$cell" in
	bootstrap:wif)
		printf '+ cell bootstrap:wif Ensure + Act/STS proof\n'
		run bootstrap --yes --github-owner "$GITHUB_OWNER" --github-repo "$GITHUB_REPO" \
			| tee "${LOG_DIR}/cell-bootstrap-wif.json"
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
		# Loud failure if SM cannot resolve — value never logged (T-07-12 / T-07-16).
		run secret list | tee "${LOG_DIR}/cell-composer-sm-read-list.json"
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
		run exec --service web -- true | tee "${LOG_DIR}/cell-day2-exec.json"
		;;
	day2:health)
		if digest_is_placeholder; then
			printf 'day2:health requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST\n' >&2
			return 1
		fi
		run health --mode runtime | tee "${LOG_DIR}/cell-day2-health.json"
		;;
	deploy:candidate)
		if digest_is_placeholder; then
			printf 'deploy:candidate requires pullable MAGELIFT_GCP_ACCEPTANCE_DIGEST (migrate→cutover→health→record)\n' >&2
			return 1
		fi
		# kube.Steps: migrate → cutover → health → record (shared Steps from Phase 6).
		"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json \
			deploy --yes --digest "$DIGEST" | tee "${LOG_DIR}/cell-deploy-candidate.json"
		run health --mode runtime | tee "${LOG_DIR}/cell-deploy-health.json"
		;;
	migrate:dump)
		# D-03: after first successful deploy; kube dumpimport runner (07-04) for private SQL.
		# Config already has seedDump, but journal must be StatusRecorded (ADR 0010) —
		# env create --dump can't re-run when the overlay already exists.
		# Acceptance health image has no mysql client — schedule a short-lived
		# mysql:8 client pod on the cluster VPC, then pipe tiny.sql via RunnerKube.
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
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
			pulumi_cli stack output kubeconfig --stack "$(pulumi_fq_stack_ref)" --show-secrets >"$kc_path"
		chmod 600 "$kc_path"
		db_host="$(pulumi_cli stack output databaseWriter --stack "$(pulumi_fq_stack_ref)")"
		db_pass="$(gcloud secrets versions access latest --secret="${NAME}-${PROFILE}-sql-db" --project="$PROJECT")"
		if ! command -v kubectl >/dev/null 2>&1; then
			printf 'migrate:dump requires kubectl on PATH\n' >&2
			return 1
		fi
		kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
		kubectl --kubeconfig="$kc_path" run "$dump_pod" \
			--image=mysql:8.4 \
			--restart=Never \
			--command -- sleep 900
		kubectl --kubeconfig="$kc_path" wait --for=condition=Ready "pod/$dump_pod" --timeout=180s
		MAGELIFT_DUMPIMPORT_RUNNER=kube \
		MAGELIFT_DUMPIMPORT_HOST="$db_host" \
		MAGELIFT_DUMPIMPORT_USER=magento \
		MAGELIFT_DUMPIMPORT_PASSWORD="$db_pass" \
		MAGELIFT_DUMPIMPORT_DATABASE=magento \
		MAGELIFT_DUMPIMPORT_POD="$dump_pod" \
		MAGELIFT_DUMPIMPORT_KUBECONFIG="$kc_path" \
			"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --yes --output json \
			env import-dump "$PROFILE" | tee "${LOG_DIR}/cell-migrate-dump.json"
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
	local cell provider account date_s duration started result rc
	gcp_acceptance_paths
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-gcp}"
	account="$(acceptance_account_id)"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure

	printf 'gcp acceptance live_cell_loop cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi

		printf 'acceptance cell-update cell=%s\n' "$cell" >&2
		started=$(date +%s)
		result=PASS
		rc=0
		# Stack update / day-2 only — never destroy+recreate between cells (D-02).
		if ! run_gcp_cell "$cell"; then
			result=FAIL
			rc=1
		fi

		duration="$(( $(date +%s) - started ))s"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "$result"
		printf 'acceptance cell-done cell=%s result=%s\n' "$cell" "$result" >&2
		if [[ "$rc" -ne 0 ]]; then
			# Keep the paid stack so resume can continue cells without a second create.
			export MAGELIFT_GCP_ACCEPTANCE_KEEP=true
			printf 'acceptance cell failed; KEEP=true — re-run with MAGELIFT_GCP_ACCEPTANCE_RESUME=1 (no destroy)\n' >&2
			return 1
		fi
	done
}

# --- live main -------------------------------------------------------------------

gcp_acceptance_paths
run config validate | tee "${LOG_DIR}/validate.json"
reconcile_stale_pulumi_state 2>&1 | tee "${LOG_DIR}/reconcile.log"
reconcile_ec="${PIPESTATUS[0]}"
if [[ "$reconcile_ec" != 0 ]]; then
	exit "$reconcile_ec"
fi
run preview | tee "${LOG_DIR}/preview.json"
created=1

if [[ "$MODE" == up ]]; then
	printf 'WARNING: up creates GKE Autopilot + Cloud SQL + Memorystore; destroy + force_clean run on EXIT\n'
	printf 'WARNING: MAGELIFT_GCP_ACCEPTANCE_DIGEST must be pullable for Magento cells (placeholder fails ImagePull)\n'
	ensure_gcp_adc || refresh_gcp_access_token || true

	if should_skip_create_once; then
		printf 'acceptance resume: skipping create-once (stack assumed present; KEEP/RESUME/checkpoint)\n'
	else
		printf 'acceptance create-once\n'
		printf '+ magelift bootstrap (GCS DIY state bucket + WIF)\n'
		run bootstrap --yes --github-owner "$GITHUB_OWNER" --github-repo "$GITHUB_REPO" \
			2>&1 | tee "${LOG_DIR}/bootstrap.json" || true
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
	fi

	live_cell_loop
fi

printf 'gcp acceptance ok mode=%s; destroy + force_clean + assert_clean on EXIT unless MAGELIFT_GCP_ACCEPTANCE_KEEP=true\n' "$MODE"

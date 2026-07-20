#!/usr/bin/env bash
# Local, credit-efficient GCP acceptance: preview by default, destroy on EXIT.
# Experimental target only (ADR 0007/0008). Never leave Autopilot/SQL/Valkey running.
set -Eeuo pipefail

if [[ "${MAGELIFT_GCP_ACCEPTANCE:-}" != "1" ]]; then
	printf 'refusing to run without MAGELIFT_GCP_ACCEPTANCE=1\n' >&2
	exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="${MAGELIFT_GCP_ACCEPTANCE_DIR:-/tmp/magelift-gcp-wt}"
PROJECT="${MAGELIFT_GCP_PROJECT:-digital-lab-341608}"
REGION="${MAGELIFT_GCP_REGION:-europe-west1}"
MODE="${1:-preview}"
PROFILE="${MAGELIFT_GCP_ACCEPTANCE_PROFILE:-preview}"
DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
# Isolation prefix for this worktree — do not reuse mlacc (other agents / prior orphans).
NAME="${MAGELIFT_GCP_ACCEPTANCE_NAME:-mlgcpwt}"
# Magelift DIY stack identity is project-env-provider-runtime (see platform.FormatStackName).
STACK_NAME="${NAME}-${PROFILE}-gcp-gke-autopilot"
PULUMI_PROJECT="magelift"
LOG_DIR="${WORKDIR}/logs"
BIN="${WORKDIR}/magelift"
CONFIG="${WORKDIR}/magelift.yaml"

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

printf '+ building magelift -> %s\n' "$BIN"
(cd "$ROOT" && go build -o "$BIN" ./cmd/magelift)

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
extensions: {}
EOF

run() {
	printf '+ magelift %s\n' "$*"
	"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json "$@"
}

created=0
cleanup() {
	local ec=$?
	if [[ "$created" == 1 && "${MAGELIFT_GCP_ACCEPTANCE_KEEP:-false}" != true ]]; then
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
	preview_raw="$(run preview 2>/dev/null)" || {
		printf 'preview_reports_stale_state: magelift preview failed; treating as stale\n' >&2
		return 0
	}
	if ! command -v jq >/dev/null 2>&1; then
		printf 'preview_reports_stale_state: jq missing; treating as stale\n' >&2
		return 0
	fi
	preview_json="$(magelift_json_from_output "$preview_raw")"
	if ! stale="$(printf '%s' "$preview_json" | jq -e '[.preview.changes[]? | select(.operation == "same" or .operation == "update" or .operation == "delete") | .count] | add // 0' 2>/dev/null)"; then
		printf 'preview_reports_stale_state: jq parse failed; treating as stale state\n' >&2
		return 0
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

run config validate | tee "${LOG_DIR}/validate.json"
reconcile_stale_pulumi_state 2>&1 | tee "${LOG_DIR}/reconcile.log"
reconcile_ec="${PIPESTATUS[0]}"
if [[ "$reconcile_ec" != 0 ]]; then
	exit "$reconcile_ec"
fi
run preview | tee "${LOG_DIR}/preview.json"
created=1

	if [[ "$MODE" == up ]]; then
	printf 'WARNING: up creates GKE Autopilot + Cloud SQL + Memorystore; destroy runs on EXIT\n'
	ensure_gcp_adc || refresh_gcp_access_token || true
	printf '+ magelift bootstrap (GCS DIY state bucket)\n'
	run bootstrap --yes 2>&1 | tee "${LOG_DIR}/bootstrap.json" || true
	# Placeholder digests cannot run Magento migrate; validate the production-shaped
	# infrastructure graph + outputs, then destroy. Full Magento suite needs a pullable digest.
	printf '+ magelift deploy --yes --infra-only\n'
	"$BIN" --config "$CONFIG" --env "$PROFILE" --no-interaction --output json deploy --yes --infra-only | tee "${LOG_DIR}/deploy.json"
	run outputs | tee "${LOG_DIR}/outputs.json"
fi

printf 'gcp acceptance ok mode=%s; destroy + assert_clean run on EXIT unless MAGELIFT_GCP_ACCEPTANCE_KEEP=true\n' "$MODE"

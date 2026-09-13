#!/usr/bin/env bash
# Shared experimental acceptance harness for OVH MKS and Scaleway Kapsule.
# The live path requires one explicit service shape. It refuses to replay a
# multi-cell catalog against one unchanged configuration.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROVIDER="${1:-}"
case "$PROVIDER" in
ovh|scaleway) ;;
*)
	printf 'usage: %s ovh|scaleway\n' "$0" >&2
	exit 2
;;
esac

# shellcheck source=acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-dependencies.sh
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-cosign.sh
source "$ROOT/scripts/acceptance/lib-cosign.sh"
# shellcheck source=acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

json_dependency_status=0
acceptance_campaign_serial_go
acceptance_require_json_yaml_tools || json_dependency_status=1

PROFILE="${MAGELIFT_ACCEPTANCE_PROFILE:-preview}"
CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-${PROVIDER}-${PROFILE}.txt}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}"
CELLS=()

dry_run_checkpoint_path() {
	local path="${1:?checkpoint path required}"
	case "$path" in
	*.json) printf '%s-dry-run.json' "${path%.json}" ;;
	*) printf '%s-dry-run' "$path" ;;
	esac
}

acceptance_paths() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_CHECKPOINT:-}" ]]; then
		export ACCEPTANCE_CHECKPOINT="$MAGELIFT_ACCEPTANCE_CHECKPOINT"
		if [[ "$DRY_RUN" == 1 || "$DRY_RUN" == true ]]; then
			export ACCEPTANCE_CHECKPOINT="$(dry_run_checkpoint_path "$ACCEPTANCE_CHECKPOINT")"
		fi
	elif [[ "${ACCEPTANCE_CHECKPOINT:-}" != /* && "${ACCEPTANCE_CHECKPOINT:-}" != *"${PROVIDER}-matrix"* ]]; then
		if [[ "$DRY_RUN" == 1 || "$DRY_RUN" == true ]]; then
			export ACCEPTANCE_CHECKPOINT=".magelift/${PROVIDER}-matrix-dry-run/acceptance-checkpoint.json"
		else
			export ACCEPTANCE_CHECKPOINT=".magelift/${PROVIDER}-matrix/acceptance-checkpoint.json"
		fi
	fi
	if [[ -n "${MAGELIFT_ACCEPTANCE_EVIDENCE:-}" ]]; then
		export ACCEPTANCE_EVIDENCE="$MAGELIFT_ACCEPTANCE_EVIDENCE"
	elif [[ "${ACCEPTANCE_EVIDENCE:-}" != /* && "${ACCEPTANCE_EVIDENCE:-}" != *"${PROVIDER}-matrix"* ]]; then
		export ACCEPTANCE_EVIDENCE=".magelift/${PROVIDER}-matrix/matrix-results.md"
	fi
}

load_cells() {
	local line
	CELLS=()
	if [[ ! -f "$CELL_CATALOG" ]]; then
		printf 'acceptance profile %s has no default cell catalog; set MAGELIFT_ACCEPTANCE_CELL_CATALOG explicitly: %s\n' "$PROFILE" "$CELL_CATALOG" >&2
		exit 2
	fi
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
		CELLS+=("$line")
	done <"$CELL_CATALOG"
	if [[ ${#CELLS[@]} -eq 0 ]]; then
		printf 'no cells in catalog: %s\n' "$CELL_CATALOG" >&2
		exit 2
	fi
}

provider_account() {
	case "$PROVIDER" in
	ovh) printf '%s' "${MAGELIFT_OVH_ACCEPTANCE_PROJECT:-dry-run}" ;;
	scaleway) printf '%s' "${MAGELIFT_SCALEWAY_ACCEPTANCE_PROJECT:-dry-run}" ;;
	esac
}

dry_run_cell_loop() {
	acceptance_paths
	local cell provider account date_s duration started
	provider="$PROVIDER"
	account="$(provider_account)"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure
	printf '%s acceptance dry-run start cells=%d catalog=%s\n' "$PROVIDER" "${#CELLS[@]}" "$CELL_CATALOG" >&2
	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
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
	printf '%s acceptance dry-run ok; no provider mutate invoked\n' "$PROVIDER" >&2
}

if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
	if (( json_dependency_status != 0 )); then
		exit 2
	fi
	dry_run_cell_loop
	exit 0
fi

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to a provider acceptance configuration file}"
if [[ "${MAGELIFT_K8S_ACCEPTANCE:-}" != 1 ]]; then
	printf 'refusing live %s acceptance without MAGELIFT_K8S_ACCEPTANCE=1\n' "$PROVIDER" >&2
	exit 2
fi

K8S_RUNTIME_HEALTH_ENABLED=0
case "${MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH:-false}" in
1|true|TRUE)
	K8S_RUNTIME_HEALTH_ENABLED=1
	;;
0|false|FALSE|"")
	;;
*)
	printf 'MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH must be true or false\n' >&2
	exit 2
	;;
esac

case "$PROVIDER" in
ovh)
	: "${MAGELIFT_OVH_ACCEPTANCE_PROJECT:?set the exact OVH cloud project ID}"
	: "${MAGELIFT_OVH_ACCEPTANCE_REGION:?set the OVH region used by the config}"
	: "${MAGELIFT_OVH_ACCEPTANCE_DIGEST:?set an immutable Magento image digest}"
	OVH_PROFILE="${MAGELIFT_OVH_PROFILE:-default}"
	PROJECT="$MAGELIFT_OVH_ACCEPTANCE_PROJECT"
	REGION="$MAGELIFT_OVH_ACCEPTANCE_REGION"
	DIGEST="$MAGELIFT_OVH_ACCEPTANCE_DIGEST"
	OWNER_PREFIX="${MAGELIFT_OVH_ACCEPTANCE_PREFIX:-}"
	;;
scaleway)
	: "${MAGELIFT_SCALEWAY_ACCEPTANCE_PROJECT:?set the exact Scaleway project ID}"
	: "${MAGELIFT_SCALEWAY_ACCEPTANCE_REGION:?set the Scaleway region used by the config}"
	: "${MAGELIFT_SCALEWAY_ACCEPTANCE_DIGEST:?set an immutable Magento image digest}"
	SCW_PROFILE="${MAGELIFT_SCALEWAY_PROFILE:-default}"
	PROJECT="$MAGELIFT_SCALEWAY_ACCEPTANCE_PROJECT"
	REGION="$MAGELIFT_SCALEWAY_ACCEPTANCE_REGION"
	DIGEST="$MAGELIFT_SCALEWAY_ACCEPTANCE_DIGEST"
	OWNER_PREFIX="${MAGELIFT_SCALEWAY_ACCEPTANCE_PREFIX:-}"
	export SCW_PROFILE
	;;
esac

k8s_certificate_identity=""
k8s_certificate_issuer=""
case "$PROVIDER" in
ovh)
	k8s_certificate_identity="${MAGELIFT_OVH_CERTIFICATE_IDENTITY:-}"
	k8s_certificate_issuer="${MAGELIFT_OVH_CERTIFICATE_OIDC_ISSUER:-}"
	;;
scaleway)
	k8s_certificate_identity="${MAGELIFT_SCALEWAY_CERTIFICATE_IDENTITY:-}"
	k8s_certificate_issuer="${MAGELIFT_SCALEWAY_CERTIFICATE_OIDC_ISSUER:-}"
	;;
esac

if [[ "$K8S_RUNTIME_HEALTH_ENABLED" == 1 ]]; then
	acceptance_require_certificate_identity "$k8s_certificate_identity" "$k8s_certificate_issuer"
fi

# The OVH CLI currently exposes the vRack private-network API but not the
# regular /network/private endpoint used by the Pulumi adapter. Keep this
# small signed reader here so acceptance can detect and remove a network left
# behind when a provider create fails before Pulumi records the resource.
ovh_config_value() {
	local config_file="$1" section="$2" key="$3"
	awk -v wanted_section="$section" -v wanted_key="$key" '
		/^\[/ { in_section=($0 == "[" wanted_section "]"); next }
		in_section && $0 ~ "^[[:space:]]*" wanted_key "[[:space:]]*=" {
			sub("^[[:space:]]*" wanted_key "[[:space:]]*=[[:space:]]*", "")
			sub("[[:space:]]*$", "")
			print
			exit
		}
	' "$config_file"
}

ovh_api_init() {
	if [[ -n "${OVH_API_BASE:-}" ]]; then
		return 0
	fi
	local config_file="${MAGELIFT_OVH_CONFIG_FILE:-${OVH_CONFIG_FILE:-$HOME/.ovh.conf}}"
	local profile="${OVH_PROFILE:-default}"
	local endpoint_profile
	if [[ ! -f "$config_file" ]]; then
		printf 'OVH API config file not found: %s\n' "$config_file" >&2
		return 1
	fi
	endpoint_profile="$(ovh_config_value "$config_file" "$profile" endpoint)"
	if [[ -n "$endpoint_profile" ]]; then
		profile="$endpoint_profile"
	fi
	OVH_API_APPLICATION_KEY="$(ovh_config_value "$config_file" "$profile" application_key)"
	OVH_API_APPLICATION_SECRET="$(ovh_config_value "$config_file" "$profile" application_secret)"
	OVH_API_CONSUMER_KEY="$(ovh_config_value "$config_file" "$profile" consumer_key)"
	if [[ -z "$OVH_API_APPLICATION_KEY" || -z "$OVH_API_APPLICATION_SECRET" || -z "$OVH_API_CONSUMER_KEY" ]]; then
		printf 'OVH API config profile %s is missing application, secret, or consumer credentials\n' "$profile" >&2
		return 1
	fi
	case "${MAGELIFT_OVH_API_BASE:-$profile}" in
	ovh-eu|eu|EU|default) OVH_API_BASE="https://eu.api.ovh.com/1.0" ;;
	ovh-ca|ca|CA) OVH_API_BASE="https://ca.api.ovh.com/1.0" ;;
	ovh-us|us|US) OVH_API_BASE="https://api.us.ovhcloud.com/1.0" ;;
	*) OVH_API_BASE="${MAGELIFT_OVH_API_BASE:-$profile}" ;;
	esac
	if [[ "$OVH_API_BASE" != https://* ]]; then
		printf 'unsupported OVH API endpoint profile %s; set MAGELIFT_OVH_API_BASE to an HTTPS API base\n' "$profile" >&2
		return 1
	fi
}

ovh_api_request() {
	local method="$1" request_path="$2" url timestamp signature_data signature
	ovh_api_init
	url="${OVH_API_BASE}${request_path}"
	timestamp="$(curl -fsS "${OVH_API_BASE}/auth/time" | tr -d '\n\r')"
	signature_data="${OVH_API_APPLICATION_SECRET}+${OVH_API_CONSUMER_KEY}+${method}+${url}++${timestamp}"
	signature="$(printf '%s' "$signature_data" | shasum -a 1 | awk '{print $1}')"
	curl -fsS -X "$method" \
		-H "X-Ovh-Application: ${OVH_API_APPLICATION_KEY}" \
		-H "X-Ovh-Consumer: ${OVH_API_CONSUMER_KEY}" \
		-H "X-Ovh-Timestamp: ${timestamp}" \
		-H "X-Ovh-Signature: \$1\$${signature}" \
		"$url"
}

if [[ -z "$OWNER_PREFIX" ]]; then
	printf 'set the exact unique ownership prefix for the live %s run\n' "$PROVIDER" >&2
	exit 2
fi
if ! acceptance_campaign_require_prefix "$PROVIDER" "$OWNER_PREFIX"; then
	exit 2
fi
if ! acceptance_campaign_require_isolated_backend "${MAGELIFT_K8S_ACCEPTANCE_BACKEND_URL:-}"; then
	exit 2
fi
if [[ ! "$DIGEST" =~ ^[^@[:space:]]+@sha256:[a-f0-9]{64}$ ]]; then
	printf 'refusing a mutable or malformed %s image reference\n' "$PROVIDER" >&2
	exit 2
fi

digest_is_placeholder() {
	[[ "$DIGEST" == *sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef* ]]
}

verify_acceptance_digest() {
	if digest_is_placeholder; then
		printf '%s acceptance requires a pullable OCI digest, not the placeholder\n' "$PROVIDER" >&2
		return 1
	fi
	if ! command -v docker >/dev/null 2>&1; then
		printf 'docker with Buildx is required to verify the %s acceptance image\n' "$PROVIDER" >&2
		return 1
	fi
	if ! docker buildx imagetools inspect "$DIGEST" >/dev/null 2>&1; then
		printf '%s acceptance image digest is not pullable: %s\n' "$PROVIDER" "$DIGEST" >&2
		return 1
	fi
}

if ! verify_acceptance_digest; then
	exit 2
fi

dependency_status="$json_dependency_status"
acceptance_require_commands docker pulumi openssl || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

case "$PROVIDER" in
ovh)
	acceptance_require_commands ovhcloud curl shasum || exit 2
	# The OVH CLI profile name is not the same thing as its endpoint alias.
	# Validate it before Pulumi can create any paid managed service.
	if ! ovhcloud --profile "$OVH_PROFILE" config profile show "$OVH_PROFILE" >/dev/null 2>&1; then
		printf 'OVH CLI profile %s is not configured; set MAGELIFT_OVH_PROFILE to an authenticated profile name (the endpoint alias is not a profile)\n' "$OVH_PROFILE" >&2
		exit 2
	fi
;;
scaleway) acceptance_require_commands scw || exit 2 ;;
esac

load_cells
if [[ ${#CELLS[@]} -ne 1 ]]; then
	printf 'refusing live %s matrix: the current adapter cannot rewrite service cells; provide a one-cell catalog for the exact config shape\n' "$PROVIDER" >&2
	exit 2
fi

LIVE_WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/magelift-${PROVIDER}-acceptance.XXXXXX")"
LIVE_CONFIG="$LIVE_WORKDIR/magelift.yaml"
cp "$MAGELIFT_CONFIG" "$LIVE_CONFIG"
if [[ -n "${MAGELIFT_K8S_ACCEPTANCE_BACKEND_URL:-}" ]]; then
	export PULUMI_BACKEND_URL="$MAGELIFT_K8S_ACCEPTANCE_BACKEND_URL"
else
	# Experimental targets do not bootstrap a managed state backend yet. Keep
	# acceptance state local and disposable instead of inheriting a user's
	# global Pulumi login or an unrelated provider backend.
	export PULUMI_BACKEND_URL="file://${LIVE_WORKDIR}/pulumi"
	mkdir -p "$LIVE_WORKDIR/pulumi"
fi
if [[ -z "${PULUMI_CONFIG_PASSPHRASE:-}" && -z "${PULUMI_CONFIG_PASSPHRASE_FILE:-}" ]]; then
	PULUMI_CONFIG_PASSPHRASE_FILE="$LIVE_WORKDIR/pulumi-passphrase"
	if ! openssl rand -hex 32 >"$PULUMI_CONFIG_PASSPHRASE_FILE"; then
		printf 'unable to create the temporary Pulumi passphrase file\n' >&2
		exit 2
	fi
	chmod 600 "$PULUMI_CONFIG_PASSPHRASE_FILE"
	export PULUMI_CONFIG_PASSPHRASE_FILE
fi
cleanup_workdir() {
	rm -rf "$LIVE_WORKDIR"
}

MAGELIFT_ACCEPTANCE_OWNER="magelift-acceptance-${ACCEPTANCE_EVIDENCE_RUN_ID}"
export MAGELIFT_ACCEPTANCE_OWNER
export MAGELIFT_ACCEPTANCE_ENVIRONMENT="$PROFILE"
apply_acceptance_labels() {
	case "$PROVIDER" in
	ovh)
		yq -i '.target.ovh.labels."magelift-acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
		yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.ovh.labels."magelift-acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
		;;
	scaleway)
		yq -i '.target.scaleway.labels."magelift-acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
		yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.scaleway.labels."magelift-acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
		;;
	esac
}

# A managed control plane and node pool can take longer than a short test
# window, but a disposable acceptance run still needs a finite expiry boundary.
# The EXIT cleanup and direct inventory assertion remain authoritative.
acceptance_prepare_lifecycle
if [[ -n "${MAGELIFT_ACCEPTANCE_ENVIRONMENT:-}" ]]; then
	yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].expiresAt = strenv(ACCEPTANCE_EXPIRES_AT)' "$LIVE_CONFIG"
fi

case "$PROVIDER" in
ovh)
provider_json() { ovhcloud --profile "$OVH_PROFILE" --output json cloud managed-kubernetes list --cloud-project "$PROJECT"; }
database_json() { ovhcloud --profile "$OVH_PROFILE" --output json cloud managed-database list --cloud-project "$PROJECT"; }
network_json() { ovh_api_request GET "/cloud/project/${PROJECT}/network/private"; }
loadbalancer_json() { ovhcloud --profile "$OVH_PROFILE" --output json cloud loadbalancer list --cloud-project "$PROJECT"; }
;;
scaleway)
provider_json() { scw --profile "$SCW_PROFILE" --output json k8s cluster list "project-id=$PROJECT" "region=$REGION"; }
database_json() { scw --profile "$SCW_PROFILE" --output json rdb instance list "project-id=$PROJECT" "region=$REGION"; }
cache_json() { scw --profile "$SCW_PROFILE" --output json redis cluster list "project-id=$PROJECT" "zone=all"; }
network_json() { scw --profile "$SCW_PROFILE" --output json vpc private-network list "project-id=$PROJECT" "region=all"; }
loadbalancer_json() { scw --profile "$SCW_PROFILE" --output json lb lb list "project-id=$PROJECT" "zone=all"; }
;;
esac

resource_names() {
	jq -r '.. | objects | .name? // .description? // empty'
}

assert_clean() {
	local failed=0 matches raw
	for source_name in cluster database cache network loadbalancer; do
		if [[ "$source_name" == cluster ]]; then
			if ! raw="$(provider_json)"; then
				printf 'unable to list %s cluster resources\n' "$PROVIDER" >&2
				return 1
			fi
		elif [[ "$source_name" == database ]]; then
			if ! raw="$(database_json)"; then
				printf 'unable to list %s database resources\n' "$PROVIDER" >&2
				return 1
			fi
		elif [[ "$source_name" == cache && "$PROVIDER" == scaleway ]]; then
			if ! raw="$(cache_json)"; then
				printf 'unable to list %s cache resources\n' "$PROVIDER" >&2
				return 1
			fi
		elif [[ "$source_name" == network && "$PROVIDER" == scaleway ]]; then
			if ! raw="$(network_json)"; then
				printf 'unable to list %s network resources\n' "$PROVIDER" >&2
				return 1
			fi
		elif [[ "$source_name" == network && "$PROVIDER" == ovh ]]; then
			if ! raw="$(network_json)"; then
				printf 'unable to list %s network resources\n' "$PROVIDER" >&2
				return 1
			fi
		elif [[ "$source_name" == loadbalancer ]]; then
			if ! raw="$(loadbalancer_json)"; then
				printf 'unable to list %s load balancer resources\n' "$PROVIDER" >&2
				return 1
			fi
		else
			continue
		fi
		if ! matches="$(printf '%s' "$raw" | resource_names | awk -v prefix="$OWNER_PREFIX" 'index($0, prefix) == 1 {print}')"; then
			printf 'unable to parse %s %s resource listing\n' "$PROVIDER" "$source_name" >&2
			return 1
		fi
		if [[ -n "$matches" ]]; then
			printf 'leftover %s resources with prefix %s: %s\n' "$source_name" "$OWNER_PREFIX" "$matches" >&2
			failed=1
		fi
	done
	if [[ "$failed" -ne 0 ]]; then
		printf '%s assert_clean FAILED\n' "$PROVIDER" >&2
		return 1
	fi
	printf '%s assert_clean ok\n' "$PROVIDER" >&2
}

remove_orphaned_ovh_networks() {
	[[ "$PROVIDER" == ovh ]] || return 0
	local raw network_id network_name region openstack_id subnets subnet_id subnet_name
	if ! raw="$(network_json)"; then
		printf 'unable to list OVH private networks for orphan cleanup\n' >&2
		return 1
	fi
	while IFS=$'\t' read -r network_id network_name; do
		[[ -n "$network_id" && -n "$network_name" ]] || continue
		while IFS=$'\t' read -r region openstack_id; do
			[[ -n "$region" && -n "$openstack_id" ]] || continue
			if ! subnets="$(ovh_api_request GET "/cloud/project/${PROJECT}/region/${region}/network/${openstack_id}/subnet")"; then
				printf 'unable to list OVH subnets for orphan network %s\n' "$network_name" >&2
				return 1
			fi
			while IFS=$'\t' read -r subnet_id subnet_name; do
				[[ -n "$subnet_id" && "$subnet_name" == "$OWNER_PREFIX"* ]] || continue
				if ! ovh_api_request DELETE "/cloud/project/${PROJECT}/region/${region}/network/${openstack_id}/subnet/${subnet_id}" >/dev/null; then
					printf 'unable to delete OVH orphan subnet %s\n' "$subnet_name" >&2
					return 1
				fi
			 done < <(printf '%s' "$subnets" | jq -r --arg prefix "$OWNER_PREFIX" '.[] | select((.name // "") | startswith($prefix)) | [.id, (.name // "")] | @tsv')
		done < <(printf '%s' "$raw" | jq -r --arg id "$network_id" '.[] | select(.id == $id) | .regions[]? | [.region, .openstackId] | @tsv')
		if ! ovh_api_request DELETE "/cloud/project/${PROJECT}/network/private/${network_id}" >/dev/null; then
			printf 'unable to delete OVH orphan network %s\n' "$network_name" >&2
			return 1
		fi
	done < <(printf '%s' "$raw" | jq -r --arg prefix "$OWNER_PREFIX" '.[] | select((.name // .description // "") | startswith($prefix)) | [.id, (.name // .description // "")] | @tsv')
}

wait_for_clean() {
	local timeout="${MAGELIFT_K8S_ACCEPTANCE_CLEANUP_TIMEOUT_SECS:-900}"
	local interval="${MAGELIFT_K8S_ACCEPTANCE_CLEANUP_INTERVAL_SECS:-15}"
	local started now
	started=$(date +%s)
	while ! assert_clean; do
		if [[ "$PROVIDER" == ovh ]]; then
			remove_orphaned_ovh_networks || true
		fi
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf '%s cleanup polling timed out after %ss\n' "$PROVIDER" "$timeout" >&2
			return 1
		fi
		sleep "$interval"
	done
}

config=("$MAGELIFT_BIN" --config "$LIVE_CONFIG" --env "$PROFILE" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

export MAGELIFT_ACCEPTANCE_RELEASE="$(yq -r '.application.version // "unknown"' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_RUNTIME="$(yq -r '.target.runtime // "unknown"' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_PRESET="$PROFILE"
export MAGELIFT_ACCEPTANCE_DIGEST="$DIGEST"
export MAGELIFT_ACCEPTANCE_PHP_VERSION="$(yq -r '.build.php // ""' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS="$(yq -r '(.build.extensions // ["intl", "pdo_mysql"]) | join(",")' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_COMPOSER_VERSION="$(yq -r '.build.composer.version // ""' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_DATABASE="$(yq -r '.target.'"$PROVIDER"'.databaseVersion // .target.'"$PROVIDER"'.databaseFlavor // "mysql"' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_SEARCH="disabled"
export MAGELIFT_ACCEPTANCE_QUEUE="database"
export MAGELIFT_ACCEPTANCE_CACHE="$(yq -r '.target.scaleway.cacheMode // "valkey"' "$LIVE_CONFIG")"
export MAGELIFT_ACCEPTANCE_WEB_CACHE="none"
export MAGELIFT_ACCEPTANCE_EDGE="$(yq -r '(.edge.externalProvider // .edge.nativeProvider // "none")' "$LIVE_CONFIG")"

run_runtime_health() {
	local timeout="${MAGELIFT_K8S_ACCEPTANCE_RUNTIME_TIMEOUT_SECS:-600}"
	local interval="${MAGELIFT_K8S_ACCEPTANCE_RUNTIME_INTERVAL_SECS:-15}"
	local started now
	started=$(date +%s)
	while true; do
		if run health --mode runtime; then
			return 0
		fi
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf '%s runtime health did not become ready after %ss\n' "$PROVIDER" "$timeout" >&2
			return 1
		fi
		printf '%s runtime health is not ready; retrying in %ss\n' "$PROVIDER" "$interval" >&2
		sleep "$interval"
	done
}

created=0
acceptance_cell=""
acceptance_cell_recorded=0
cleanup() {
	local ec=$?
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		MAGELIFT_ACCEPTANCE_CLEANUP_REASON="acceptance TTL expired; forced cleanup"
		MAGELIFT_K8S_ACCEPTANCE_KEEP=false
	fi
	if [[ "${MAGELIFT_K8S_ACCEPTANCE_KEEP:-false}" == true ]]; then
		MAGELIFT_ACCEPTANCE_CLEANUP_REASON="stack retention was explicitly requested" append_shared_cleanup "SKIP" "$OWNER_PREFIX" "$MAGELIFT_ACCEPTANCE_OWNER" ""
		printf '%s acceptance workdir retained at %s for explicit resume/debugging\n' "$PROVIDER" "$LIVE_WORKDIR" >&2
		exit "$ec"
	fi
	local cleanup_result=PASS
	if [[ "$created" == 1 ]]; then
		local destroy_failed=0
		if ! run destroy --yes --skip-lock; then
			destroy_failed=1
		fi
		if ! wait_for_clean; then
			cleanup_result=FAIL
			ec=1
		elif [[ "$destroy_failed" == 1 ]]; then
			# Provider APIs can reject the first subnet delete while managed
			# Kubernetes ports are still releasing. The exact-prefix scan below is
			# authoritative; retain the Pulumi error as context without marking a
			# clean account as dirty.
			MAGELIFT_ACCEPTANCE_CLEANUP_REASON="Pulumi destroy reported a transient provider error; bounded orphan cleanup later proved the ownership prefix empty"
		fi
	fi
	if [[ "$acceptance_cell_recorded" == 1 ]]; then
		if [[ "$cleanup_result" == PASS ]]; then
			if ! update_cell_cleanup_state "$acceptance_cell" complete; then
				cleanup_result=FAIL
				ec=1
				MAGELIFT_ACCEPTANCE_CLEANUP_REASON="provider cleanup passed but checkpoint cleanup state could not be finalized"
			fi
		else
			update_cell_cleanup_state "$acceptance_cell" pending || true
		fi
	fi
	append_shared_cleanup "$cleanup_result" "$OWNER_PREFIX" "$MAGELIFT_ACCEPTANCE_OWNER" ""
	cleanup_workdir
	exit "$ec"
}
trap cleanup EXIT

acceptance_paths
ACCEPTANCE_TTL_MARKER_FILE="$LIVE_WORKDIR/acceptance-ttl-expired"
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ACCEPTANCE_TTL_MARKER_FILE"
apply_acceptance_labels
run config validate
run doctor
run preview
# OVH and Scaleway do not expose a provider Bootstrap/login port yet. Their
# default live acceptance therefore proves the Pulumi graph and provider
# resources only. Runtime health is opt-in because --infra-only still creates
# the workload manifests, which require a Magento-compatible image rather than
# a generic infrastructure smoke image. The full kube.Steps deploy path remains
# covered by offline tests until DIY object state exists.
printf 'notice: %s live acceptance uses --infra-only; provider login/bootstrap and DIY state are not implemented yet\n' "$PROVIDER" >&2
cell="${CELLS[0]}"
acceptance_cell="$cell"
started=$(date +%s)
export MAGELIFT_ACCEPTANCE_CLEANUP_STATE=pending
created=1
if [[ "$K8S_RUNTIME_HEALTH_ENABLED" == 1 || -n "$(acceptance_certificate_identity "$k8s_certificate_identity")" ]]; then
	printf '+ magelift promote (Cosign journal before %s deploy)\n' "$PROVIDER" >&2
	acceptance_maybe_sign_digest "$DIGEST"
	acceptance_promote_digest "$DIGEST" "$k8s_certificate_identity" "$k8s_certificate_issuer"
fi
run deploy --digest "$DIGEST" --yes --infra-only
run outputs

result=PASS

if [[ "$K8S_RUNTIME_HEALTH_ENABLED" == 1 ]]; then
	if ! run_runtime_health; then
		result=FAIL
	fi

else
	MAGELIFT_ACCEPTANCE_SCENARIO="infra-only"
	MAGELIFT_ACCEPTANCE_REASON="live acceptance proved infrastructure lifecycle only; runtime health was not exercised because the shared --infra-only path requires a Magento-compatible immutable image"
	printf '%s infrastructure acceptance passed; runtime health not exercised (set MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH=true only with a Magento-compatible immutable image)\n' "$PROVIDER" >&2
fi
duration="$(( $(date +%s) - started ))s"
export MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS="$duration"
date_s=$(date -u +%Y-%m-%d)
append_row "$cell" "$result" "$duration" "$PROVIDER" "$(provider_account)" "$date_s"
append_shared_row "$cell" "$result" "$duration" "$PROVIDER" "$(provider_account)" "$date_s"
record_cell "$cell" "$result"
acceptance_cell_recorded=1
if [[ "$result" != PASS ]]; then
	exit 1
fi

printf '%s acceptance complete; destroy and exact-prefix cleanup run on EXIT\n' "$PROVIDER" >&2

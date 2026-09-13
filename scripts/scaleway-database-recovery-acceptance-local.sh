#!/usr/bin/env bash
# Cost-bounded Scaleway Managed Database recovery acceptance. The wrapper owns
# one generated encrypted Block Storage RDB source; the Go cell owns the
# provider snapshot, isolated restore, polling, and output cleanup. The trap
# always removes only the exact marker-owned restore/snapshot identities before
# deleting the generated source.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-cleanup-ledger.sh
source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands scw go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${SCW_PROFILE:-default}"
project="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_PROJECT:-${SCW_PROJECT_ID:-}}"
region="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_REGION:-fr-par}"
run_id="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
source_name="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_INSTANCE_NAME:-magelift-rdb-${run_id}}"
marker="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_MARKER:-magelift/scaleway/rdb-recovery/${run_id}}"
fixture="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_FIXTURE:-fixture-rdb-control-plane-${run_id}}"
node_type="${MAGELIFT_SCALEWAY_DATABASE_RECOVERY_NODE_TYPE:-DB-DEV-S}"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^(fr-par|nl-ams|pl-waw)$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$source_name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$node_type" =~ ^[A-Za-z0-9._-]{1,64}$ ]]; then
	printf 'set valid Scaleway profile, region, run ID, generated name, marker, fixture, and node type values\n' >&2
	exit 2
fi

export SCW_PROFILE="$profile"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'Scaleway Managed Database recovery acceptance dry-run ok profile=%s project=%s region=%s sourceName=%s nodeType=%s marker=%s; no Scaleway mutation invoked\n' \
		"$profile" "${project:-unset}" "$region" "$source_name" "$node_type" "$marker"
	exit 0
fi

if [[ ! "$project" =~ ^[0-9a-fA-F-]{36}$ ]]; then
	printf 'set a Scaleway project UUID (MAGELIFT_SCALEWAY_DATABASE_RECOVERY_PROJECT or SCW_PROJECT_ID)\n' >&2
	exit 2
fi
export SCW_PROJECT_ID="$project"

export MAGELIFT_CLEANUP_LEDGER_DIR="${MAGELIFT_CLEANUP_LEDGER_DIR:-$ROOT/.magelift/cleanup}"
export MAGELIFT_CLEANUP_RUN_ID="$run_id"
export MAGELIFT_CLEANUP_MARKER="$marker"
export MAGELIFT_CLEANUP_PROVIDER=scaleway
export MAGELIFT_CLEANUP_REGION="$region"
export MAGELIFT_CLEANUP_PROJECT="$project"
export MAGELIFT_CLEANUP_PROFILE="$profile"
ledger_path="$(acceptance_cleanup_ledger_path "$run_id")"

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-7200}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-scaleway-rdb-recovery-${run_id}-ttl-expired-$$"

source_id=""
root_password=""
source_claimed=0
create_stdout_file=""
create_stderr_file=""

redact_provider_output() {
	sed -E \
		-e 's/Your generated password is .*/Your generated password is [REDACTED]/' \
		-e 's/("password"[[:space:]]*:[[:space:]]*")[^"]*(")/\1[REDACTED]\2/g'
}

extract_generated_password() {
	local file candidate
	for file in "$@"; do
		[[ -f "$file" ]] || continue
		candidate="$(sed -nE 's/.*Your generated password is:?[[:space:]]+([^[:space:]]+).*/\1/p' "$file" | tail -n1)"
		if [[ -z "$candidate" ]]; then
			candidate="$(sed -n '/^{/,$p' "$file" | jq -r '.password // empty' 2>/dev/null | tail -n1 || true)"
		fi
		if [[ -n "$candidate" && ! "$candidate" =~ [[:space:][:cntrl:]] && ${#candidate} -le 128 ]]; then
			printf '%s' "$candidate"
			return 0
		fi
	done
	return 1
}

list_instances() {
	scw --profile "$profile" --output json rdb instance list "region=$region" "project-id=$project"
}

list_snapshots() {
	scw --profile "$profile" --output json rdb snapshot list "region=$region" "project-id=$project"
}

delete_owned_outputs() {
	local instances_json snapshots_json id name
	instances_json="$(list_instances 2>/dev/null)" || return 1
	while IFS=$'\t' read -r id name; do
		[[ -z "$id" || "$id" == "$source_id" || "$name" == "$source_name" ]] && continue
		if ! scw --profile "$profile" rdb instance delete "$id" "region=$region" >/dev/null 2>&1; then
			if ! scw --profile "$profile" --output json rdb instance get "$id" "region=$region" >/dev/null 2>&1; then
				continue
			fi
			printf 'owned Scaleway RDB restore deletion failed id=%s\n' "$id" >&2
			return 1
		fi
	done < <(jq -r --arg marker "$marker" '
		.[]? |
		select(any((.tags // [])[]?; . == ("magelift.io/ownership=" + $marker))) |
		[ (.id // ""), (.name // "") ] | @tsv
	' <<<"$instances_json")

	if [[ -z "$source_id" ]]; then
		return 0
	fi
	snapshots_json="$(list_snapshots 2>/dev/null)" || return 1
	while IFS= read -r id; do
		[[ -z "$id" ]] && continue
		if ! scw --profile "$profile" rdb snapshot delete "$id" "region=$region" >/dev/null 2>&1; then
			if ! scw --profile "$profile" --output json rdb snapshot get "$id" "region=$region" >/dev/null 2>&1; then
				continue
			fi
			printf 'generated Scaleway RDB snapshot deletion failed id=%s\n' "$id" >&2
			return 1
		fi
	done < <(jq -r --arg source "$source_id" '.[]? | select((.instance_id // "") == $source and ((.name // "") | startswith("magelift-recovery-"))) | (.id // "")' <<<"$snapshots_json")
}

wait_for_outputs_absent() {
	local attempt instances_json snapshots_json
	for attempt in {1..90}; do
		instances_json="$(list_instances 2>/dev/null)" || return 1
		snapshots_json="$(list_snapshots 2>/dev/null)" || return 1
		if [[ "$(jq -r --arg marker "$marker" --arg source "$source_id" --arg source_name "$source_name" '[.[]? | select(any((.tags // [])[]?; . == ("magelift.io/ownership=" + $marker))) | select((.id // "") != $source and (.name // "") != $source_name)] | length' <<<"$instances_json")" == 0 && "$(jq -r --arg source "$source_id" '[.[]? | select((.instance_id // "") == $source and ((.name // "") | startswith("magelift-recovery-")))] | length' <<<"$snapshots_json")" == 0 ]]; then
			return 0
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 5
		fi
	done
	printf 'Scaleway RDB recovery outputs did not reach zero before cleanup timeout marker=%s\n' "$marker" >&2
	return 1
}

delete_source() {
	local attempt instances_json source_json id tags
	for attempt in {1..90}; do
		instances_json="$(list_instances 2>/dev/null)" || return 1
		source_json="$(jq -c --arg source "$source_id" --arg source_name "$source_name" '.[]? | select((.id // "") == $source or (.name // "") == $source_name)' <<<"$instances_json" | head -n1)"
		if [[ -z "$source_json" ]]; then
			return 0
		fi
		id="$(jq -r '.id // empty' <<<"$source_json")"
		tags="$(jq -r '.tags // [] | .[]' <<<"$source_json")"
		if ! grep -Fqx "magelift.io/ownership=$marker" <<<"$tags"; then
			printf 'refusing to delete Scaleway RDB source without the exact ownership marker id=%s\n' "$id" >&2
			return 1
		fi
		if ! scw --profile "$profile" rdb instance delete "$id" "region=$region" >/dev/null 2>&1; then
			if scw --profile "$profile" --output json rdb instance get "$id" "region=$region" >/dev/null 2>&1; then
				printf 'generated Scaleway RDB source deletion failed id=%s\n' "$id" >&2
				return 1
			fi
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 5
		fi
	done
	printf 'generated Scaleway RDB source did not reach terminal absence before cleanup timeout name=%s\n' "$source_name" >&2
	return 1
}

cleanup() {
	local exit_status=$?
	local cleanup_status=0
	set +e
	acceptance_stop_ttl_watchdog || true
	if [[ -n "$create_stdout_file" ]]; then
		rm -f "$create_stdout_file"
	fi
	if [[ -n "$create_stderr_file" ]]; then
		rm -f "$create_stderr_file"
	fi
	if acceptance_ttl_expired; then
		printf 'Scaleway Managed Database recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if (( source_claimed == 1 )) || [[ -f "${ledger_path:-}" ]]; then
		delete_owned_outputs || cleanup_status=1
		wait_for_outputs_absent || cleanup_status=1
		delete_source || cleanup_status=1
	fi
	if (( cleanup_status != 0 )); then
		exit_status=1
	fi
	if (( exit_status == 0 )); then
		printf 'Scaleway Managed Database recovery acceptance source and outputs were cleaned through exact ownership marker=%s\n' "$marker"
	else
		printf 'Scaleway Managed Database recovery acceptance failed; exact cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"
acceptance_cleanup_reconcile_pending "$MAGELIFT_CLEANUP_LEDGER_DIR"

existing_instances="$(list_instances)"
if jq -e --arg source_name "$source_name" --arg marker "$marker" 'any(.[]?; (.name // "") == $source_name or any((.tags // [])[]?; . == ("magelift.io/ownership=" + $marker)))' <<<"$existing_instances" >/dev/null; then
	printf 'generated Scaleway RDB source already exists or the marker is already owned; refusing to adopt it: %s\n' "$source_name" >&2
	exit 2
fi

acceptance_cleanup_ledger_claim "$ledger_path" rdb-instance source "$source_name" 30

printf '+ scaleway-rdb: creating encrypted source name=%s nodeType=%s region=%s\n' "$source_name" "$node_type" "$region"

create_stdout_file="$(mktemp "${TMPDIR:-/tmp}/magelift-scaleway-rdb-create-stdout.XXXXXX")"
create_stderr_file="$(mktemp "${TMPDIR:-/tmp}/magelift-scaleway-rdb-create-stderr.XXXXXX")"
if ! scw --profile "$profile" --output json rdb instance create \
	"project-id=$project" "region=$region" "name=$source_name" engine=MySQL-8 user-name=magelift generate-password=true "node-type=$node_type" \
	volume-type=sbs_5k volume-size=30G encryption.enabled=true disable-backup=true backup-same-region=true \
	"tags.0=magelift.io/ownership=$marker" "tags.1=magelift.io/data-class=database" "tags.2=magelift.io/fixture=$fixture" --wait >"$create_stdout_file" 2>"$create_stderr_file"; then
	printf 'Scaleway RDB source creation failed; provider response follows:\n' >&2
	redact_provider_output <"$create_stderr_file" >&2 || true
	redact_provider_output <"$create_stdout_file" >&2 || true
	exit 1
fi
source_claimed=1
source_id="$(sed -n '/^{/,$p' "$create_stdout_file" | jq -r '.id // empty' 2>/dev/null || true)"
if ! root_password="$(extract_generated_password "$create_stdout_file" "$create_stderr_file")"; then
	printf 'Scaleway RDB create did not return a usable generated password; refusing to run an unverified application fixture\n' >&2
	exit 1
fi
rm -f "$create_stdout_file" "$create_stderr_file"
create_stdout_file=""
create_stderr_file=""
if [[ -z "$source_id" ]]; then
	printf 'Scaleway RDB create did not return a parseable instance ID\n' >&2
	exit 1
fi
acceptance_cleanup_ledger_record "$ledger_path" rdb-instance "$source_name" "$source_id"

printf '+ scaleway-rdb: source ready id=%s; starting snapshot/restore cell\n' "$source_id"
(cd "$ROOT" && MAGELIFT_SCALEWAY_DATABASE_ROOT_PASSWORD="$root_password" go run ./cmd/scaleway-database-recovery-acceptance \
	--profile "$profile" \
	--project "$project" \
	--region "$region" \
	--instance "$source_id" \
	--marker "$marker" \
	--fixture "$fixture" \
	--restore-node-type "$node_type")

printf 'Scaleway Managed Database recovery acceptance application-fixture cell completed source=%s marker=%s\n' "$source_id" "$marker"

#!/usr/bin/env bash
# Cost-bounded OVHcloud Public Cloud Database recovery acceptance. The wrapper
# owns one generated encrypted MySQL source, its runner CIDR, and the primary
# credential; the Go cell materializes the known fixture before selecting the
# provider-managed PITR point, then owns isolated fork restore, polling, and
# output cleanup. The trap always removes only the exact marker-owned restore
# identities before deleting the generated source.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-cleanup-ledger.sh
source "$ROOT/scripts/acceptance/lib-cleanup-ledger.sh"

profile="${MAGELIFT_OVH_DATABASE_RECOVERY_PROFILE:-${MAGELIFT_OVH_PROFILE:-default}}"
project="${MAGELIFT_OVH_DATABASE_RECOVERY_PROJECT:-8728028545db487baeee2e472e7e96dd}"
region="${MAGELIFT_OVH_DATABASE_RECOVERY_REGION:-GRA}"
engine="${MAGELIFT_OVH_DATABASE_RECOVERY_ENGINE:-mysql}"
version="${MAGELIFT_OVH_DATABASE_RECOVERY_VERSION:-8.4}"
plan="${MAGELIFT_OVH_DATABASE_RECOVERY_PLAN:-essential}"
flavor="${MAGELIFT_OVH_DATABASE_RECOVERY_FLAVOR:-db1-4}"
disk_gb="${MAGELIFT_OVH_DATABASE_RECOVERY_DISK_GB:-80}"
run_id="${MAGELIFT_OVH_DATABASE_RECOVERY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
source_description="${MAGELIFT_OVH_DATABASE_RECOVERY_SOURCE_DESCRIPTION:-magelift-database-${run_id}}"
marker="${MAGELIFT_OVH_DATABASE_RECOVERY_MARKER:-magelift/ovh/database-recovery/${run_id}}"
fixture="${MAGELIFT_OVH_DATABASE_RECOVERY_FIXTURE:-fixture-database-control-plane-${run_id}}"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$project" =~ ^[0-9a-fA-F]{32}$ || ! "$region" =~ ^[A-Z0-9-]{2,16}$ || ! "$engine" =~ ^[a-z0-9]{2,16}$ || ! "$version" =~ ^[0-9][A-Za-z0-9._-]{0,15}$ || ! "$plan" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$flavor" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$source_description" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$disk_gb" =~ ^[0-9]{1,4}$ ]]; then
	printf 'set valid OVHcloud profile, project, region, engine, version, plan, flavor, disk size, run ID, source description, marker, and fixture values\n' >&2
	exit 2
fi

export MAGELIFT_OVH_PROFILE="$profile"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'OVHcloud Public Cloud Database recovery acceptance dry-run ok profile=%s project=%s region=%s engine=%s version=%s plan=%s flavor=%s sourceDescription=%s marker=%s; no OVH mutation invoked\n' \
		"$profile" "$project" "$region" "$engine" "$version" "$plan" "$flavor" "$source_description" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands ovhcloud curl go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

export MAGELIFT_CLEANUP_LEDGER_DIR="${MAGELIFT_CLEANUP_LEDGER_DIR:-$ROOT/.magelift/cleanup}"
export MAGELIFT_CLEANUP_RUN_ID="$run_id"
export MAGELIFT_CLEANUP_MARKER="$marker"
export MAGELIFT_CLEANUP_PROVIDER=ovh
export MAGELIFT_CLEANUP_REGION="$region"
export MAGELIFT_CLEANUP_PROJECT="$project"
export MAGELIFT_CLEANUP_PROFILE="$profile"
ledger_path="$(acceptance_cleanup_ledger_path "$run_id")"

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-10800}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-ovh-database-recovery-${run_id}-ttl-expired-$$"

source_id=""
root_password=""
source_claimed=0
create_stdout_file=""
create_stderr_file=""
reset_stdout_file=""
reset_stderr_file=""
database_user="avnadmin"
marker_digest=""
restore_prefix=""

redact_provider_output() {
	sed -E \
		-e 's/Your generated password is .*/Your generated password is [REDACTED]/' \
		-e 's/([Pp]assword:?[[:space:]]+)[^"[:space:],}]+/\1[REDACTED]/g' \
		-e 's/("password"[[:space:]]*:[[:space:]]*")[^"]*(")/\1[REDACTED]\2/g' \
		-e 's/("generatedPassword"[[:space:]]*:[[:space:]]*")[^"]*(")/\1[REDACTED]\2/g'
}

extract_generated_password() {
	local file candidate
	for file in "$@"; do
		[[ -f "$file" ]] || continue
		candidate="$(sed -n '/^{/,$p' "$file" | jq -r 'if type == "object" then (.details.password // .password // .credentials.password // .user.password // empty) else empty end' 2>/dev/null | tail -n1 || true)"
		if [[ -z "$candidate" ]]; then
			candidate="$(sed -nE 's/.*[Pp]assword[:=][[:space:]]+([^"[:space:],}]+).*/\1/p' "$file" | tail -n1)"
		fi
		if [[ -n "$candidate" && ! "$candidate" =~ [[:space:][:cntrl:]] && ${#candidate} -le 128 ]]; then
			printf '%s' "$candidate"
			return 0
		fi
	done
	return 1
}

ovh_restore_prefix() {
	local digest
	digest="$(printf '%s' "$marker" | shasum -a 256 | awk '{print $1}' | cut -c1-24)"
	printf 'magelift-restore-%s-' "$digest"
}

list_instances() {
	ovhcloud --profile "$profile" --output json cloud managed-database list --cloud-project "$project"
}

delete_owned_outputs() {
	local instances_json id description
	instances_json="$(list_instances 2>/dev/null)" || return 1
	while IFS=$'\t' read -r id description; do
		[[ -z "$id" || "$id" == "$source_id" ]] && continue
		[[ "$description" == "$source_description" || "$description" == "$marker" ]] && continue
		if [[ -z "$restore_prefix" || "$description" != "${restore_prefix}"* ]]; then
			continue
		fi
		if ! ovhcloud --profile "$profile" cloud managed-database delete "$id" --cloud-project "$project" >/dev/null 2>&1; then
			if ! ovhcloud --profile "$profile" --output json cloud managed-database get "$id" --cloud-project "$project" >/dev/null 2>&1; then
				continue
			fi
			printf 'owned OVHcloud database restore deletion failed id=%s\n' "$id" >&2
			return 1
		fi
	done < <(jq -r --arg prefix "$restore_prefix" '
		(. // [])[]? |
		select((.description // "") | startswith($prefix)) |
		[ (.id // ""), (.description // "") ] | @tsv
	' <<<"$instances_json")
}

wait_for_outputs_absent() {
	local attempt instances_json
	for attempt in {1..90}; do
		instances_json="$(list_instances 2>/dev/null)" || return 1
		if [[ "$(jq -r --arg prefix "$restore_prefix" --arg source "$source_id" --arg source_description "$source_description" --arg marker "$marker" '
			[(. // [])[]? |
				select((.description // "") | startswith($prefix)) |
				select((.id // "") != $source and (.description // "") != $source_description and (.description // "") != $marker)
			] | length
		' <<<"$instances_json")" == 0 ]]; then
			return 0
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 5
		fi
	done
	printf 'OVHcloud database recovery outputs did not reach zero before cleanup timeout marker=%s\n' "$marker" >&2
	return 1
}

delete_source() {
	local attempt instances_json source_json id description
	for attempt in {1..90}; do
		instances_json="$(list_instances 2>/dev/null)" || return 1
		source_json="$(jq -c --arg source "$source_id" --arg source_description "$source_description" --arg marker "$marker" '
			(. // [])[]? | select((.id // "") == $source or (.description // "") == $source_description or (.description // "") == $marker)
		' <<<"$instances_json" | head -n1)"
		if [[ -z "$source_json" ]]; then
			return 0
		fi
		id="$(jq -r '.id // empty' <<<"$source_json")"
		description="$(jq -r '.description // empty' <<<"$source_json")"
		if [[ "$description" != "$marker" && "$description" != "$source_description" ]]; then
			printf 'refusing to delete OVHcloud database source without the exact ownership description id=%s\n' "$id" >&2
			return 1
		fi
		if ! ovhcloud --profile "$profile" cloud managed-database delete "$id" --cloud-project "$project" >/dev/null 2>&1; then
			if ovhcloud --profile "$profile" --output json cloud managed-database get "$id" --cloud-project "$project" >/dev/null 2>&1; then
				printf 'generated OVHcloud database source deletion failed id=%s\n' "$id" >&2
				return 1
			fi
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 5
		fi
	done
	printf 'generated OVHcloud database source did not reach terminal absence before cleanup timeout description=%s\n' "$source_description" >&2
	return 1
}

wait_for_source_ready() {
	local attempt status_json status
	for attempt in {1..120}; do
		status_json="$(ovhcloud --profile "$profile" --output json cloud managed-database get "$source_id" --cloud-project "$project" 2>/dev/null)" || return 1
		status="$(jq -r '.status // empty' <<<"$status_json")"
		if [[ "$status" == "READY" ]]; then
			printf '+ ovh-database: source ready id=%s\n' "$source_id"
			return 0
		fi
		if [[ "$status" == "ERROR" || "$status" == "ERROR_INCONSISTENT_SPEC" ]]; then
			printf 'OVHcloud database source entered terminal status=%s id=%s\n' "$status" "$source_id" >&2
			return 1
		fi
		if [[ "$attempt" -eq 1 ]]; then
			printf '+ ovh-database: source creating id=%s status=%s\n' "$source_id" "$status"
		fi
		if [[ "$attempt" -lt 120 ]]; then
			sleep 15
		fi
	done
	printf 'OVHcloud database source did not reach READY before timeout id=%s\n' "$source_id" >&2
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
	if [[ -n "$reset_stdout_file" ]]; then
		rm -f "$reset_stdout_file"
	fi
	if [[ -n "$reset_stderr_file" ]]; then
		rm -f "$reset_stderr_file"
	fi
	if acceptance_ttl_expired; then
		printf 'OVHcloud Public Cloud Database recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
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
		printf 'OVHcloud Public Cloud Database recovery acceptance source and outputs were cleaned through exact ownership marker=%s\n' "$marker"
	else
		printf 'OVHcloud Public Cloud Database recovery acceptance failed; exact cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"
acceptance_cleanup_reconcile_pending "$MAGELIFT_CLEANUP_LEDGER_DIR"

caller_ipv4="$(curl -4 -fsS --max-time 15 https://api.ipify.org 2>/dev/null || true)"
if [[ ! "$caller_ipv4" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
	printf 'could not determine a usable runner public IPv4 address for the temporary OVHcloud database IP restriction\n' >&2
	exit 2
fi

restore_prefix="$(ovh_restore_prefix)"
marker_digest="${restore_prefix#magelift-restore-}"
marker_digest="${marker_digest%-}"

existing_instances="$(list_instances)"
if jq -e --arg source_description "$source_description" --arg marker "$marker" --arg prefix "$restore_prefix" '
	any((. // [])[]?;
		(.description // "") == $source_description or (.description // "") == $marker or ((.description // "") | startswith($prefix)))
' <<<"$existing_instances" >/dev/null; then
	printf 'generated OVHcloud database source already exists or the marker is already owned; refusing to adopt it: %s\n' "$source_description" >&2
	exit 2
fi

acceptance_cleanup_ledger_claim "$ledger_path" database-instance source "$source_description" 30

printf '+ ovh-database: creating encrypted source description=%s engine=%s version=%s plan=%s flavor=%s region=%s disk=%sGi\n' \
	"$marker" "$engine" "$version" "$plan" "$flavor" "$region" "$disk_gb"

create_stdout_file="$(mktemp "${TMPDIR:-/tmp}/magelift-ovh-database-create-stdout.XXXXXX")"
create_stderr_file="$(mktemp "${TMPDIR:-/tmp}/magelift-ovh-database-create-stderr.XXXXXX")"
if ! ovhcloud --profile "$profile" --output json cloud managed-database create \
	--cloud-project "$project" \
	--engine "$engine" \
	--version "$version" \
	--plan "$plan" \
	--description "$marker" \
	--nodes-pattern.flavor "$flavor" \
	--nodes-pattern.region "$region" \
	--nodes-pattern.number 1 \
	--ip-restrictions "$caller_ipv4/32" \
	--disk-size "$disk_gb" >"$create_stdout_file" 2>"$create_stderr_file"; then
	printf 'OVHcloud database source creation failed; provider response follows:\n' >&2
	redact_provider_output <"$create_stderr_file" >&2 || true
	redact_provider_output <"$create_stdout_file" >&2 || true
	exit 1
fi
source_claimed=1
source_id="$(jq -r '.id // .serviceId // empty' <"$create_stdout_file" 2>/dev/null || true)"
rm -f "$create_stdout_file" "$create_stderr_file"
create_stdout_file=""
create_stderr_file=""
if [[ -z "$source_id" ]]; then
	instances_json="$(list_instances)"
	source_id="$(jq -r --arg marker "$marker" '(. // [])[]? | select((.description // "") == $marker) | (.id // empty)' <<<"$instances_json" | head -n1)"
fi
if [[ -z "$source_id" ]]; then
	printf 'OVHcloud database create did not return a parseable service ID\n' >&2
	exit 1
fi
acceptance_cleanup_ledger_record "$ledger_path" database-instance "$source_description" "$source_id"

if ! wait_for_source_ready; then
	exit 1
fi

users_json="$(ovhcloud --profile "$profile" --output json cloud managed-database user list "$source_id" --cloud-project "$project" 2>/dev/null)" || {
	printf 'could not list OVHcloud database users for the generated source id=%s\n' "$source_id" >&2
	exit 1
}
user_id="$(jq -r --arg username "$database_user" '[(. // [])[]? | select((.username // "") == $username) | (.id // "")] | if length == 1 then .[0] else empty end' <<<"$users_json")"
if [[ -z "$user_id" ]]; then
	printf 'could not identify exactly one primary OVHcloud database user username=%s\n' "$database_user" >&2
	exit 1
fi
reset_stdout_file="$(mktemp "${TMPDIR:-/tmp}/magelift-ovh-database-reset-stdout.XXXXXX")"
reset_stderr_file="$(mktemp "${TMPDIR:-/tmp}/magelift-ovh-database-reset-stderr.XXXXXX")"
if ! ovhcloud --profile "$profile" --output json cloud managed-database user credentials-reset "$source_id" "$user_id" --cloud-project "$project" >"$reset_stdout_file" 2>"$reset_stderr_file"; then
	printf 'OVHcloud database primary credential reset failed; provider response follows:\n' >&2
	redact_provider_output <"$reset_stderr_file" >&2 || true
	redact_provider_output <"$reset_stdout_file" >&2 || true
	exit 1
fi
if ! root_password="$(extract_generated_password "$reset_stdout_file" "$reset_stderr_file")"; then
	printf 'OVHcloud database credential reset did not return a usable password; refusing to run an unverified application fixture\n' >&2
	exit 1
fi
rm -f "$reset_stdout_file" "$reset_stderr_file"
reset_stdout_file=""
reset_stderr_file=""

printf '+ ovh-database: starting application-fixture backup/restore cell\n'
(cd "$ROOT" && MAGELIFT_OVH_DATABASE_ROOT_PASSWORD="$root_password" MAGELIFT_OVH_DATABASE_RESTORE_IP_RESTRICTION="$caller_ipv4/32" go run ./cmd/ovh-database-recovery-acceptance \
	--profile "$profile" \
	--project "$project" \
	--region "$region" \
	--engine "$engine" \
	--instance "$source_id" \
	--marker "$marker" \
	--fixture "$fixture" \
	--restore-plan "$plan" \
	--restore-flavor "$flavor" \
	--restore-version "$version" \
	--disk-gb "$disk_gb" \
	--restore-ip-restriction "$caller_ipv4/32")
root_password=""

printf 'OVHcloud Public Cloud Database recovery acceptance application-fixture cell completed source=%s marker=%s\n' "$source_id" "$marker"

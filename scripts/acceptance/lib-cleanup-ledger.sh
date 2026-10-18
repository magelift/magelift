#!/usr/bin/env bash
# Restartable cleanup ledger helpers. EXIT traps are last-mile; this JSON
# document is the authority after a crash, kill, or machine sleep.
# Schema: {"version":1,"runId":"...","marker":"...","provider":"...","region":"...","project":"...","profile":"...","claimedAt":"RFC3339","resources":[{"kind":"...","role":"source|restore|snapshot","name":"...","identity":"...","rank":10,"status":"intended|claimed|deleting|gone|tombstone|refused"}]}
# shellcheck shell=bash

: "${MAGELIFT_CLEANUP_LEDGER_DIR:=.magelift/cleanup}"

acceptance_cleanup_ledger_path() {
	local run_id="${1:?run id required}"
	printf '%s/%s.json' "$MAGELIFT_CLEANUP_LEDGER_DIR" "$run_id"
}

acceptance_cleanup_ledger_init() {
	local path="${1:?ledger path required}"
	local run_id="${2:?run id required}"
	local marker="${3:?marker required}"
	local provider="${4:?provider required}"
	local region="${5:-}"
	local project="${6:-}"
	local profile="${7:-}"
	local claimed_at
	claimed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	mkdir -p "$(dirname "$path")"
	if [[ -f "$path" ]]; then
		return 0
	fi
	jq -n --arg runId "$run_id" --arg marker "$marker" --arg provider "$provider" \
		--arg region "$region" --arg project "$project" --arg profile "$profile" --arg claimedAt "$claimed_at" \
		'{version:1, runId:$runId, marker:$marker, provider:$provider, region:$region, project:$project, profile:$profile, claimedAt:$claimedAt, resources:[]}' \
		>"$path"
	chmod 600 "$path"
}

acceptance_cleanup_ledger_claim() {
	local path="${1:?ledger path required}"
	local kind="${2:?kind required}"
	local role="${3:?role required}"
	local name="${4:?name required}"
	local rank="${5:?rank required}"
	local tmp
	acceptance_cleanup_ledger_init "$path" "${MAGELIFT_CLEANUP_RUN_ID:?}" "${MAGELIFT_CLEANUP_MARKER:?}" "${MAGELIFT_CLEANUP_PROVIDER:?}" "${MAGELIFT_CLEANUP_REGION:-}" "${MAGELIFT_CLEANUP_PROJECT:-}" "${MAGELIFT_CLEANUP_PROFILE:-}"
	tmp=$(mktemp "${path}.tmp.XXXXXX")
	jq --arg kind "$kind" --arg role "$role" --arg name "$name" --argjson rank "$rank" \
		'if any(.resources[]?; .kind == $kind and .role == $role and .name == $name) then .
		else .resources += [{kind:$kind, role:$role, name:$name, rank:$rank, status:"intended"}]
		end' \
		"$path" >"$tmp"
	mv "$tmp" "$path"
	chmod 600 "$path"
}

acceptance_cleanup_ledger_record() {
	local path="${1:?ledger path required}"
	local kind="${2:?kind required}"
	local name="${3:?name required}"
	local identity="${4:?identity required}"
	local tmp
	if ! jq -e --arg kind "$kind" --arg name "$name" \
		'any(.resources[]?; .kind == $kind and .name == $name)' "$path" >/dev/null; then
		printf 'cleanup ledger has no %s named %s to record\n' "$kind" "$name" >&2
		return 1
	fi
	tmp=$(mktemp "${path}.tmp.XXXXXX")
	jq --arg kind "$kind" --arg name "$name" --arg identity "$identity" \
		'(.resources[] | select(.kind == $kind and .name == $name) | .identity) = $identity |
		 (.resources[] | select(.kind == $kind and .name == $name) | .status) = "claimed"' \
		"$path" >"$tmp"
	mv "$tmp" "$path"
	chmod 600 "$path"
}

acceptance_cleanup_reconcile_pending() {
	local dir="${1:-$MAGELIFT_CLEANUP_LEDGER_DIR}"
	local root="${ROOT:-.}"
	if [[ ! -d "$dir" ]]; then
		return 0
	fi
	if ! find "$dir" -maxdepth 1 -name '*.json' -print -quit | grep -q .; then
		return 0
	fi
	local file provider current_provider="${MAGELIFT_CLEANUP_PROVIDER:-}"
	for file in "$dir"/*.json; do
		[[ -f "$file" ]] || continue
		provider="$(jq -r '.provider // empty' "$file")"
		if [[ -n "$current_provider" && "$provider" != "$current_provider" ]]; then
			# A wrapper may share the ledger directory with another provider's
			# interrupted run. Reconcile only the provider this wrapper can
			# authenticate and mutate; leave the other ledger for its owner.
			continue
		fi
		if jq -e '.resources[]? | select(.status == "intended" or .status == "claimed" or .status == "deleting")' "$file" >/dev/null; then
			printf '+ cleanup ledger: reconciling pending claims in %s\n' "$file" >&2
			(cd "$root" && go run ./cmd/magelift --yes --output json cleanup reconcile --ledger "$file") || return $?
		fi
	done
	return 0
}

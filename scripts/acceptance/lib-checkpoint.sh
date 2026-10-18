#!/usr/bin/env bash
# Checkpoint helpers for credit-efficient multi-cell acceptance (ACCEPT-02).
# Schema: {"version":4,"fingerprint":"sha256","cells":{"cell-id":{"result":"PASS|FAIL","at":"ISO8601","durationSeconds":1,"sessionMode":"baseline|reused","artifactDigest":"registry/image@sha256:...","fixtureId":"...","backupSet":"...","migrationFingerprint":"...","schemaFingerprint":"...","stateBackend":"...","edgeSetup":"...","observabilitySetup":"...","providerOperationIds":[],"backupIds":[],"restoreIds":[],"edgeIdentities":[],"observabilityIdentities":[],"cleanupState":"pending|complete"}}}
# Resume: killing mid-matrix re-enters at the first cell that is not PASS.
# GCP harness defaults ACCEPTANCE_CHECKPOINT to
# .magelift/gcp-matrix/acceptance-checkpoint.json (see gcp-acceptance-local.sh).
# shellcheck shell=bash

: "${ACCEPTANCE_CHECKPOINT:=${MAGELIFT_ACCEPTANCE_CHECKPOINT:-.magelift/acceptance-checkpoint.json}}"

acceptance_checkpoint_ensure() {
	local dir
	dir=$(dirname "$ACCEPTANCE_CHECKPOINT")
	mkdir -p "$dir"
	if [[ ! -f "$ACCEPTANCE_CHECKPOINT" ]]; then
		if [[ -n "${ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}" ]]; then
		jq -n --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" \
			'{version: 4, fingerprint: $fingerprint, cells: {}}' >"$ACCEPTANCE_CHECKPOINT"
		else
		printf '%s\n' '{"version":4,"cells":{}}' >"$ACCEPTANCE_CHECKPOINT"
		fi
		return 0
	fi

	# A retained provider stack can be reused for another release or topology.
	# Never let a PASS from the previous configuration skip a cell in the new
	# configuration. Keep the old file as local evidence before resetting it.
	# Create-once can fail before any cell is recorded; stamp the fingerprint
	# onto an empty checkpoint so KEEP resume does not drop the run id.
	if [[ -n "${ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}" ]] && command -v jq >/dev/null 2>&1 && \
		jq -e '.cells | type == "object"' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1 && \
		! jq -e --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" \
			'.fingerprint == $fingerprint' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
		local stale_path suffix tmp
		if jq -e '.cells == {}' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
			tmp=$(mktemp "${ACCEPTANCE_CHECKPOINT}.tmp.XXXXXX")
			jq --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" \
				'.fingerprint = $fingerprint' "$ACCEPTANCE_CHECKPOINT" >"$tmp"
			mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
			printf 'acceptance checkpoint stamped configuration fingerprint %s (empty cells, run id kept)\n' \
				"$ACCEPTANCE_CHECKPOINT_FINGERPRINT" >&2
			return 0
		fi
		suffix="$(date -u +%Y%m%dT%H%M%SZ)-$$"
		stale_path="${ACCEPTANCE_CHECKPOINT%.json}.stale-${suffix}.json"
		if [[ "$stale_path" == "$ACCEPTANCE_CHECKPOINT" ]]; then
			stale_path="${ACCEPTANCE_CHECKPOINT}.stale-${suffix}"
		fi
		mv "$ACCEPTANCE_CHECKPOINT" "$stale_path"
		tmp=$(mktemp "${ACCEPTANCE_CHECKPOINT}.tmp.XXXXXX")
		jq -n --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" \
			'{version: 4, fingerprint: $fingerprint, cells: {}}' >"$tmp"
		mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
		printf 'acceptance checkpoint reset for configuration fingerprint %s (previous file: %s)\n' \
			"$ACCEPTANCE_CHECKPOINT_FINGERPRINT" "$stale_path" >&2
	fi
}

acceptance_checkpoint_load() {
	acceptance_checkpoint_ensure
	if ! jq -e '.cells | type == "object"' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
		printf 'invalid checkpoint (expected .cells object): %s\n' "$ACCEPTANCE_CHECKPOINT" >&2
		return 1
	fi
}

# cell_done CELL_ID; exit 0 if cell already recorded as PASS (FAIL is retryable on resume).
cell_done() {
	local cell="${1:?cell id required}"
	acceptance_checkpoint_load
	jq -e --arg c "$cell" '.cells[$c].result == "PASS"' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1
}

# record_cell CELL_ID RESULT; persist result + UTC timestamp.
record_cell() {
	local cell="${1:?cell id required}"
	local result="${2:?result required}"
	local at mode stack_id fingerprint artifact_digest fixture_id backup_set migration_fingerprint schema_fingerprint state_backend edge_setup observability_setup duration_seconds provider_operation_ids backup_ids restore_ids edge_identities observability_identities cleanup_state
	at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	mode="${MAGELIFT_ACCEPTANCE_SESSION_MODE:-baseline}"
	stack_id="${MAGELIFT_ACCEPTANCE_STACK_ID:-}"
	fingerprint="${MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT:-${ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}}"
	artifact_digest="${MAGELIFT_ACCEPTANCE_DIGEST:-${MAGELIFT_AWS_ACCEPTANCE_DIGEST:-${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_OVH_ACCEPTANCE_DIGEST:-${MAGELIFT_SCALEWAY_ACCEPTANCE_DIGEST:-}}}}}"
	fixture_id="${MAGELIFT_ACCEPTANCE_FIXTURE_ID:-}"
	backup_set="${MAGELIFT_ACCEPTANCE_BACKUP_SET:-}"
	migration_fingerprint="${MAGELIFT_ACCEPTANCE_MIGRATION_FINGERPRINT:-}"
	schema_fingerprint="${MAGELIFT_ACCEPTANCE_SCHEMA_FINGERPRINT:-}"
	state_backend="${MAGELIFT_ACCEPTANCE_STATE_BACKEND:-}"
	edge_setup="${MAGELIFT_ACCEPTANCE_EDGE_SETUP:-${MAGELIFT_ACCEPTANCE_EDGE:-}}"
	observability_setup="${MAGELIFT_ACCEPTANCE_OBSERVABILITY_SETUP:-${MAGELIFT_ACCEPTANCE_OBSERVABILITY:-}}"
	duration_seconds="${MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS:-0}"
	[[ "$duration_seconds" =~ ^[0-9]+$ ]] || duration_seconds=0
	provider_operation_ids="${MAGELIFT_ACCEPTANCE_PROVIDER_OPERATION_IDS:-}"
	backup_ids="${MAGELIFT_ACCEPTANCE_BACKUP_IDS:-}"
	restore_ids="${MAGELIFT_ACCEPTANCE_RESTORE_IDS:-}"
	edge_identities="${MAGELIFT_ACCEPTANCE_EDGE_IDENTITIES:-}"
	observability_identities="${MAGELIFT_ACCEPTANCE_OBSERVABILITY_IDENTITIES:-}"
	cleanup_state="${MAGELIFT_ACCEPTANCE_CLEANUP_STATE:-pending}"
	case "$cleanup_state" in pending|complete) ;; *) cleanup_state=pending ;; esac
	acceptance_checkpoint_load
	local tmp
	tmp=$(mktemp "${ACCEPTANCE_CHECKPOINT}.tmp.XXXXXX")
	jq --arg c "$cell" --arg r "$result" --arg at "$at" --arg mode "$mode" \
		--arg stackId "$stack_id" --arg fingerprint "$fingerprint" --arg artifactDigest "$artifact_digest" \
		--arg fixtureId "$fixture_id" --arg backupSet "$backup_set" --arg migrationFingerprint "$migration_fingerprint" \
		--arg schemaFingerprint "$schema_fingerprint" --arg stateBackend "$state_backend" --arg edgeSetup "$edge_setup" \
		--arg observabilitySetup "$observability_setup" --argjson durationSeconds "$duration_seconds" \
		--arg providerOperationIds "$provider_operation_ids" --arg backupIds "$backup_ids" --arg restoreIds "$restore_ids" \
		--arg edgeIdentities "$edge_identities" --arg observabilityIdentities "$observability_identities" --arg cleanupState "$cleanup_state" \
		'.version = 4 | .cells[$c] = {result: $r, at: $at, durationSeconds: $durationSeconds, sessionMode: $mode, stackId: $stackId, fingerprint: $fingerprint,
			artifactDigest: $artifactDigest, fixtureId: $fixtureId, backupSet: $backupSet, migrationFingerprint: $migrationFingerprint,
			schemaFingerprint: $schemaFingerprint, stateBackend: $stateBackend, edgeSetup: $edgeSetup, observabilitySetup: $observabilitySetup,
			providerOperationIds: ($providerOperationIds | if . == "" then [] else split(",") | map(select(length > 0)) end),
			backupIds: ($backupIds | if . == "" then [] else split(",") | map(select(length > 0)) end),
			restoreIds: ($restoreIds | if . == "" then [] else split(",") | map(select(length > 0)) end),
			edgeIdentities: ($edgeIdentities | if . == "" then [] else split(",") | map(select(length > 0)) end),
			observabilityIdentities: ($observabilityIdentities | if . == "" then [] else split(",") | map(select(length > 0)) end),
			cleanupState: $cleanupState}' \
		"$ACCEPTANCE_CHECKPOINT" >"$tmp"
	mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
}

# Bind JSONL run IDs to the checkpoint so KEEP destroy/resume records cleanup
# on the same run as Magento PASS cells. A new prefix has its own checkpoint
# directory; fingerprint mismatch still resets the file.
acceptance_checkpoint_bind_run_id() {
	acceptance_checkpoint_ensure
	if ! command -v jq >/dev/null 2>&1; then
		return 0
	fi
	local existing
	existing="$(jq -r '.runId // empty' "$ACCEPTANCE_CHECKPOINT" 2>/dev/null || true)"
	if [[ -z "${MAGELIFT_ACCEPTANCE_RUN_ID:-}" && -n "$existing" ]]; then
		export ACCEPTANCE_EVIDENCE_RUN_ID="$existing"
		export MAGELIFT_ACCEPTANCE_RUN_ID="$existing"
		printf '+ reusing acceptance run id %s from checkpoint\n' "$existing" >&2
		return 0
	fi
	local run_id="${MAGELIFT_ACCEPTANCE_RUN_ID:-${ACCEPTANCE_EVIDENCE_RUN_ID:-}}"
	if [[ -z "$run_id" ]]; then
		return 0
	fi
	local tmp
	tmp=$(mktemp "${ACCEPTANCE_CHECKPOINT}.tmp.XXXXXX")
	jq --arg runId "$run_id" '.runId = $runId' "$ACCEPTANCE_CHECKPOINT" >"$tmp"
	mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
}

# update_cell_cleanup_state CELL_ID STATE; update the cleanup truth after the
# provider-owned delete/inventory phase without rewriting the cell evidence.
update_cell_cleanup_state() {
	local cell="${1:?cell id required}"
	local cleanup_state="${2:?cleanup state required}"
	case "$cleanup_state" in
	pending|complete) ;;
	*)
		printf 'invalid cleanup state for %s: %s\n' "$cell" "$cleanup_state" >&2
		return 2
		;;
	esac
	acceptance_checkpoint_load
	if ! jq -e --arg c "$cell" '.cells[$c] | type == "object"' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
		printf 'cannot update cleanup state for unrecorded cell: %s\n' "$cell" >&2
		return 1
	fi
	local tmp
	tmp=$(mktemp "${ACCEPTANCE_CHECKPOINT}.tmp.XXXXXX")
	jq --arg c "$cell" --arg cleanupState "$cleanup_state" \
		'.cells[$c].cleanupState = $cleanupState' \
		"$ACCEPTANCE_CHECKPOINT" >"$tmp"
	mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
}

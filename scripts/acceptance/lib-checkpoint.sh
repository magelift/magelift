#!/usr/bin/env bash
# Checkpoint helpers for credit-efficient multi-cell acceptance (ACCEPT-02).
# Schema: {"cells":{"cell-id":{"result":"PASS|FAIL","at":"ISO8601"}}}
# shellcheck shell=bash

: "${ACCEPTANCE_CHECKPOINT:=${MAGELIFT_ACCEPTANCE_CHECKPOINT:-.magelift/acceptance-checkpoint.json}}"

acceptance_checkpoint_ensure() {
	local dir
	dir=$(dirname "$ACCEPTANCE_CHECKPOINT")
	mkdir -p "$dir"
	if [[ ! -f "$ACCEPTANCE_CHECKPOINT" ]]; then
		printf '%s\n' '{"cells":{}}' >"$ACCEPTANCE_CHECKPOINT"
	fi
}

acceptance_checkpoint_load() {
	acceptance_checkpoint_ensure
	if ! jq -e '.cells | type == "object"' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
		printf 'invalid checkpoint (expected .cells object): %s\n' "$ACCEPTANCE_CHECKPOINT" >&2
		return 1
	fi
}

# cell_done CELL_ID — exit 0 if cell already recorded.
cell_done() {
	local cell="${1:?cell id required}"
	acceptance_checkpoint_load
	jq -e --arg c "$cell" '.cells[$c] != null' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1
}

# record_cell CELL_ID RESULT — persist result + UTC timestamp.
record_cell() {
	local cell="${1:?cell id required}"
	local result="${2:?result required}"
	local at
	at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	acceptance_checkpoint_load
	local tmp
	tmp=$(mktemp)
	jq --arg c "$cell" --arg r "$result" --arg at "$at" \
		'.cells[$c] = {result: $r, at: $at}' \
		"$ACCEPTANCE_CHECKPOINT" >"$tmp"
	mv "$tmp" "$ACCEPTANCE_CHECKPOINT"
}

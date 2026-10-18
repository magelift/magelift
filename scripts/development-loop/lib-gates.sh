#!/usr/bin/env bash
# Independent development-loop gate records.
# A FAIL in one gate is not copied onto another. A missing prerequisite is SKIP.
# shellcheck shell=bash

devloop_gate_allowed() {
	case "$1" in
	distribution | artifact | gcp-product) return 0 ;;
	*) return 1 ;;
	esac
}

devloop_status_allowed() {
	case "$1" in
	PASS | FAIL | SKIP) return 0 ;;
	*) return 1 ;;
	esac
}

# devloop_append_gate FILE GATE STATUS DETAIL
# DETAIL is required for FAIL and SKIP and must not contain a secret value.
# The caller passes a redacted reason. This function does not inspect logs.
devloop_append_gate() {
	local file="${1:?gate file required}" gate="${2:?gate required}" status="${3:?status required}" detail="${4:-}"
	if ! devloop_gate_allowed "$gate"; then
		printf 'gate must be distribution, artifact, or gcp-product\n' >&2
		return 1
	fi
	if ! devloop_status_allowed "$status"; then
		printf 'status must be PASS, FAIL, or SKIP\n' >&2
		return 1
	fi
	if [[ "$status" != PASS && -z "$detail" ]]; then
		printf '%s requires a detail\n' "$status" >&2
		return 1
	fi
	mkdir -p "$(dirname "$file")"
	jq -nc --arg gate "$gate" --arg status "$status" --arg detail "$detail" \
		'{gate:$gate,status:$status,detail:$detail}' >>"$file"
}

# devloop_record_blocked FILE GATE PREREQUISITE
# Records SKIP for a gate that cannot start. Does not copy another gate's FAIL.
devloop_record_blocked() {
	local file="${1:?gate file required}" gate="${2:?gate required}" prerequisite="${3:?prerequisite required}"
	devloop_append_gate "$file" "$gate" SKIP "missing prerequisite: ${prerequisite}"
}

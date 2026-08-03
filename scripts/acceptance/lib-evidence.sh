#!/usr/bin/env bash
# Evidence append helpers for capability-matrix SC#2 columns (ACCEPT-03).
# Columns: cell | result | duration | provider | account | date
# Do not write secrets or digests into evidence columns.
# Certification (GCP 07-07 / AWS certify) requires append_row provenance only; # hand-typed or hand-edited matrix rows are not valid SC1-SC5 evidence (T-07-12).
# GCP harness defaults ACCEPTANCE_EVIDENCE to
# .magelift/gcp-matrix/matrix-results.md (six-column table).
# shellcheck shell=bash

: "${ACCEPTANCE_EVIDENCE:=${MAGELIFT_ACCEPTANCE_EVIDENCE:-.magelift/matrix-results.md}}"

acceptance_evidence_ensure() {
	local dir
	dir=$(dirname "$ACCEPTANCE_EVIDENCE")
	mkdir -p "$dir"
	if [[ ! -f "$ACCEPTANCE_EVIDENCE" ]]; then
		cat >"$ACCEPTANCE_EVIDENCE" <<'EOF'
| cell | result | duration | provider | account | date |
|------|--------|----------|----------|---------|------|
EOF
	fi
}

# append_row CELL RESULT DURATION PROVIDER ACCOUNT DATE
append_row() {
	local cell="${1:?cell required}"
	local result="${2:?result required}"
	local duration="${3:?duration required}"
	local provider="${4:?provider required}"
	local account="${5:?account required}"
	local date="${6:?date required}"
	acceptance_evidence_ensure
	printf '| %s | %s | %s | %s | %s | %s |\n' \
		"$cell" "$result" "$duration" "$provider" "$account" "$date" \
		>>"$ACCEPTANCE_EVIDENCE"
}

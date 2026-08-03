#!/usr/bin/env bash
# Local, credit-efficient AWS acceptance: multi-cell catalog on one logical stack.
# Dry-run (MAGELIFT_ACCEPTANCE_DRY_RUN=1): fixture path; no Pulumi/AWS mutate.
# Live: preview by default; one create-once then catalog cell updates; destroy on
# EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
# shellcheck source=acceptance/lib-assert-clean-aws.sh
source "$ROOT/scripts/acceptance/lib-assert-clean-aws.sh"

CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-aws-preview.txt}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}"

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

dry_run_cell_loop() {
	local cell provider account date_s duration started created_once=0
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-aws}"
	account="${MAGELIFT_ACCEPTANCE_ACCOUNT:-dry-run}"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure

	printf 'aws acceptance dry-run start cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

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
		# Fixture success; no magelift / aws mutate.
		duration="$(( $(date +%s) - started ))s"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "PASS" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "PASS"
		printf 'acceptance cell-done cell=%s result=PASS\n' "$cell" >&2
	done

	printf 'aws acceptance dry-run ok; no AWS create invoked\n' >&2
}

if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
	dry_run_cell_loop
	exit 0
fi

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to an acceptance configuration file}"
: "${MAGELIFT_AWS_ACCEPTANCE_DIGEST:?set MAGELIFT_AWS_ACCEPTANCE_DIGEST to a signed immutable image reference}"
: "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:?set MAGELIFT_AWS_CERTIFICATE_IDENTITY to the expected Sigstore identity}"
: "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:=https://token.actions.githubusercontent.com}"

profile="${MAGELIFT_AWS_ACCEPTANCE_PROFILE:-preview}"
case "$profile" in
preview|standard|high-availability) ;;
*)
	printf 'unsupported acceptance profile: %s (use preview unless credits allow more)\n' "$profile" >&2
	exit 2
;;
esac

if [[ "$profile" != preview && "${MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY:-}" != true ]]; then
	printf 'refusing %s without MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true\n' "$profile" >&2
	exit 2
fi

region="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
if [[ -z "$region" ]]; then
	printf 'set AWS_REGION (or AWS_DEFAULT_REGION) for leftover assertions\n' >&2
	exit 2
fi

# Working copy of config so cell updates never mutate the caller's MAGELIFT_CONFIG.
LIVE_WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/magelift-aws-acceptance.XXXXXX")
LIVE_CONFIG="$LIVE_WORKDIR/magelift.yaml"
cp "$MAGELIFT_CONFIG" "$LIVE_CONFIG"
cleanup_workdir() {
	rm -rf "$LIVE_WORKDIR"
}

config=("$MAGELIFT_BIN" --config "$LIVE_CONFIG" --env "$profile" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

created=0
cleanup() {
	local status=$?
	if [[ "$created" == 1 && "${MAGELIFT_AWS_ACCEPTANCE_KEEP:-false}" != true ]]; then
		printf '+ magelift destroy --yes (EXIT trap)\n' >&2
		"${config[@]}" destroy --yes || printf 'acceptance cleanup failed for %s; inspect with aws-cli and destroy manually\n' "$profile" >&2
		assert_clean || status=1
	fi
	cleanup_workdir
	exit "$status"
}
trap cleanup EXIT

# --- live helpers ----------------------------------------------------------------

checkpoint_has_cells() {
	acceptance_checkpoint_load
	local n
	n=$(jq '.cells | length' "$ACCEPTANCE_CHECKPOINT")
	[[ "${n:-0}" -gt 0 ]]
}

# Resume when KEEP left a stack up: RESUME=1, any checkpoint cells, or first cell done.
should_skip_create_once() {
	local resume="${MAGELIFT_AWS_ACCEPTANCE_RESUME:-0}"
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

# Cell IDs are catalogKey:value (e.g. queueMode:ecs-rabbitmq). No CLI mutate API; # patch target (+ env) catalog in the temp YAML copy, then redeploy same digest.
apply_cell_config() {
	local cell="${1:?cell id required}"
	local key value
	if [[ "$cell" != *:* ]]; then
		printf 'invalid cell id (want key:value): %s\n' "$cell" >&2
		return 2
	fi
	key="${cell%%:*}"
	value="${cell#*:}"
	case "$key" in
	queueMode)
		case "$value" in
		db|ecs-rabbitmq|ecs-artemis) ;;
		amazon-mq)
			printf 'refusing cell %s (amazon-mq excluded from free-tier harness)\n' "$cell" >&2
			return 2
			;;
		*)
			printf 'unsupported queueMode cell value: %s\n' "$value" >&2
			return 2
			;;
		esac
		;;
	*)
		printf 'unsupported cell key %s (harness supports queueMode only)\n' "$key" >&2
		return 2
		;;
	esac

	if ! command -v yq >/dev/null 2>&1; then
		printf 'yq is required to patch catalog cells into the temp config (brew install yq)\n' >&2
		return 2
	fi

	# Prefer env-scoped catalog so preview inherits do not shadow the cell; also set
	# target.aws.catalog for configs that only define queueMode globally.
	yq -i ".target.aws.catalog.${key} = \"${value}\"" "$LIVE_CONFIG"
	yq -i ".environments.\"${profile}\".target.aws.catalog.${key} = \"${value}\"" "$LIVE_CONFIG"
	printf 'acceptance patched %s=%s into temp config\n' "$key" "$value" >&2
}

acceptance_account_id() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_ACCOUNT:-}" ]]; then
		printf '%s' "$MAGELIFT_ACCEPTANCE_ACCOUNT"
		return 0
	fi
	aws sts get-caller-identity --query Account --output text 2>/dev/null || printf 'unknown'
}

live_cell_loop() {
	local cell provider account date_s duration started result rc
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-aws}"
	account="$(acceptance_account_id)"
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure

	printf 'aws acceptance live cell loop cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi

		printf 'acceptance cell-update cell=%s\n' "$cell" >&2
		started=$(date +%s)
		result=PASS
		rc=0
		if ! apply_cell_config "$cell"; then
			result=FAIL
			rc=1
		elif ! run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes; then
			result=FAIL
			rc=1
		elif ! run outputs; then
			result=FAIL
			rc=1
		elif ! run health --mode runtime; then
			result=FAIL
			rc=1
		fi

		duration="$(( $(date +%s) - started ))s"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "$result"
		printf 'acceptance cell-done cell=%s result=%s\n' "$cell" "$result" >&2
		if [[ "$rc" -ne 0 ]]; then
			printf 'acceptance cell failed; recorded FAIL for resume (re-run with KEEP/RESUME)\n' >&2
			return 1
		fi
	done
}

# --- live main -------------------------------------------------------------------

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf 'aws acceptance start profile=%s at=%s\n' "$profile" "$started_at" >&2

run config validate
run doctor
run login

if should_skip_create_once; then
	printf 'acceptance resume: skipping create-once (stack assumed present; KEEP/RESUME/checkpoint)\n' >&2
	created=1
else
	printf 'acceptance create-once\n' >&2
	run preview
	run promote \
		--digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" \
		--certificate-identity "$MAGELIFT_AWS_CERTIFICATE_IDENTITY" \
		--certificate-oidc-issuer "$MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER"
	created=1
	run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes
	run outputs
	run health --mode runtime
fi

live_cell_loop

printf 'aws acceptance ok profile=%s; destroy + assert_clean run on EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true\n' "$profile" >&2

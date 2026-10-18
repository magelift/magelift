#!/usr/bin/env bash
# GCP product gate: delegates to providers/gcp/scripts/gcp-acceptance-local.sh.
# Dry-run never creates cloud resources. When required harness env is missing,
# the gate records SKIP instead of mutating the acceptance project.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are not performed
# by this script.
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"
# shellcheck source=../acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

HARNESS="${MAGELIFT_DEVLOOP_GCP_HARNESS:-$ROOT/providers/gcp/scripts/gcp-acceptance-local.sh}"
SHA=""
GATE_FILE="${MAGELIFT_DEVLOOP_GATE_FILE:-${MAGELIFT_DEVLOOP_GATES:-.magelift/development-loop/gates.jsonl}}"
EVIDENCE_DIR="${MAGELIFT_DEVLOOP_EVIDENCE_DIR:-$ROOT/.magelift/development-loop}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-1}"

usage() {
  sed -n '2,8p' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
  --sha)
    SHA="${2:?--sha needs a value}"
    shift 2
    ;;
  --gate-file)
    GATE_FILE="${2:?--gate-file needs a value}"
    shift 2
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    printf 'gcp-product-gate: unknown argument %s\n' "$1" >&2
    exit 2
    ;;
  esac
done

if [[ -z "$SHA" ]]; then
  printf 'gcp-product-gate: --sha is required\n' >&2
  exit 2
fi

devloop_gcp_is_dry_run() {
  case "${MAGELIFT_ACCEPTANCE_DRY_RUN:-1}" in
  1 | true) return 0 ;;
  *) return 1 ;;
  esac
}

devloop_gcp_digest_is_placeholder() {
  local digest="${1:-}"
  case "$digest" in
  *@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef) return 0 ;;
  *@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa) return 0 ;;
  esac
  return 1
}

devloop_gcp_valid_immutable_digest() {
  local digest="${1:-}"
  [[ "$digest" =~ ^[^@[:space:]]+@sha256:[0-9a-f]{64}$ ]] || return 1
  devloop_gcp_digest_is_placeholder "$digest" && return 1
  return 0
}

devloop_gcp_default_acceptance_name() {
  local nonce
  nonce="$(printf '%s' "$SHA-$$-$RANDOM" | shasum -a 256 | cut -c1-7)" || return 1
  [[ "$nonce" =~ ^[0-9a-f]{7}$ ]] || return 1
  printf 'magelift-%s' "$nonce"
}

devloop_gcp_validate_acceptance_name() {
  local name="${1:-}"
  if ((${#name} < 1 || ${#name} > 16)); then
    printf 'gcp-product-gate: MAGELIFT_GCP_ACCEPTANCE_NAME must be 1-16 characters (got %d)\n' "${#name}" >&2
    return 1
  fi
  if [[ ! "$name" =~ ^[a-z][a-z0-9-]*$ ]]; then
    printf 'gcp-product-gate: MAGELIFT_GCP_ACCEPTANCE_NAME must be lowercase alphanumeric with optional hyphens\n' >&2
    return 1
  fi
  acceptance_campaign_require_prefix gcp "$name"
}

devloop_gcp_keep_retention_requested() {
  case "${MAGELIFT_GCP_ACCEPTANCE_KEEP:-false}" in
  true | 1) return 0 ;;
  esac
  case "${MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE:-false}" in
  true | 1) return 0 ;;
  esac
  case "${MAGELIFT_GCP_ACCEPTANCE_RESUME:-0}" in
  1 | true) return 0 ;;
  esac
  return 1
}

devloop_gcp_reject_unauthorized_retention() {
  if devloop_gcp_keep_retention_requested; then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL \
      'product proof requires a fresh run and cleanup; retained debug/resume runs must use the explicitly authorized harness separately'
    exit 1
  fi
}

devloop_gcp_load_catalog_cells() {
  local catalog="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}"
  local line
  CELLS=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    CELLS+=("$line")
  done <"$catalog"
}

devloop_gcp_verify_live_evidence() {
  local catalog="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}"
  local evidence="${ACCEPTANCE_EVIDENCE:-}"
  local shared="${ACCEPTANCE_SHARED_EVIDENCE:-}"
  local cell
  if [[ -z "$evidence" || ! -f "$evidence" ]]; then
    printf 'gcp-product-gate: live evidence file missing\n' >&2
    return 1
  fi
  devloop_gcp_load_catalog_cells
  if [[ ${#CELLS[@]} -eq 0 ]]; then
    printf 'gcp-product-gate: cell catalog empty: %s\n' "$catalog" >&2
    return 1
  fi
  for cell in "${CELLS[@]}"; do
    if ! grep -qF "| ${cell} | PASS |" "$evidence"; then
      printf 'gcp-product-gate: evidence missing PASS for cell %s\n' "$cell" >&2
      return 1
    fi
  done
  if [[ -z "$shared" || ! -f "$shared" ]]; then
    printf 'gcp-product-gate: shared acceptance evidence missing\n' >&2
    return 1
  fi
  if ! jq -es --arg run "$MAGELIFT_ACCEPTANCE_RUN_ID" --arg prefix "${ACCEPTANCE_NAME}-preview" '
    map(select(.type == "cleanup")) as $cleanup |
    ($cleanup | length) == 1 and
    $cleanup[0].runId == $run and $cleanup[0].status == "PASS" and
    $cleanup[0].cleanup.prefix == $prefix and
    (($cleanup[0].cleanup.remaining // []) | length) == 0
  ' "$shared" >/dev/null 2>&1; then
    printf 'gcp-product-gate: shared cleanup evidence is not PASS\n' >&2
    return 1
  fi
  return 0
}

devloop_gcp_verify_live_log() {
  local log="${1:?log required}"
  if grep -q 'gcp acceptance dry-run ok' "$log"; then
    printf 'gcp-product-gate: live run must not report dry-run success\n' >&2
    return 1
  fi
  if ! grep -q 'gcp acceptance live_cell_loop' "$log"; then
    printf 'gcp-product-gate: live run missing live_cell_loop marker\n' >&2
    return 1
  fi
  if ! grep -q 'gcp acceptance ok mode=up' "$log"; then
    printf 'gcp-product-gate: live run missing mode=up completion marker\n' >&2
    return 1
  fi
  if grep -q 'assert_clean FAILED' "$log"; then
    printf 'gcp-product-gate: live cleanup assert_clean failed\n' >&2
    return 1
  fi
  if ! grep -q 'assert_clean ok' "$log"; then
    printf 'gcp-product-gate: live cleanup assert_clean ok missing\n' >&2
    return 1
  fi
  return 0
}

if ! canary_ref_for_sha "$SHA" >/dev/null; then
  devloop_append_gate "$GATE_FILE" gcp-product FAIL 'commit SHA must be 40 lowercase hexadecimal characters'
  exit 1
fi

devloop_gcp_reject_unauthorized_retention

if devloop_gcp_is_dry_run; then
  if ! command -v jq >/dev/null 2>&1; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP 'missing prerequisite: jq'
    exit 0
  fi
else
  if [[ "${MAGELIFT_GCP_ACCEPTANCE:-}" != 1 ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP 'MAGELIFT_GCP_ACCEPTANCE=1 is required for live product gates'
    exit 0
  fi
  if [[ -z "${MAGELIFT_GCP_PROJECT:-}" ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP 'MAGELIFT_GCP_PROJECT is required for live product gates'
    exit 0
  fi
  DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_DEVLOOP_ARTIFACT_DIGEST:-}}"
  if [[ -z "$DIGEST" ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP 'MAGELIFT_GCP_ACCEPTANCE_DIGEST or MAGELIFT_DEVLOOP_ARTIFACT_DIGEST is required for live product gates'
    exit 0
  fi
  if ! devloop_gcp_valid_immutable_digest "$DIGEST"; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP 'live product gate requires a non-placeholder immutable OCI digest (@sha256:...)'
    exit 0
  fi
  if [[ "${MAGELIFT_GCP_ACCEPTANCE_PROFILE:-preview}" != preview ||
        "${MAGELIFT_GCP_ACCEPTANCE_RUNTIME:-gke-autopilot}" != gke-autopilot ||
        ! "${MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY:-0}" =~ ^(0|false)$ ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL 'product proof requires the preview Autopilot application recipe (not infra-only)'
    exit 1
  fi
  if ! cmp -s \
    <(sed '/^[[:space:]]*#/d; /^[[:space:]]*$/d' "${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}") \
    <(sed '/^[[:space:]]*#/d; /^[[:space:]]*$/d' "$ROOT/scripts/acceptance/cells-gcp-preview.txt"); then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL 'product proof requires the complete ordered reference cell catalog'
    exit 1
  fi
  missing_tools=()
  for cmd in gcloud docker pulumi go kubectl gke-gcloud-auth-plugin curl openssl shasum jq; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing_tools+=("$cmd")
    fi
  done
  if [[ "${#missing_tools[@]}" -gt 0 ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product SKIP "missing prerequisite: ${missing_tools[*]}"
    exit 0
  fi
fi

ACCEPTANCE_NAME="${MAGELIFT_GCP_ACCEPTANCE_NAME:-$(devloop_gcp_default_acceptance_name)}"
if ! devloop_gcp_validate_acceptance_name "$ACCEPTANCE_NAME"; then
  devloop_append_gate "$GATE_FILE" gcp-product FAIL 'MAGELIFT_GCP_ACCEPTANCE_NAME is invalid for preview acceptance'
  exit 1
fi

mkdir -p "$EVIDENCE_DIR"
WORK="$(mktemp -d "$EVIDENCE_DIR/gcp-product-${SHA:0:12}.XXXXXX")"
LOG="$WORK/harness.log"
printf 'gcp-product-gate: private diagnostics %s\n' "$WORK"

export MAGELIFT_ACCEPTANCE_DRY_RUN="$DRY_RUN"
export MAGELIFT_ACCEPTANCE_PROVIDER=gcp
export MAGELIFT_ACCEPTANCE_ACCOUNT="${MAGELIFT_ACCEPTANCE_ACCOUNT:-devloop-gcp}"
export MAGELIFT_GCP_ACCEPTANCE_NAME="$ACCEPTANCE_NAME"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-gcp-preview.txt}"
export MAGELIFT_ACCEPTANCE_RUN_ID="${MAGELIFT_DEVLOOP_RUN_ID:-${MAGELIFT_ACCEPTANCE_RUN_ID:-devloop-${SHA:0:12}-$$}}"
export MAGELIFT_GCP_ACCEPTANCE_DIR="$WORK/harness"
export ACCEPTANCE_CHECKPOINT="$WORK/checkpoint.json"
export ACCEPTANCE_EVIDENCE="$WORK/matrix-results.md"
export ACCEPTANCE_SHARED_EVIDENCE="$WORK/acceptance-evidence.jsonl"

if devloop_gcp_is_dry_run; then
  export MAGELIFT_GCP_ACCEPTANCE_DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_DEVLOOP_ARTIFACT_DIGEST:-registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}}"
else
  export MAGELIFT_GCP_ACCEPTANCE=1
  export MAGELIFT_GCP_ACCEPTANCE_DIGEST="${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_DEVLOOP_ARTIFACT_DIGEST:?}}"
  export MAGELIFT_GCP_ACCEPTANCE_KEEP=false
  export MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE=false
  export MAGELIFT_GCP_ACCEPTANCE_RESUME=0
fi

HARNESS_ARGS=()
if ! devloop_gcp_is_dry_run; then
  HARNESS_ARGS=(up)
fi

set +e
bash "$HARNESS" "${HARNESS_ARGS[@]}" >"$LOG" 2>&1
rc=$?
set -e

if devloop_gcp_is_dry_run; then
  if [[ "$rc" -ne 0 ]]; then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp-acceptance-local.sh dry-run failed (see development-loop log)'
    exit 1
  fi
  if ! grep -q 'gcp acceptance dry-run ok' "$LOG"; then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp dry-run completion marker missing (see development-loop log)'
    exit 1
  fi
  if grep -Eq '\+ magelift (up|destroy)|gcloud .+ create|created=1' "$LOG"; then
    devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp dry-run performed a mutating operation'
    exit 1
  fi
  devloop_append_gate "$GATE_FILE" gcp-product SKIP \
    'GCP product gate dry-run completed; live acceptance not executed (MAGELIFT_ACCEPTANCE_DRY_RUN=1)'
  exit 0
fi

if [[ "$rc" -ne 0 ]]; then
  devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp-acceptance-local.sh live run failed (see development-loop log)'
  exit 1
fi
if ! devloop_gcp_verify_live_log "$LOG"; then
  devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp live harness did not complete mode=up with cleanup PASS (see development-loop log)'
  exit 1
fi
if ! devloop_gcp_verify_live_evidence; then
  devloop_append_gate "$GATE_FILE" gcp-product FAIL 'gcp live cell or cleanup evidence incomplete (see development-loop log)'
  exit 1
fi
devloop_append_gate "$GATE_FILE" gcp-product PASS ''
exit 0

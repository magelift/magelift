#!/usr/bin/env bash
# Measure wall-clock time from config validate through a preview-preset AWS deploy
# that is destroyed on exit. Record the result in docs/release-readiness.md only
# after a successful maintainer run; do not invent numbers.
#
# Prerequisites: magelift on PATH, AWS credentials, magelift.yaml with preview preset,
# bootstrap already completed for the account/region.
#
# Set MAGELIFT_TIME_TO_PREVIEW_KEEP=true to leave the stack up for further matrix
# tests (queueMode, searchMode, …). Destroy once when the session is fully done.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_NAME="${MAGELIFT_ENV:-preview}"
CONFIG="${MAGELIFT_CONFIG:-magelift.yaml}"
LOG="${MAGELIFT_TIME_TO_PREVIEW_LOG:-${ROOT}/.magelift/time-to-preview.log}"

mkdir -p "$(dirname "$LOG")"
START="$(date +%s)"
{
  echo "time-to-preview start $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "config=${CONFIG} env=${ENV_NAME}"
} | tee "$LOG"

cleanup() {
  local code=$?
  set +e
  if [[ "${MAGELIFT_TIME_TO_PREVIEW_KEEP:-}" == true ]]; then
    echo "keeping stack (MAGELIFT_TIME_TO_PREVIEW_KEEP=true); skip destroy" | tee -a "$LOG"
  else
    magelift destroy --env "$ENV_NAME" --config "$CONFIG" --yes >>"$LOG" 2>&1
  fi
  END="$(date +%s)"
  ELAPSED=$((END - START))
  {
    echo "time-to-preview end $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "elapsed_seconds=${ELAPSED}"
    echo "exit_code=${code}"
    echo "kept=${MAGELIFT_TIME_TO_PREVIEW_KEEP:-false}"
  } | tee -a "$LOG"
  exit "$code"
}
trap cleanup EXIT

magelift doctor --config "$CONFIG" 2>&1 | tee -a "$LOG"
magelift config validate --env "$ENV_NAME" --config "$CONFIG" 2>&1 | tee -a "$LOG"
magelift preview --env "$ENV_NAME" --config "$CONFIG" 2>&1 | tee -a "$LOG"
magelift deploy --env "$ENV_NAME" --config "$CONFIG" --yes 2>&1 | tee -a "$LOG"
magelift outputs --env "$ENV_NAME" --config "$CONFIG" 2>&1 | tee -a "$LOG"
magelift health --env "$ENV_NAME" --config "$CONFIG" 2>&1 | tee -a "$LOG"

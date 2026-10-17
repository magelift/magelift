#!/usr/bin/env bash
# Exercise existing harness ownership helpers with cloud command doubles only.
# Variables are consumed by dynamically extracted harness functions.
# shellcheck disable=SC1090,SC2034
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HARNESS="$ROOT/providers/gcp/scripts/gcp-acceptance-local.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
for function in ensure_acceptance_state_bucket remove_acceptance_state_bucket ensure_acceptance_encryption_key cleanup_acceptance_encryption_key; do
  sed -n "/^${function}() {/,/^}/p" "$HARNESS" > "$TMP/$function.sh"
  [[ -s "$TMP/$function.sh" ]]
  source "$TMP/$function.sh"
done
unset MAGELIFT_GCP_ENCRYPTION_KEY_SECRET_ID
PROJECT=example-gcp-project
REGION=us-central1
NAME=magelift-test
PROFILE=preview
STATE_BUCKET_NAME=magelift-test-state
ENCRYPTION_KEY_SECRET_ID=magelift-test-key
AUTO_STATE_BUCKET=1
bucket_exists=1; secret_exists=1
STATE_BUCKET_ACCEPTANCE_OWNED=0
ENCRYPTION_KEY_ACCEPTANCE_OWNED=0
trace="$TMP/trace"
: > "$trace"
gcloud() {
  printf '%s\n' "$*" >> "$trace"
  case "$*" in
    'storage buckets describe '*) [[ "$bucket_exists" == 1 ]] ;;
    'storage buckets create '*) bucket_exists=1 ;;
    'storage rm '*) bucket_exists=0 ;;
    'secrets describe '*) [[ "$secret_exists" == 1 ]] ;;
    'secrets create '*) secret_exists=1 ;;
    'secrets versions add '*) cat >/dev/null ;;
    'secrets delete '*) secret_exists=0 ;;
    *) printf 'unexpected cloud operation\n' >&2; return 99 ;;
  esac
}
openssl() { printf 'synthetic-key-fixture'; }

# A name collision never transfers ownership or permits deletion.
if ensure_acceptance_state_bucket; then printf 'pre-existing bucket was implicitly adopted\n' >&2; exit 1; fi
remove_acceptance_state_bucket
[[ "$STATE_BUCKET_ACCEPTANCE_OWNED" == 0 && "$bucket_exists" == 1 ]]
if ensure_acceptance_encryption_key; then printf 'pre-existing key was implicitly adopted\n' >&2; exit 1; fi
cleanup_acceptance_encryption_key
[[ "$ENCRYPTION_KEY_ACCEPTANCE_OWNED" == 0 && "$secret_exists" == 1 ]]
if grep -Eq 'storage rm|secrets delete' "$trace"; then exit 1; fi

# Explicit external references remain external, including during cleanup.
MAGELIFT_GCP_ENCRYPTION_KEY_SECRET_ID=external-fixture
ensure_acceptance_encryption_key
cleanup_acceptance_encryption_key
[[ "$ENCRYPTION_KEY_ACCEPTANCE_OWNED" == 0 && "$secret_exists" == 1 ]]
unset MAGELIFT_GCP_ENCRYPTION_KEY_SECRET_ID
AUTO_STATE_BUCKET=0
ensure_acceptance_state_bucket
remove_acceptance_state_bucket
[[ "$bucket_exists" == 1 ]]

# Only resources created successfully by this run may be removed.
AUTO_STATE_BUCKET=1
bucket_exists=0; secret_exists=0
ensure_acceptance_state_bucket
ensure_acceptance_encryption_key
[[ "$STATE_BUCKET_ACCEPTANCE_OWNED" == 1 && "$ENCRYPTION_KEY_ACCEPTANCE_OWNED" == 1 ]]
remove_acceptance_state_bucket
cleanup_acceptance_encryption_key
[[ "$bucket_exists" == 0 && "$secret_exists" == 0 ]]
# Prefix collisions and unavailable inventories fail before any adoption.
(
  for function in prefix_match assert_acceptance_prefix_fresh; do
    source <(sed -n "/^${function}() {/,/^}/p" "$HARNESS")
  done
  MAGELIFT_GCP_ACCEPTANCE_RESUME=0
  assert_clean() { [[ "${RUNTIME_INVENTORY_FAILURE:-0}" == 0 ]]; }
  gcloud() {
    [[ "${INVENTORY_FAILURE:-0}" == 0 ]] || return 13
    case "$*" in
      'iam workload-identity-pools list '*) printf '%s\n' "${POOL_LISTING:-}" ;;
      'iam service-accounts list '*) printf '%s\n' "${ACCOUNT_LISTING:-}" ;;
      *) return 99 ;;
    esac
  }
  assert_acceptance_prefix_fresh
  if POOL_LISTING="projects/fixture/locations/global/workloadIdentityPools/ml-${NAME}-${PROFILE}" assert_acceptance_prefix_fresh; then exit 1; fi
  if ACCOUNT_LISTING="ml-${NAME}-${PROFILE}-ci@example.invalid" assert_acceptance_prefix_fresh; then exit 1; fi
  if INVENTORY_FAILURE=1 assert_acceptance_prefix_fresh; then exit 1; fi
  if RUNTIME_INVENTORY_FAILURE=1 assert_acceptance_prefix_fresh; then exit 1; fi
  POOL_LISTING=unrelated-pool ACCOUNT_LISTING=unrelated-account assert_acceptance_prefix_fresh
)

# Cleanup must retain state, key and passphrase on runtime/auxiliary failure.
sed -n '/^cleanup() {/,/^}/p' "$HARNESS" > "$TMP/cleanup.sh"
for outcome in pass runtime-fail auxiliary-fail; do
  : > "$trace"
  rc=0
  (
    source "$TMP/cleanup.sh"
    MAGELIFT_GCP_ACCEPTANCE_KEEP=false
    ORIGIN_DNS_CLAIMED=0; created=0; ORIGIN_KUBECONFIG=''
    ACCEPTANCE_EVIDENCE_RUN_ID=fixture
    STATE_BUCKET_ACCEPTANCE_OWNED=1
    acceptance_stop_ttl_watchdog() { :; }
    acceptance_ttl_expired() { return 1; }
    assert_clean() { printf 'inventory\n' >> "$trace"; [[ "$outcome" != runtime-fail ]]; }
    cleanup_acceptance_wif() { printf 'identity-cleanup\n' >> "$trace"; [[ "$outcome" != auxiliary-fail ]]; }
    cleanup_acceptance_encryption_key() { printf 'key-cleanup\n' >> "$trace"; }
    remove_acceptance_state_bucket() { printf 'state-cleanup\n' >> "$trace"; }
    cleanup_acceptance_passphrase() { printf 'passphrase-cleanup\n' >> "$trace"; }
    append_shared_cleanup() { printf 'proof=%s\n' "$1" >> "$trace"; }
    true
    cleanup
  ) || rc=$?
  [[ "$(head -n 1 "$trace")" == inventory ]]
  if [[ "$outcome" == pass ]]; then
    [[ "$rc" -eq 0 ]]
    grep -q 'state-cleanup' "$trace"
    grep -q 'passphrase-cleanup' "$trace"
    grep -q 'proof=PASS' "$trace"
  else
    [[ "$rc" -ne 0 ]]
    if grep -Eq 'state-cleanup|passphrase-cleanup' "$trace"; then exit 1; fi
    grep -q 'proof=FAIL' "$trace"
    if [[ "$outcome" == runtime-fail ]] && grep -q 'key-cleanup' "$trace"; then exit 1; fi
  fi
done
printf 'gcp_acceptance_ownership_test OK (doubles only)\n' 

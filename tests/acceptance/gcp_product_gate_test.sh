#!/usr/bin/env bash
# Offline gate control-flow doubles. These are not real GCP product proofs.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"
for tool in gcloud docker pulumi go kubectl gke-gcloud-auth-plugin curl openssl; do
  printf '#!/bin/sh\nexit 99\n' > "$TMP/bin/$tool"
  chmod +x "$TMP/bin/$tool"
done
cat > "$TMP/harness.sh" <<'DOUBLE'
#!/usr/bin/env bash
set -euo pipefail
jq -n --arg mode "${1:-preview}" --arg name "$MAGELIFT_GCP_ACCEPTANCE_NAME" --arg checkpoint "$ACCEPTANCE_CHECKPOINT" '{mode:$mode,name:$name,checkpoint:$checkpoint}' > "$FAKE_TRACE"
printf 'private-diagnostic-fixture\n'
if [[ "${FAKE_FAILURE:-0}" == 1 ]]; then exit 8; fi
if [[ "$MAGELIFT_ACCEPTANCE_DRY_RUN" == 1 ]]; then
  printf 'gcp acceptance dry-run ok; created=0\n'; exit 0
fi
[[ "${1:-}" == up ]]
printf 'gcp acceptance live_cell_loop cells=13\n'
printf 'gcp acceptance ok mode=up\nassert_clean ok\n'
while IFS= read -r cell || [[ -n "$cell" ]]; do
  [[ -z "$cell" || "$cell" =~ ^[[:space:]]*# ]] && continue
  printf '| %s | PASS | 0s | gcp | fixture | 2026-10-17 |\n' "$cell"
done < "$MAGELIFT_ACCEPTANCE_CELL_CATALOG" > "$ACCEPTANCE_EVIDENCE"
if [[ "${FAKE_CLEANUP:-PASS}" == missing ]]; then exit 0; fi
if [[ "${FAKE_CLEANUP:-PASS}" == stale ]]; then
  jq -n '{type:"cleanup",runId:"old-run",status:"PASS",cleanup:{prefix:"old-preview"}}' > "$ACCEPTANCE_SHARED_EVIDENCE"
fi
status=PASS; prefix="${MAGELIFT_GCP_ACCEPTANCE_NAME}-preview"; remaining='[]'
case "${FAKE_CLEANUP:-PASS}" in
  FAIL | stale) status=FAIL ;;
  foreign) prefix=foreign-preview ;;
  remaining) remaining='["leftover-fixture"]' ;;
esac
jq -nc --arg status "$status" --arg run "$MAGELIFT_ACCEPTANCE_RUN_ID" --arg prefix "$prefix" --argjson remaining "$remaining" \
  '{type:"cleanup",runId:$run,status:$status,cleanup:{prefix:$prefix,remaining:$remaining}}' >> "$ACCEPTANCE_SHARED_EVIDENCE"
DOUBLE
sha=0123456789abcdef0123456789abcdef01234567
digest="example.invalid/shop@sha256:$(printf '1%.0s' {1..64})"
common=(BASH_ENV= "PATH=$TMP/bin:$PATH" "MAGELIFT_DEVLOOP_GCP_HARNESS=$TMP/harness.sh"
  MAGELIFT_GCP_ACCEPTANCE=1 MAGELIFT_GCP_PROJECT=example-gcp-project
  "MAGELIFT_GCP_ACCEPTANCE_DIGEST=$digest" MAGELIFT_DEVLOOP_ARTIFACT_DIGEST=
  MAGELIFT_GCP_ACCEPTANCE_KEEP=false MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE=false
  MAGELIFT_GCP_ACCEPTANCE_RESUME=0 MAGELIFT_GCP_ACCEPTANCE_NAME=
  MAGELIFT_GCP_ACCEPTANCE_PROFILE=preview MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-autopilot
  MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY=0
  "MAGELIFT_ACCEPTANCE_CELL_CATALOG=$ROOT/scripts/acceptance/cells-gcp-preview.txt")
run_case() {
  local name="$1" expected="$2" rc=0
  shift 2
  env "${common[@]}" MAGELIFT_ACCEPTANCE_DRY_RUN=0 "MAGELIFT_DEVLOOP_EVIDENCE_DIR=$TMP/$name" \
    "FAKE_TRACE=$TMP/$name.trace" "$@" bash "$ROOT/scripts/development-loop/gcp-product-gate.sh" \
    --sha "$sha" --gate-file "$TMP/$name.jsonl" > "$TMP/$name.out" 2>&1 || rc=$?
  jq -es --arg expected "$expected" 'length == 1 and .[0].status == $expected' "$TMP/$name.jsonl" >/dev/null
  if [[ "$expected" == FAIL ]]; then [[ "$rc" -ne 0 ]]; else [[ "$rc" -eq 0 ]]; fi
  if grep -q 'private-diagnostic-fixture' "$TMP/$name.jsonl"; then exit 1; fi
}
run_case dry SKIP MAGELIFT_ACCEPTANCE_DRY_RUN=1
run_case dry-failure FAIL MAGELIFT_ACCEPTANCE_DRY_RUN=1 FAKE_FAILURE=1
run_case missing-project SKIP MAGELIFT_GCP_PROJECT=
run_case missing-digest SKIP MAGELIFT_GCP_ACCEPTANCE_DIGEST=
run_case placeholder SKIP "MAGELIFT_GCP_ACCEPTANCE_DIGEST=example.invalid/shop@sha256:$(printf 'a%.0s' {1..64})"
run_case malformed SKIP "MAGELIFT_GCP_ACCEPTANCE_DIGEST=bad image@sha256:$(printf '1%.0s' {1..64})"
run_case keep FAIL MAGELIFT_GCP_ACCEPTANCE_KEEP=true
run_case resume FAIL MAGELIFT_GCP_ACCEPTANCE_RESUME=1
run_case long-name FAIL MAGELIFT_GCP_ACCEPTANCE_NAME=magelift-overlong-fixture
run_case infra-only FAIL MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY=1
run_case standard FAIL MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-standard
run_case costly FAIL MAGELIFT_GCP_ACCEPTANCE_PROFILE=standard
printf 'bootstrap:wif\n' > "$TMP/short-catalog"
run_case partial-catalog FAIL "MAGELIFT_ACCEPTANCE_CELL_CATALOG=$TMP/short-catalog"
for cleanup in FAIL missing stale foreign remaining; do run_case "cleanup-$cleanup" FAIL "FAKE_CLEANUP=$cleanup"; done
for attempt in 1 2; do
  run_case "live-$attempt" PASS
  jq -e '.mode == "up" and (.name | test("^magelift-[0-9a-f]{7}$"))' "$TMP/live-$attempt.trace" >/dev/null
done
[[ "$(jq -r .name "$TMP/live-1.trace")" != "$(jq -r .name "$TMP/live-2.trace")" ]]
[[ "$(jq -r .checkpoint "$TMP/live-1.trace")" != "$(jq -r .checkpoint "$TMP/live-2.trace")" ]]
grep -q 'private-diagnostic-fixture' "$TMP/live-1"/*/harness.log
[[ -z "$(find "$TMP/live-1" -name harness.log -perm -004)" ]]
printf 'gcp_product_gate_test OK (doubles only)\n'

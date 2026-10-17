#!/usr/bin/env bash
# Runner control-flow doubles: no provider, network, or cloud calls.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/scripts/development-loop" "$TMP/scripts/acceptance"
cp "$ROOT/scripts/development-loop/"{run-gates,lib-gates,lib-canary-identity}.sh "$TMP/scripts/development-loop/"
cp "$ROOT/scripts/acceptance/lib-evidence.sh" "$TMP/scripts/acceptance/"
for gate in distribution artifact gcp-product; do
  cat > "$TMP/scripts/development-loop/$gate-gate.sh" <<'DOUBLE'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
source "$ROOT/scripts/development-loop/lib-gates.sh"
gate="$(basename "$0" -gate.sh)"
while [[ $# -gt 0 ]]; do
  case "$1" in --gate-file) file="$2"; shift 2 ;; *) shift ;; esac
done
if [[ "$gate" == distribution ]]; then
  case "${RUNNER_FAILURE:-crash}" in
    crash) printf 'private diagnostic fixture\n' >&2; exit 23 ;;
    malformed) printf '{partial' > "$file"; exit 0 ;;
    false-pass) devloop_append_gate "$file" "$gate" PASS ''; exit 17 ;;
    recorded-fail) devloop_append_gate "$file" "$gate" FAIL 'synthetic failure'; exit 0 ;;
  esac
fi
devloop_append_gate "$file" "$gate" PASS ''
DOUBLE
 done
sha=0123456789abcdef0123456789abcdef01234567
for failure in crash malformed false-pass recorded-fail none; do
  status=0
  RUNNER_FAILURE="$failure" MAGELIFT_DEVLOOP_EVIDENCE_DIR="$TMP/$failure" \
    MAGELIFT_DEVLOOP_SHARED_EVIDENCE="$TMP/$failure/shared.jsonl" \
    bash "$TMP/scripts/development-loop/run-gates.sh" --sha "$sha" --gate-file "$TMP/$failure/result.jsonl" >"$TMP/output" 2>&1 || status=$?
  if [[ "$failure" == none ]]; then
    [[ "$status" -eq 0 ]]
    expected=PASS
  else
    [[ "$status" -ne 0 ]]
    expected=FAIL
  fi
  jq -es --arg expected "$expected" '
    length == 3 and
    (map(select(.gate == "distribution"))[0].status == $expected) and
    (map(select(.gate == "artifact"))[0].status == "PASS") and
    (map(select(.gate == "gcp-product"))[0].status == "PASS")
  ' "$TMP/$failure/result.jsonl" >/dev/null
  if grep -q 'private diagnostic fixture' "$TMP/$failure/result.jsonl"; then
    printf 'raw diagnostic leaked into gate results\n' >&2; exit 1
  fi
  [[ "$(find "$TMP/$failure" -name distribution.log | wc -l)" -eq 1 ]]
done
printf 'development_loop_runner_test OK (doubles only)\n'

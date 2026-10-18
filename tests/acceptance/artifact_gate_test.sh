#!/usr/bin/env bash
# Artifact contract doubles distinguish an offline check from an image build.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/scripts/development-loop" "$TMP/bin"
cp "$ROOT/scripts/development-loop/"{artifact-gate,lib-gates,lib-canary-identity}.sh "$TMP/scripts/development-loop/"
cp "$ROOT/scripts/go-memlimit.sh" "$TMP/scripts/"
printf '#!/usr/bin/env bash\nexit "${ARTIFACT_GO_EXIT:-0}"\n' > "$TMP/bin/go"
printf '#!/usr/bin/env bash\nexit "${ARTIFACT_PHP_EXIT:-0}"\n' > "$TMP/scripts/php-test.sh"
chmod +x "$TMP/bin/go"
sha=0123456789abcdef0123456789abcdef01234567
for scenario in pass php-error go-error; do
  php_exit=0; go_exit=0; expected=SKIP
  case "$scenario" in php-error) php_exit=2; expected=FAIL ;; go-error) go_exit=9; expected=FAIL ;; esac
  status=0
  BASH_ENV='' PATH="$TMP/bin:$PATH" ARTIFACT_PHP_EXIT="$php_exit" ARTIFACT_GO_EXIT="$go_exit" \
    MAGELIFT_DEVLOOP_ARTIFACT_DIGEST='' MAGELIFT_DEVLOOP_EVIDENCE_DIR="$TMP/$scenario" \
    bash "$TMP/scripts/development-loop/artifact-gate.sh" --sha "$sha" --gate-file "$TMP/$scenario.jsonl" || status=$?
  jq -es --arg expected "$expected" 'length == 1 and .[0].status == $expected' "$TMP/$scenario.jsonl" >/dev/null
  if [[ "$expected" == FAIL ]]; then [[ "$status" -ne 0 ]]; else [[ "$status" -eq 0 ]]; fi
done
jq -e '.detail | contains("exit 2")' "$TMP/php-error.jsonl" >/dev/null
jq -e '.detail | contains("image build NOT_RUN")' "$TMP/pass.jsonl" >/dev/null
printf 'artifact_gate_test OK (doubles only)\n'

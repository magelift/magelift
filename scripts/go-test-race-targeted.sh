#!/usr/bin/env bash
# Race-sensitive paths in the CLI, SDK and providers.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=go-memlimit.sh
source "$ROOT/scripts/go-memlimit.sh"
magelift_apply_go_memlimit
cd "$ROOT"

run() {
  printf '+ test-race: %s\n' "$*"
  "$@"
}

status=0

if ! run go test -race ./internal/lintcoverage/ ./internal/usererr/ -count=1; then
  status=1
fi
if ! run go test -race ./internal/providerhost/... -count=1; then
  status=1
fi
if ! run go test -race ./internal/cli/ ./internal/platform/ -count=1; then
  status=1
fi
if ! run bash "$ROOT/scripts/go-test-modules.sh" --race --only sdk; then
  status=1
fi
for provider_dir in "$ROOT"/providers/*; do
  [[ -f "$provider_dir/go.mod" ]] || continue
  packages=(./plugin/)
  if [[ -d "$provider_dir/operations" ]]; then packages+=(./operations/); fi
  if ! (cd "$provider_dir" && run go test -race "${packages[@]}" -count=1); then
    status=1
  fi
done

exit "$status"

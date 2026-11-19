#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
source "$root/scripts/go-memlimit.sh"
magelift_apply_go_memlimit
compose=(docker compose --project-name magelift-floci-azure -f providers/azure/docker-compose.floci.yml)
cleanup() {
  "${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT
"${compose[@]}" up -d --wait
MAGELIFT_FLOCI_AZURE=1 MAGELIFT_AZURE_ENDPOINT="${MAGELIFT_AZURE_ENDPOINT:-http://127.0.0.1:4577}" \
  go test -race -tags=floci ./providers/azure/floci -count=1
MAGELIFT_FLOCI_AZURE=1 MAGELIFT_AZURE_ENDPOINT="${MAGELIFT_AZURE_ENDPOINT:-http://127.0.0.1:4577}" \
  go test -race -tags=floci ./providers/azure/plugin -run '^TestAzureFloci' -count=1

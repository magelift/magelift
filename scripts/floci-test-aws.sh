#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=go-memlimit.sh
source "$root/scripts/go-memlimit.sh"
magelift_apply_go_memlimit

compose=(docker compose -f docker-compose.floci.yml)
cleanup() {
	"${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up -d --wait
MAGELIFT_FLOCI=1 MAGELIFT_FLOCI_ENDPOINT="${MAGELIFT_FLOCI_ENDPOINT:-http://localhost:4566}" \
go test -race -tags=floci ./tests/floci -count=1

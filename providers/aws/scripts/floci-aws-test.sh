#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
# shellcheck source=../../../scripts/go-memlimit.sh
source "$root/scripts/go-memlimit.sh"
magelift_apply_go_memlimit

cd "$root"
compose=(docker compose --project-name magelift-floci-aws -f providers/aws/docker-compose.floci.yml)
cleanup() {
	"${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up -d --wait
MAGELIFT_FLOCI_AWS=1 MAGELIFT_AWS_ENDPOINT_URL="${MAGELIFT_AWS_ENDPOINT_URL:-http://127.0.0.1:4566}" \
go test -race -tags=floci ./providers/aws/floci -count=1

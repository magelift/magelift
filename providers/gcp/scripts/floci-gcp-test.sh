#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
# shellcheck source=../../../scripts/go-memlimit.sh
source "$root/scripts/go-memlimit.sh"
magelift_apply_go_memlimit

cd "$root"
compose=(docker compose --project-name magelift-floci-gcp -f docker-compose.floci-gcp.yml)
cleanup() {
	"${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up -d --wait
export MAGELIFT_FLOCI_GCP=1
export MAGELIFT_GCP_ENDPOINT_URL="${MAGELIFT_GCP_ENDPOINT_URL:-http://127.0.0.1:4588}"
export STORAGE_EMULATOR_HOST="${STORAGE_EMULATOR_HOST:-127.0.0.1:4588}"
export SECRET_MANAGER_EMULATOR_HOST="${SECRET_MANAGER_EMULATOR_HOST:-127.0.0.1:4588}"
export PUBSUB_EMULATOR_HOST="${PUBSUB_EMULATOR_HOST:-127.0.0.1:4588}"
export GOOGLE_CLOUD_PROJECT="${GOOGLE_CLOUD_PROJECT:-floci-local}"
go test -race -tags=floci_gcp ./providers/gcp/floci/floci-gcp -count=1
go test -tags=floci_gcp ./providers/gcp/plugin -run '^TestGCPFlociPreviewSweepUsesSharedBucket$' -count=1

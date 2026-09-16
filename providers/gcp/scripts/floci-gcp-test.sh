#!/usr/bin/env bash
set -Eeuo pipefail

export GOMAXPROCS=1
export GOFLAGS=-p=1
export GOMEMLIMIT=1GiB

compose=(docker compose -f docker-compose.floci-gcp.yml)
cleanup() {
	"${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up -d --wait
MAGELIFT_FLOCI_GCP=1 \
MAGELIFT_GCP_ENDPOINT_URL="${MAGELIFT_GCP_ENDPOINT_URL:-http://127.0.0.1:4588}" \
STORAGE_EMULATOR_HOST="${STORAGE_EMULATOR_HOST:-127.0.0.1:4588}" \
SECRET_MANAGER_EMULATOR_HOST="${SECRET_MANAGER_EMULATOR_HOST:-127.0.0.1:4588}" \
PUBSUB_EMULATOR_HOST="${PUBSUB_EMULATOR_HOST:-127.0.0.1:4588}" \
GOOGLE_CLOUD_PROJECT="${GOOGLE_CLOUD_PROJECT:-floci-local}" \
go test -race -tags=floci_gcp ./providers/gcp/floci/floci-gcp -count=1

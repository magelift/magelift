#!/usr/bin/env bash
set -Eeuo pipefail

export GOMAXPROCS=1
export GOFLAGS=-p=1
export GOMEMLIMIT=1GiB

compose=(docker compose -f docker-compose.floci.yml)
cleanup() {
	"${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up -d --wait
MAGELIFT_FLOCI=1 MAGELIFT_FLOCI_ENDPOINT="${MAGELIFT_FLOCI_ENDPOINT:-http://localhost:4566}" \
go test -race -tags=floci ./tests/floci -count=1

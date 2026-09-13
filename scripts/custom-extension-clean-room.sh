#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/magelift-extension.XXXXXX")
cleanup() {
	status=$?
	if [[ -d "$scratch" ]]; then
		/usr/bin/find "$scratch" -type f -exec chmod u+rw {} + 2>/dev/null || true
		/usr/bin/find "$scratch" -type d -exec chmod u+rwx {} + 2>/dev/null || true
		/usr/bin/find "$scratch" -depth -delete 2>/dev/null || true
	fi
	exit "$status"
}
trap cleanup EXIT

export GOMODCACHE="$scratch/modcache"
export GOCACHE="$scratch/gocache"
export GOMAXPROCS=1
export GOFLAGS=-p=1
export GOMEMLIMIT=1GiB
export GOGC=50

cd "$repo_root"
printf '+ building the public extension SDK contract with empty caches\n'
go build -trimpath -o "$scratch/magelift-extension-contract" ./examples/custom-extension-contract
"$scratch/magelift-extension-contract" | rg '^example\.community-contract$'

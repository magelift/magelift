#!/usr/bin/env bash
# go-memlimit: 75% of available RAM, no GOMAXPROCS / -p pin.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/go-memlimit.sh"

got="$(MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB=1048576 MAGELIFT_GOMEMLIMIT_PERCENT=75 bash "$SCRIPT")"
if [[ "$got" != "768MiB" ]]; then
	printf '75%% of 1GiB available: got %s want 768MiB\n' "$got" >&2
	exit 1
fi

got="$(MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB=1048576 MAGELIFT_GOMEMLIMIT_PERCENT=50 bash "$SCRIPT")"
if [[ "$got" != "512MiB" ]]; then
	printf '50%% of 1GiB available: got %s want 512MiB\n' "$got" >&2
	exit 1
fi

got="$(MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB=1000 MAGELIFT_GOMEMLIMIT_PERCENT=75 bash "$SCRIPT")"
if [[ "$got" != "256MiB" ]]; then
	printf 'floor: got %s want 256MiB\n' "$got" >&2
	exit 1
fi

if MAGELIFT_GOMEMLIMIT_PERCENT=0 bash "$SCRIPT" >/dev/null 2>&1; then
	printf 'PERCENT=0 must fail\n' >&2
	exit 1
fi

# shellcheck source=../../scripts/go-memlimit.sh
source "$SCRIPT"
export GOMAXPROCS=1
export GOFLAGS='-race -p=1'
export MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB=1048576
export MAGELIFT_GOMEMLIMIT_PERCENT=75
magelift_apply_go_memlimit
if [[ -n "${GOMAXPROCS+x}" ]]; then
	printf 'apply must unset GOMAXPROCS, got %s\n' "${GOMAXPROCS-}" >&2
	exit 1
fi
if [[ "${GOFLAGS:-}" != "-race" ]]; then
	printf 'apply must drop -p=1 and keep other GOFLAGS, got %s\n' "${GOFLAGS-}" >&2
	exit 1
fi
if [[ "$GOMEMLIMIT" != "768MiB" ]]; then
	printf 'apply GOMEMLIMIT=%s want 768MiB\n' "$GOMEMLIMIT" >&2
	exit 1
fi

printf 'go_memlimit_test OK\n'

#!/usr/bin/env bash
# Packed-campaign isolation: unique prefixes, serial Go, no inherited Pulumi backend.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

unset GOMAXPROCS GOFLAGS GOMEMLIMIT PULUMI_BACKEND_URL
acceptance_campaign_serial_go
if [[ "$GOMAXPROCS" != 1 || "$GOFLAGS" != "-p=1" || "$GOMEMLIMIT" != "1GiB" ]]; then
	printf 'serial Go defaults = GOMAXPROCS=%s GOFLAGS=%s GOMEMLIMIT=%s\n' "$GOMAXPROCS" "$GOFLAGS" "$GOMEMLIMIT" >&2
	exit 1
fi

export GOFLAGS="-race"
acceptance_campaign_serial_go >/dev/null
if [[ "$GOFLAGS" != "-race -p=1" ]]; then
	printf 'serial Go must append -p=1 to existing GOFLAGS, got %s\n' "$GOFLAGS" >&2
	exit 1
fi

if ! acceptance_campaign_prefix_is_reserved '' || ! acceptance_campaign_prefix_is_reserved mlacc || ! acceptance_campaign_prefix_is_reserved shared; then
	printf 'reserved prefixes must be rejected\n' >&2
	exit 1
fi
if acceptance_campaign_prefix_is_reserved gcap29 || acceptance_campaign_prefix_is_reserved awsba; then
	printf 'unique campaign prefixes must be allowed\n' >&2
	exit 1
fi

if acceptance_campaign_require_prefix aws '' 2>/dev/null; then
	printf 'empty AWS prefix must fail\n' >&2
	exit 1
fi
if acceptance_campaign_require_prefix gcp mlacc 2>/dev/null; then
	printf 'reserved GCP prefix mlacc must fail\n' >&2
	exit 1
fi
acceptance_campaign_require_prefix aws awsba >/dev/null
acceptance_campaign_require_prefix gcp gcap29 >/dev/null
acceptance_campaign_require_prefix ovh ovh812 >/dev/null
acceptance_campaign_require_prefix scaleway scwrc1 >/dev/null

PULUMI_BACKEND_URL='s3://sibling-worktree-state'
if acceptance_campaign_require_isolated_backend '' 2>/dev/null; then
	printf 'inherited Pulumi backend must fail without an explicit override\n' >&2
	exit 1
fi
acceptance_campaign_require_isolated_backend 's3://this-worktree-state'
unset PULUMI_BACKEND_URL
acceptance_campaign_require_isolated_backend ''

for script in aws-acceptance-local.sh gcp-acceptance-local.sh k8s-acceptance-local.sh; do
	path="$ROOT/scripts/$script"
	for required in 'lib-campaign-isolation.sh' 'acceptance_campaign_serial_go'; do
		if ! grep -q "$required" "$path"; then
			printf '%s missing %s\n' "$script" "$required" >&2
			exit 1
		fi
	done
done
if ! grep -q 'acceptance_campaign_require_prefix aws' "$ROOT/scripts/aws-acceptance-local.sh"; then
	printf 'AWS live path must require a unique campaign prefix\n' >&2
	exit 1
fi
if ! grep -q 'acceptance_campaign_require_prefix gcp' "$ROOT/scripts/gcp-acceptance-local.sh"; then
	printf 'GCP live path must require a unique campaign prefix\n' >&2
	exit 1
fi
if ! grep -q 'acceptance_campaign_require_prefix "$PROVIDER"' "$ROOT/scripts/k8s-acceptance-local.sh"; then
	printf 'OVH/Scaleway live path must require a unique campaign prefix\n' >&2
	exit 1
fi

printf 'campaign_isolation_test OK\n'

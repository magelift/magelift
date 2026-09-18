#!/usr/bin/env bash
# Packed-campaign isolation: unique prefixes, GOMEMLIMIT 75%, no inherited Pulumi backend.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

unset GOMAXPROCS GOFLAGS GOMEMLIMIT PULUMI_BACKEND_URL
export MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB=1048576
export MAGELIFT_GOMEMLIMIT_PERCENT=75
acceptance_campaign_go_memlimit
if [[ -n "${GOMAXPROCS+x}" ]]; then
	printf 'campaign isolation must unset GOMAXPROCS, got %s\n' "${GOMAXPROCS-}" >&2
	exit 1
fi
if [[ "$GOMEMLIMIT" != "768MiB" ]]; then
	printf 'campaign isolation GOMEMLIMIT=%s want 768MiB\n' "$GOMEMLIMIT" >&2
	exit 1
fi

export GOFLAGS="-race -p=1"
acceptance_campaign_go_memlimit >/dev/null
if [[ "${GOFLAGS:-}" != "-race" ]]; then
	printf 'campaign isolation must drop -p=1 and keep other GOFLAGS, got %s\n' "${GOFLAGS-}" >&2
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

for script in scripts/aws-acceptance-local.sh providers/gcp/scripts/gcp-acceptance-local.sh scripts/k8s-acceptance-local.sh; do
	path="$ROOT/$script"
	for required in 'lib-campaign-isolation.sh' 'acceptance_campaign_go_memlimit'; do
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
if ! grep -q 'acceptance_campaign_require_prefix gcp' "$ROOT/providers/gcp/scripts/gcp-acceptance-local.sh"; then
	printf 'GCP live path must require a unique campaign prefix\n' >&2
	exit 1
fi
if ! grep -q 'acceptance_campaign_require_prefix "$PROVIDER"' "$ROOT/scripts/k8s-acceptance-local.sh"; then
	printf 'OVH/Scaleway live path must require a unique campaign prefix\n' >&2
	exit 1
fi

printf 'campaign_isolation_test OK\n'

#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKFLOW="$ROOT/.github/workflows/ci.yml"
fail=0

# cloud|build-tag|test-package|test-root|contract-test
MAPPINGS=(
	"aws|floci|./providers/aws/floci|providers/aws/floci|TestBootstrapStateAndLockAgainstFloci"
	"gcp|floci|./providers/gcp/floci|providers/gcp/floci|TestObjectWriteAndReadAgainstFlociGCP"
	"azure|floci|./providers/azure/floci|providers/azure|TestAzureFlociBootstrapCreatesStateContainer"
)

for row in "${MAPPINGS[@]}"; do
	IFS='|' read -r cloud tag package test_root symbol <<<"$row"
	script="$ROOT/providers/$cloud/scripts/floci-$cloud-test.sh"
	compose="$ROOT/providers/$cloud/docker-compose.floci.yml"
	target="floci-$cloud-test"

	if ! grep -R --include='*_test.go' -l -F "func $symbol" "$ROOT/$test_root" >/dev/null 2>&1; then
		printf 'missing Floci contract test for %s: %s in %s\n' "$cloud" "$symbol" "$package" >&2
		fail=1
	fi
	if ! grep -R --include='*_test.go' -l -F "//go:build $tag" "$ROOT/$test_root" >/dev/null 2>&1; then
		printf 'missing %s build tag in %s\n' "$tag" "$package" >&2
		fail=1
	fi
	if [[ ! -x "$script" || ! -f "$compose" ]]; then
		printf 'missing Floci runner or Compose file for %s\n' "$cloud" >&2
		fail=1
		continue
	fi
	if ! grep -Fq "providers/$cloud/docker-compose.floci.yml" "$script"; then
		printf 'Floci %s runner does not use its provider Compose file\n' "$cloud" >&2
		fail=1
	fi
	if ! grep -Fq -- "-tags=$tag $package" "$script"; then
		printf 'Floci %s runner does not test %s with tag %s\n' "$cloud" "$package" "$tag" >&2
		fail=1
	fi
	if ! make --no-print-directory -n "$target" | grep -Fq "providers/$cloud/scripts/floci-$cloud-test.sh"; then
		printf 'Make target %s does not call the provider Floci runner\n' "$target" >&2
		fail=1
	fi

	compose_image="$(awk '/^[[:space:]]*image:/ {print $2; exit}' "$compose")"
	ci_block="$(awk -v heading="  floci-$cloud:" '$0 == heading {inside=1; next} /^  [[:alnum:]_-]+:/ {inside=0} inside {print}' "$WORKFLOW")"
	ci_image="$(printf '%s\n' "$ci_block" | awk '$1 == "image:" {print $2; exit}')"
	if [[ -z "$compose_image" || "$compose_image" != "$ci_image" ]]; then
		printf 'Floci %s image pin differs between Compose and CI\n' "$cloud" >&2
		fail=1
	fi
	if ! grep -Fq -- "-tags=$tag $package" <<<"$ci_block"; then
		printf 'CI Floci job for %s does not test %s with tag %s\n' "$cloud" "$package" "$tag" >&2
		fail=1
	fi
	case "$cloud" in
		gcp)
			plugin_run="go test -tags=floci ./providers/gcp/plugin -run '^TestGCPFlociPreviewSweepUsesSharedBucket$' -count=1"
			if ! grep -Fq -- "$plugin_run" "$script" || ! grep -Fq -- "$plugin_run" <<<"$ci_block"; then
				printf 'GCP Floci plugin sweep is missing from the local runner or CI job\n' >&2
				fail=1
			fi
			;;
		azure)
			plugin_run="go test -race -tags=floci ./providers/azure/plugin -run '^TestAzureFloci' -count=1"
			if ! grep -Fq -- "$plugin_run" "$script" || ! grep -Fq -- "$plugin_run" <<<"$ci_block"; then
				printf 'Azure Floci plugin tests are missing from the local runner or CI job\n' >&2
				fail=1
			fi
			;;
	esac
done

live_cloud='uses:[[:space:]]*(aws-actions/[^@[:space:]]+|azure/login|google-github-actions/auth)@|scripts/(aws|gcp|k8s)-acceptance-local\.sh|(magelift|pulumi)[[:space:]]+(deploy|destroy|up)([[:space:]]|$)|secrets\.(AWS_ACCESS_KEY_ID|AWS_SECRET_ACCESS_KEY|AZURE_CREDENTIALS|GOOGLE_CREDENTIALS)'
if grep -REin --include='*.yml' --include='*.yaml' "$live_cloud" "$ROOT/.github/workflows" "$ROOT/.github/actions"; then
	printf 'MageLift repository workflows must use Floci instead of real cloud credentials or deployments\n' >&2
	fail=1
fi

if [[ "$fail" -ne 0 ]]; then
	printf 'port_coverage_floci_gate_test FAILED\n' >&2
	exit 1
fi

printf 'port_coverage_floci_gate_test OK\n'

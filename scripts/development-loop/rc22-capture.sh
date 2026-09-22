#!/usr/bin/env bash
# Read-only rc.22 diagnostic capture plan. Lists the evidence set from the
# alpha development loop spec and refuses mutating commands. Live capture runs
# only against retained rc.22 resources in the acceptance project.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

MUTATING_PATTERNS=(
	'magelift up'
	'magelift destroy'
	'gcloud .+ create'
	'pulumi up'
	'pulumi destroy'
	'kubectl delete'
)

usage() {
	cat <<'EOF'
rc22-capture.sh lists the required rc.22 evidence set and writes a dry-run
diagnosis stub when MAGELIFT_RC22_CAPTURE=dry-run. It never mutates cloud
resources. Operator actions: pushing and deleting v0.0.0-canary.<sha> are
explicit maintainer steps outside this script.
EOF
}

print_evidence_list() {
	cat <<'EOF'
rc.22 evidence set (collect before destroy):
- deployed identities (commit, CLI checksum, provider digest/version, Magento image/manifest digests)
- deploy Job status and logs
- web-server and PHP-FPM output
- Magento exception.log and system.log
- redacted effective runtime bindings
- database, cache, search, object-storage, and queue reachability
- in-cluster service HTTP request
- corresponding ingress HTTP request
EOF
}

refuse_mutating() {
	local cmd="$1"
	local pattern
	for pattern in "${MUTATING_PATTERNS[@]}"; do
		if printf '%s' "$cmd" | grep -Eq "$pattern"; then
			printf 'rc22-capture: refusing mutating command: %s\n' "$cmd" >&2
			return 1
		fi
	done
	return 0
}

write_dry_run_diagnosis() {
	local out="${MAGELIFT_RC22_DIAGNOSIS_FILE:-.magelift/development-loop/rc22-diagnosis.json}"
	mkdir -p "$(dirname "$out")"
	jq -nc \
		--arg generatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		--arg cause "unresolved" \
		'{version:"v1",cause:$cause,generatedAt:$generatedAt,
		  reason:"live logs were not collected; root cause remains unresolved",
		  evidence:["deploy-job","web-server","php-fpm","magento-logs","runtime-bindings",
		            "dependency-health","in-cluster-request","ingress-request"]}' >"$out"
	printf 'rc22-capture: wrote dry-run diagnosis stub to %s\n' "$out"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--command)
		if ! refuse_mutating "${2:-}"; then
			exit 1
		fi
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'rc22-capture: unknown argument %s\n' "$1" >&2
		exit 2
		;;
	esac
done

print_evidence_list

case "${MAGELIFT_RC22_CAPTURE:-}" in
dry-run)
	write_dry_run_diagnosis
	;;
live)
	printf 'rc22-capture: live capture must target the retained rc.22 acceptance cell only\n' >&2
	exit 2
	;;
esac

exit 0

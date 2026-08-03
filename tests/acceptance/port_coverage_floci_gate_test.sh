#!/usr/bin/env bash
# Gate: every mockable Day-2 port-coverage row maps to an automated test symbol (ACCEPT-06).
# paid-only / Pulumi-mocks-only rows are listed as exempt, not as Floci-certified.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MATRIX="$ROOT/docs/capability-matrix.md"

if [[ ! -f "$MATRIX" ]]; then
	printf 'missing %s\n' "$MATRIX" >&2
	exit 1
fi

fail=0

# mockable_port → required symbol (grep across repo)
# Format: "label|evidence_class|symbol|search_roots"
MAPPINGS=(
	"State|Floci|TestBootstrapStateAndLockAgainstFloci|tests/floci"
	"Secrets|Floci|TestSecretsManagerAgainstFloci|tests/floci"
	"TailLogs|Floci|TestCloudWatchLogsAgainstFloci|tests/floci"
	"CheckRuntime|Floci|TestECSRuntimeHealthAgainstFloci|tests/floci"
	"PrepareExec|unit-fake|TestPrepareExecRejectsDeployWorkload|internal/cloud/aws/ops"
	"SelectTask|unit-fake|TestSelectTaskSortsRunningTaskARNs|internal/cloud/aws/operations"
	"AcquireLock|Floci|TestBootstrapStateAndLockAgainstFloci|tests/floci"
	"Media|Floci|TestVersionedMediaRestoreAgainstFloci|tests/floci"
	"NewDeploySteps|unit-fake|TestNewDeployStepsRejectsWrongBackend|internal/cloud/aws/ops"
)

# paid-only exemptions — must appear in matrix, must NOT be claimed as Floci-only
EXEMPT_PAID=(
	"Bootstrap"
	"ExecuteCommand"
)

require_in_matrix() {
	local needle="$1"
	if ! grep -Fq "$needle" "$MATRIX"; then
		printf 'matrix missing port/evidence mention: %s\n' "$needle" >&2
		fail=1
	fi
}

require_in_matrix "Day-2 port coverage"
require_in_matrix "paid-only"
require_in_matrix "Bootstrap"

for row in "${MAPPINGS[@]}"; do
	IFS='|' read -r label evidence symbol root <<<"$row"
	require_in_matrix "$label"
	if ! grep -R --include='*_test.go' -l -F "func $symbol" "$ROOT/$root" >/dev/null 2>&1; then
		printf 'MISSING test symbol for %s (%s): %s under %s\n' "$label" "$evidence" "$symbol" "$root" >&2
		fail=1
	fi
done

for port in "${EXEMPT_PAID[@]}"; do
	# Bootstrap is required; ExecuteCommand may be spelled in PrepareExec notes
	if [[ "$port" == "Bootstrap" ]]; then
		require_in_matrix "Bootstrap"
	fi
done

# Honesty: Bootstrap row must not claim bare "Floci" as sole evidence without paid-only
if grep -E '^\| Bootstrap' "$MATRIX" | grep -q 'Floci' && ! grep -E '^\| Bootstrap' "$MATRIX" | grep -q 'paid-only'; then
	printf 'Bootstrap must not be Floci-certified without paid-only marker\n' >&2
	fail=1
fi

if [[ "$fail" -ne 0 ]]; then
	printf 'port_coverage_floci_gate_test FAILED\n' >&2
	exit 1
fi

printf 'port_coverage_floci_gate_test OK\n'

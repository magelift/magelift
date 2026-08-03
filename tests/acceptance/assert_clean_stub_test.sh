#!/usr/bin/env bash
# Offline ACCEPT-04: stubbed assert_clean dual outcome (clean→0, leftover→non-zero).
# Requires MAGELIFT_ACCEPTANCE_AWS_STUB=1 and PATH-isolated fake aws — never a real account.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:---clean}"

if [[ "${MAGELIFT_ACCEPTANCE_AWS_STUB:-}" != "1" && "${MAGELIFT_ACCEPTANCE_AWS_STUB:-}" != "true" ]]; then
	printf 'refusing: set MAGELIFT_ACCEPTANCE_AWS_STUB=1 (never export in live sessions)\n' >&2
	exit 2
fi

STUB_DIR=$(mktemp -d)
trap 'rm -rf "$STUB_DIR"' EXIT

LEFTOVER_COUNT=0
case "$MODE" in
--clean) LEFTOVER_COUNT=0 ;;
--leftover) LEFTOVER_COUNT=1 ;;
*)
	printf 'usage: %s --clean|--leftover\n' "$0" >&2
	exit 2
	;;
esac

# PATH-isolated fake aws: never invoke the real binary.
cat >"$STUB_DIR/aws" <<EOF
#!/usr/bin/env bash
set -euo pipefail
# Record invocation for isolation check
printf '%s\n' "\$*" >>"$STUB_DIR/aws-invocations.log"
# All describe/list length queries return LEFTOVER_COUNT (0=clean, 1=leftover).
printf '%s\n' "$LEFTOVER_COUNT"
EOF
chmod +x "$STUB_DIR/aws"

export PATH="$STUB_DIR:$PATH"
# Ensure we are not accidentally using a real aws ahead of stub
if ! command -v aws | grep -q "$STUB_DIR"; then
	printf 'PATH isolation failed: aws resolves to %s\n' "$(command -v aws)" >&2
	exit 1
fi

# shellcheck source=../../scripts/acceptance/lib-assert-clean-aws.sh
source "$ROOT/scripts/acceptance/lib-assert-clean-aws.sh"

project_tag=acceptance
region=eu-north-1

set +e
assert_clean_aws "$project_tag" "$region" >/dev/null 2>"$STUB_DIR/stderr.txt"
rc=$?
set -e

if [[ ! -f "$STUB_DIR/aws-invocations.log" ]]; then
	printf 'stub aws was never called\n' >&2
	exit 1
fi

# Must not have resolved outside stub dir (invocations only through our fake).
real_aws=$(command -v aws)
case "$real_aws" in
"$STUB_DIR/aws") ;;
*)
	printf 'real aws leaked into PATH: %s\n' "$real_aws" >&2
	exit 1
	;;
esac

if [[ "$MODE" == "--clean" ]]; then
	if [[ "$rc" -ne 0 ]]; then
		printf 'expected clean→0, got %s\n' "$rc" >&2
		cat "$STUB_DIR/stderr.txt" >&2
		exit 1
	fi
	if ! grep -q 'assert_clean ok' "$STUB_DIR/stderr.txt"; then
		printf 'expected assert_clean ok on stderr\n' >&2
		cat "$STUB_DIR/stderr.txt" >&2
		exit 1
	fi
	printf 'assert_clean_stub_test --clean OK\n'
	exit 0
fi

# --leftover
if [[ "$rc" -eq 0 ]]; then
	printf 'expected leftover→non-zero, got 0\n' >&2
	cat "$STUB_DIR/stderr.txt" >&2
	exit 1
fi
if ! grep -q 'assert_clean FAILED' "$STUB_DIR/stderr.txt"; then
	printf 'expected FAILED message on stderr\n' >&2
	cat "$STUB_DIR/stderr.txt" >&2
	exit 1
fi
printf 'assert_clean_stub_test --leftover OK (rc=%s)\n' "$rc"
# Exit non-zero so callers using `test $? -ne 0` after this script see leftover failure.
# But the test harness itself succeeded — use exit 1 to match plan verify:
#   ... --leftover; test $? -ne 0
# That expects the leftover run's process exit ≠ 0. So exit with assert_clean's rc.
exit "$rc"

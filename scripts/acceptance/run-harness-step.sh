#!/usr/bin/env bash
# Run one offline acceptance harness step with a progress line and a hard timeout.
# Prefer GNU timeout/gtimeout when present. macOS does not ship timeout, so
# fall back to python3 and a new process group so go build / sleep children die.
set -Eeuo pipefail

usage() {
	printf 'usage: run-harness-step.sh <seconds> <label> -- <command>...\n' >&2
	exit 2
}

if [[ "$#" -lt 4 ]]; then
	usage
fi

seconds="$1"
label="$2"
shift 2
if [[ "${1:-}" != "--" ]]; then
	usage
fi
shift

if [[ ! "$seconds" =~ ^[1-9][0-9]*$ ]]; then
	printf 'harness step timeout must be a positive integer\n' >&2
	exit 2
fi
if [[ -z "$label" ]]; then
	printf 'harness step label is required\n' >&2
	exit 2
fi
if [[ "$#" -lt 1 ]]; then
	usage
fi

printf '+ acceptance-harness-test: %s (timeout %ss)\n' "$label" "$seconds"

timeout_bin=""
if command -v gtimeout >/dev/null 2>&1; then
	timeout_bin="$(command -v gtimeout)"
elif command -v timeout >/dev/null 2>&1 && timeout --version >/dev/null 2>&1; then
	timeout_bin="$(command -v timeout)"
fi

cmd_rc=0
if [[ -n "$timeout_bin" ]]; then
	set +e
	# No "--" separator: GNU timeout treats it as the command (uutils
	# accepts it). Our commands never start with a dash.
	"$timeout_bin" --signal=TERM --kill-after=10 "$seconds" "$@"
	cmd_rc=$?
	set -e
elif command -v python3 >/dev/null 2>&1; then
	set +e
	python3 -c '
import os
import signal
import subprocess
import sys

seconds = int(sys.argv[1])
argv = sys.argv[2:]
proc = subprocess.Popen(argv, start_new_session=True)
try:
    cmd_rc = proc.wait(timeout=seconds)
except subprocess.TimeoutExpired:
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.wait()
    raise SystemExit(124)
raise SystemExit(cmd_rc if cmd_rc >= 0 else 1)
' "$seconds" "$@"
	cmd_rc=$?
	set -e
else
	printf 'acceptance-harness-test: need GNU timeout/gtimeout or python3 to enforce step timeouts\n' >&2
	exit 2
fi

if [[ "$cmd_rc" -eq 124 ]]; then
	printf 'acceptance-harness-test: %s timed out after %ss\n' "$label" "$seconds" >&2
fi
exit "$cmd_rc"

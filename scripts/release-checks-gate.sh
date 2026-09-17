#!/usr/bin/env bash
# Promotion gate: fail unless the required CI checks are green on a commit.
# The release workflow calls this before publishing a draft candidate.
# Missing checks fail: tag a commit CI actually ran on (main HEAD or a PR
# head), and dispatch full CI on the tag when filters would skip a
# required check (gh workflow run ci.yml --ref <tag> -f all=true).
#
# Usage: release-checks-gate.sh --sha <sha> [--repo owner/name]
#        [--checks-json FILE] [check...]
# Default checks: lint go-verify sdk-verify provider-verify floci-gcp php.
# --checks-json evaluates one canned check-runs payload (tests):
# exit 0 pass, 1 blocked, 2 still pending. Without it, the gate polls
# the API until every required check completes or the timeout expires.
set -Eeuo pipefail

REPO="magelift/magelift"
SHA=""
CHECKS_JSON=""
CHECKS=(lint go-verify sdk-verify provider-verify floci-gcp php)
POLL_SECONDS="${GATE_POLL_SECONDS:-60}"
TIMEOUT_SECONDS="${GATE_TIMEOUT_SECONDS:-1800}"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--sha)
		SHA="${2:?--sha needs a value}"
		shift 2
		;;
	--repo)
		REPO="${2:?--repo needs a value}"
		shift 2
		;;
	--checks-json)
		CHECKS_JSON="${2:?--checks-json needs a value}"
		shift 2
		;;
	-h | --help)
		sed -n '2,9p' "$0"
		exit 0
		;;
	--* | "")
		printf 'release-checks-gate: unknown option %s\n' "$1" >&2
		exit 3
		;;
	*)
		break
		;;
	esac
done
if [[ $# -gt 0 ]]; then
	CHECKS=("$@")
fi
if [[ -z "$SHA" ]]; then
	printf 'release-checks-gate: --sha is required\n' >&2
	exit 3
fi

evaluate() {
	export GATE_PAYLOAD="$1"
	export GATE_CHECKS="${CHECKS[*]}"
	python3 - <<'PY'
import json
import os
import sys

try:
    data = json.loads(os.environ["GATE_PAYLOAD"])
except (KeyError, ValueError) as exc:
    print(f"release-checks-gate: invalid check-runs payload: {exc}", file=sys.stderr)
    sys.exit(3)

wanted = os.environ.get("GATE_CHECKS", "").split()
runs = data.get("check_runs", []) if isinstance(data, dict) else []
latest = {}
for run in runs:
    name = run.get("name", "")
    if name not in wanted:
        continue
    # Latest attempt wins: a rerun in progress supersedes an older
    # success. Order by start time, then run id.
    key = (run.get("started_at") or "", run.get("id") or 0)
    if name not in latest or key >= latest[name][0]:
        latest[name] = (key, run)

active = any(run.get("status", "") != "completed" for run in runs)
code = 0
for name in wanted:
    entry = latest.get(name)
    if entry is None:
        if active:
            print(f"PENDING  {name}: no check run yet, CI still active")
            code = 2
        else:
            print(f"BLOCKED  {name}: no check run on this commit (tag a CI-green commit, or dispatch full CI: gh workflow run ci.yml --ref <tag> -f all=true)")
            code = 1
        continue
    run = entry[1]
    status = run.get("status", "")
    conclusion = run.get("conclusion", "")
    if status != "completed":
        print(f"PENDING  {name}: status={status or 'unknown'} (attempt {run.get('id', '?')})")
        code = 2
    elif conclusion == "success":
        print(f"PASS     {name}: success")
    else:
        print(f"BLOCKED  {name}: conclusion={conclusion or 'unknown'}")
        code = 1

sys.exit(code)
PY
}

if [[ -n "$CHECKS_JSON" ]]; then
	evaluate "$(cat "$CHECKS_JSON")"
	exit "$?"
fi

command -v gh >/dev/null 2>&1 || {
	printf 'release-checks-gate: gh is required\n' >&2
	exit 3
}
deadline=$((SECONDS + TIMEOUT_SECONDS))
while true; do
	payload="$(gh api "repos/$REPO/commits/$SHA/check-runs?per_page=100")"
	set +e
	evaluate "$payload"
	code="$?"
	set -e
	if [[ "$code" != "2" ]]; then
		exit "$code"
	fi
	if [[ "$SECONDS" -ge "$deadline" ]]; then
		printf 'release-checks-gate: timed out waiting for checks on %s\n' "$SHA" >&2
		printf 'dispatch full CI on the tag: gh workflow run ci.yml --ref <tag> -f all=true\n' >&2
		exit 1
	fi
	sleep "$POLL_SECONDS"
done

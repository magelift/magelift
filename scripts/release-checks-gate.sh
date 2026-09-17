#!/usr/bin/env bash
# Promotion gate: fail unless the required CI checks are green on a commit.
# The release workflow calls this before publishing a draft candidate.
# Missing checks fail: tag a commit CI actually ran on (main HEAD or a PR
# head), not an unpushed or unchecked tree.
#
# Usage: release-checks-gate.sh --sha <sha> [--repo owner/name]
#        [--checks-json FILE] [check...]
# Default checks: lint go-verify sdk-verify provider-verify floci-gcp php.
# --checks-json replays a canned check-runs payload instead of gh (tests).
set -Eeuo pipefail

REPO="magelift/magelift"
SHA=""
CHECKS_JSON=""
CHECKS=(lint go-verify sdk-verify provider-verify floci-gcp php)

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
		exit 2
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
	exit 2
fi

payload=""
if [[ -n "$CHECKS_JSON" ]]; then
	payload="$(cat "$CHECKS_JSON")"
else
	command -v gh >/dev/null 2>&1 || {
		printf 'release-checks-gate: gh is required\n' >&2
		exit 2
	}
	payload="$(gh api "repos/$REPO/commits/$SHA/check-runs?per_page=100")"
fi

export GATE_PAYLOAD="$payload"
export GATE_CHECKS="${CHECKS[*]}"
python3 - <<'PY'
import json
import os
import sys

try:
    data = json.loads(os.environ["GATE_PAYLOAD"])
except (KeyError, ValueError) as exc:
    print(f"release-checks-gate: invalid check-runs payload: {exc}", file=sys.stderr)
    sys.exit(2)

wanted = os.environ.get("GATE_CHECKS", "").split()
runs = data.get("check_runs", []) if isinstance(data, dict) else []
latest = {}
for run in runs:
    name = run.get("name", "")
    if name not in wanted:
        continue
    stamp = run.get("completed_at") or ""
    if name not in latest or stamp >= latest[name].get("completed_at", ""):
        latest[name] = run

failed = False
for name in wanted:
    run = latest.get(name)
    if run is None:
        print(f"BLOCKED  {name}: no check run on this commit (tag a CI-green commit)")
        failed = True
        continue
    status = run.get("status", "")
    conclusion = run.get("conclusion", "")
    if status != "completed":
        print(f"BLOCKED  {name}: status={status or 'unknown'} (not completed)")
        failed = True
    elif conclusion in ("success", "skipped"):
        extra = " (skipped by path filters)" if conclusion == "skipped" else ""
        print(f"PASS     {name}: {conclusion}{extra}")
    else:
        print(f"BLOCKED  {name}: conclusion={conclusion or 'unknown'}")
        failed = True

sys.exit(1 if failed else 0)
PY

#!/usr/bin/env bash
# Run MageLift Go CI jobs locally via nektos/act (no GitHub Actions minutes).
#
# Serial builds keep local Act/Go smoke light (see agents/skills/magelift-serial-builds).
# Does not run image/php/docs jobs; those stay for real Actions when minutes return.
#
# Prerequisites: brew install act; Colima (or Docker) running.
#
# Usage:
#   ./scripts/ci-act-go.sh              # changes → cache-prime → lint×6 → go-verify
#   ./scripts/ci-act-go.sh lint         # lint matrix only (all partitions, serial)
#   ./scripts/ci-act-go.sh lint aws     # one lint partition
#   ./scripts/ci-act-go.sh go-verify
#   ./scripts/ci-act-go.sh cache-prime
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v act >/dev/null 2>&1; then
  echo "act not found. Install: brew install act" >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon not reachable (Colima/Docker Desktop must be running)." >&2
  exit 1
fi

# Colima: act cannot mount ~/.colima/*/docker.sock into containers; disable DinD socket.
if [[ -S "${HOME}/.colima/default/docker.sock" ]]; then
  export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
fi

EVENT_DIR="${TMPDIR:-/tmp}/magelift-act"
mkdir -p "$EVENT_DIR"
EVENT_FILE="$EVENT_DIR/workflow_dispatch.json"
# Rich payload so dorny/paths-filter accepts workflow_dispatch under act.
cat >"$EVENT_FILE" <<'EOF'
{
  "action": "workflow_dispatch",
  "inputs": {
    "all": "true"
  },
  "ref": "refs/heads/main",
  "repository": {
    "default_branch": "main",
    "name": "magelift",
    "full_name": "magelift/magelift",
    "owner": {
      "login": "magelift"
    }
  },
  "sender": {
    "login": "magelift"
  }
}
EOF

ARCH_ARGS=(--container-architecture linux/arm64)
if [[ "$(uname -m)" != "arm64" ]]; then
  ARCH_ARGS=(--container-architecture linux/amd64)
fi

ACT_BASE=(
  act workflow_dispatch
  -W .github/workflows/ci.yml
  -e "$EVENT_FILE"
  --input all=true
  --concurrent-jobs 1
  --container-daemon-socket -
  "${ARCH_ARGS[@]}"
)

if [[ "${ACT_BIND:-0}" == "1" ]]; then
  ACT_BASE+=(--bind)
fi

run_job() {
  local job="$1"
  shift
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo " act ► job ${job} $*"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  "${ACT_BASE[@]}" -j "$job" "$@" \
    --env GOMAXPROCS=1 \
    --env GOFLAGS=-p=1
}

TARGET="${1:-all}"
PART="${2:-}"

case "$TARGET" in
  all)
    run_job changes
    run_job cache-prime
    for name in aws gcp ovh scaleway core aggregate; do
      run_job lint --matrix "name:${name}"
    done
    run_job go-verify
    echo ""
    echo "act Go pipeline finished (local substitute for hosted CI Go jobs)."
    ;;
  lint)
    if [[ -n "$PART" ]]; then
      run_job lint --matrix "name:${PART}"
    else
      for name in aws gcp ovh scaleway core aggregate; do
        run_job lint --matrix "name:${name}"
      done
    fi
    ;;
  changes|cache-prime|go-verify|workflow-lint)
    run_job "$TARGET"
    ;;
  *)
    echo "Unknown target: $TARGET" >&2
    echo "Usage: $0 [all|changes|cache-prime|lint [partition]|go-verify|workflow-lint]" >&2
    exit 2
    ;;
esac

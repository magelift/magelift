#!/usr/bin/env bash
# Run go test for the CLI, public SDK and provider modules.
# Optional -race via --race. MAGELIFT_GO_TEST_WRAPPER overrides the go command (tests only).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=go-memlimit.sh
source "$ROOT/scripts/go-memlimit.sh"
magelift_apply_go_memlimit

usage() {
  cat <<'EOF'
usage: go-test-modules.sh [--community] [--race] [--only <module>] [--list-modules]

  --community     omit frozen provider/vendor suites from the root module
  --race          pass -race to go test
  --only <name>   run a single module (repeatable)
  --list-modules  print module names and exit 0
EOF
}

race=0
community=0
declare -a only=()
declare -a module_names=(root sdk)
for manifest in "$ROOT"/providers/*/go.mod; do
  [[ -f "$manifest" ]] || continue
  provider="${manifest%/go.mod}"
  module_names+=("${provider##*/}")
done

go_cmd() {
  if [[ -n "${MAGELIFT_GO_TEST_WRAPPER:-}" ]]; then
    # shellcheck disable=SC2086
    "${MAGELIFT_GO_TEST_WRAPPER}" "$@"
  else
    command go "$@"
  fi
}

valid_module() {
  local module
  for module in "${module_names[@]}"; do
    [[ "$module" == "$1" ]] && return 0
  done
  return 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
  --community)
    community=1
    shift
    ;;
  --race)
    race=1
    shift
    ;;
  --only)
    [[ $# -ge 2 ]] || {
      usage >&2
      exit 2
    }
    if ! valid_module "$2"; then
      printf 'go-test-modules: unknown module %s (available: %s)\n' "$2" "${module_names[*]}" >&2
      exit 2
    fi
    only+=("$2")
    shift 2
    ;;
  --list-modules)
    printf '%s\n' "${module_names[@]}"
    exit 0
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    printf 'go-test-modules: unknown argument %s\n' "$1" >&2
    usage >&2
    exit 2
    ;;
  esac
done

module_wanted() {
  local name="$1"
  if [[ "${#only[@]}" -eq 0 ]]; then
    return 0
  fi
  local item
  for item in "${only[@]}"; do
    if [[ "$item" == "$name" ]]; then
      return 0
    fi
  done
  return 1
}

race_args=()
if ((race == 1)); then
  race_args=(-race)
fi

run_root() {
  local package_output
  local -a packages=(./...)
  cd "$ROOT"
  if ((community == 1)); then
    package_output="$(go_cmd list ./...)" || return
    packages=()
    while IFS= read -r package; do
      packages+=("$package")
    done < <(printf '%s\n' "$package_output" | awk '
      !/\/internal\/cloud\/(aws|ovh|scaleway)(\/|$)/ &&
      !/\/internal\/external(\/|$)/ &&
      !/\/cmd\/(aws-|ovh-|scaleway-|newrelic-|failed-deployment-probe)/
    ')
    if [[ "${#packages[@]}" -eq 0 ]]; then
      printf 'go-test-modules: no community root packages found\n' >&2
      return 1
    fi
  fi
  printf '+ go-test-modules: root (%s packages; community=%s)\n' "${#packages[@]}" "$community"
  go_cmd test "${race_args[@]}" "${packages[@]}" -count=1
}

run_sdk() {
  printf '+ go-test-modules: sdk (GOWORK=off) ./...\n'
  (cd "$ROOT/sdk" && GOWORK=off go_cmd test "${race_args[@]}" ./... -count=1)
}

run_provider() {
  local provider="$1"
  printf '+ go-test-modules: providers/%s ./...\n' "$provider"
  (cd "$ROOT/providers/$provider" && go_cmd test "${race_args[@]}" ./... -count=1)
}

status=0
if module_wanted root; then
  if ! run_root; then
    status=1
  fi
fi
if module_wanted sdk; then
  if ! run_sdk; then
    status=1
  fi
fi
for provider in "${module_names[@]:2}"; do
  if module_wanted "$provider" && ! run_provider "$provider"; then
    status=1
  fi
done

exit "$status"

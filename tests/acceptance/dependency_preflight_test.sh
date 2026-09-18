#!/usr/bin/env bash
# Acceptance harnesses must validate local tooling before provider mutation.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"

acceptance_require_jq
acceptance_require_yq_v4

if (
	jq() { return 127; }
	if acceptance_require_jq; then
		exit 1
	fi
) 2>/dev/null; then
	:
else
	printf 'jq preflight did not reject an unusable executable\n' >&2
	exit 1
fi

if (
	yq() { printf 'yq version 3.4.1\n'; }
	if acceptance_require_yq_v4; then
		exit 1
	fi
) 2>/dev/null; then
	:
else
	printf 'yq preflight did not reject a non-Mike-Farah v4 implementation\n' >&2
	exit 1
fi

if (
	yq() {
		if [[ "${1:-}" == --version ]]; then
			printf 'yq (https://github.com/mikefarah/yq/) version v4.45.1\n'
			return 0
		fi
		return 1
	}
	if acceptance_require_yq_v4; then
		exit 1
	fi
) 2>/dev/null; then
	:
else
	printf 'yq preflight did not reject an incompatible v4 executable\n' >&2
	exit 1
fi

if (
	yq() {
		if [[ "${1:-}" == --version ]]; then
			printf 'yq version v4.45.1\n'
			return 0
		fi
		if [[ "${1:-}" == -n ]]; then
			printf 'ok\n'
			return 0
		fi
		if [[ "${1:-}" == -i ]]; then
			[[ "${2:-}" == '.target.marker = strenv(MAGELIFT_ACCEPTANCE_DEPENDENCY_PROBE)' ]] || return 1
			printf 'target:\n  marker: ok\n' >"${3:?probe file required}"
			return 0
		fi
		if [[ "${1:-}" == -r ]]; then
			printf 'ok\n'
			return 0
		fi
		return 1
	}
	if acceptance_require_yq_v4; then
		exit 1
	fi
) 2>/dev/null; then
	:
else
	printf 'yq preflight did not reject a non-Mike-Farah v4 executable\n' >&2
	exit 1
fi

missing_output="$(acceptance_require_command __magelift_missing_dependency__ __magelift_missing_package__ 2>&1 || true)"
if [[ "$missing_output" != *'__magelift_missing_dependency__'* || "$missing_output" != *'before provisioning'* || "$missing_output" != *'install it with:'* ]]; then
	printf 'dependency failure did not provide an actionable install hint: %s\n' "$missing_output" >&2
	exit 1
fi

missing_group_output="$(acceptance_require_commands __magelift_missing_one__ __magelift_missing_two__ 2>&1 || true)"
if [[ "$missing_group_output" != *'__magelift_missing_one__'* || "$missing_group_output" != *'__magelift_missing_two__'* ]]; then
	printf 'group dependency preflight did not report every missing command: %s\n' "$missing_group_output" >&2
	exit 1
fi

missing_json_yaml_output="$(
	jq() { return 127; }
	yq() { return 127; }
	acceptance_require_json_yaml_tools 2>&1 || true
)"
if [[ "$missing_json_yaml_output" != *'acceptance dependency jq'* || "$missing_json_yaml_output" != *'acceptance dependency yq'* ]]; then
	printf 'JSON/YAML preflight did not report both missing tools: %s\n' "$missing_json_yaml_output" >&2
	exit 1
fi

# Enumerate the entrypoints instead of maintaining a second, silently stale
# list. The two provider aliases are deliberately thin exec wrappers; their
# dependency gate is owned by k8s-acceptance-local.sh.
acceptance_wrapper_count=0
while IFS= read -r script; do
	[[ -z "$script" ]] && continue
	acceptance_wrapper_count=$((acceptance_wrapper_count + 1))
	bash -n "$script"
	if grep -Eq '^[[:space:]]*exec .*k8s-acceptance-local\.sh' "$script"; then
		continue
	fi
	grep -Fq 'lib-dependencies.sh' "$script" || {
		printf '%s does not source the shared dependency preflight\n' "${script#"$ROOT/"}" >&2
		exit 1
	}
	grep -Eq 'acceptance_require_(jq|yq_v4|json_yaml_tools)' "$script" || {
		printf '%s has no jq/yq dependency preflight\n' "${script#"$ROOT/"}" >&2
		exit 1
	}

	dependency_line="$(grep -n -m1 -E 'acceptance_require_(jq|yq_v4|json_yaml_tools)' "$script" | cut -d: -f1)"
	first_provider_command_line="$(awk '
		/^[[:space:]]*#/ || /acceptance_require_/ { next }
		/(^|[[:space:];|&()])(aws|gcloud|pulumi|fastly|newrelic|ovhcloud|scw|cf|docker|curl|go)[[:space:]]/ {
			print NR
			exit
		}
	' "$script")"
	if [[ -z "$first_provider_command_line" || "$dependency_line" -ge "$first_provider_command_line" ]]; then
		printf '%s must gate provider commands with jq/yq preflight (dependency line %s, first provider command %s)\n' \
			"${script#"$ROOT/"}" "$dependency_line" "${first_provider_command_line:-missing}" >&2
		exit 1
	fi

	# Dry-run is a mode of an acceptance wrapper, not a dependency bypass. If a
	# wrapper has a dry-run branch, its jq/yq gate must precede that branch too.
	dry_run_line="$(awk '
		/^[[:space:]]*#/ { next }
		/MAGELIFT_ACCEPTANCE_DRY_RUN/ {
			print NR
			exit
		}
	' "$script")"
	if [[ -n "$dry_run_line" && "$dependency_line" -ge "$dry_run_line" ]]; then
		printf '%s must run jq/yq preflight before its dry-run path (dependency line %s, dry-run line %s)\n' \
			"${script#"$ROOT/"}" "$dependency_line" "$dry_run_line" >&2
		exit 1
	fi

	# Provider CLIs are a live-path concern. Dry-run must not require aws,
	# scw, ovhcloud, cf, gcloud, or newrelic just to print the contract.
	command_line="$(grep -n -m1 'acceptance_require_commands' "$script" | cut -d: -f1 || true)"
	if [[ -n "$dry_run_line" && -n "$command_line" && "$command_line" -le "$dry_run_line" ]]; then
		printf '%s must require provider CLIs after its dry-run path (command line %s, dry-run line %s)\n' \
			"${script#"$ROOT/"}" "$command_line" "$dry_run_line" >&2
		exit 1
	fi
done < <(find "$ROOT/scripts" "$ROOT/providers/gcp/scripts" -maxdepth 1 -type f -name '*-acceptance-local.sh' -print | sort)

if (( acceptance_wrapper_count == 0 )); then
	printf 'no top-level acceptance wrappers found\n' >&2
	exit 1
fi

# Keep the guard future-proof: a new top-level acceptance entrypoint must not
# be able to invoke jq/yq before the shared preflight merely because it was
# omitted from the table above. Sourced implementation libraries are excluded;
# their entrypoints own the dependency gate.
while IFS= read -r script; do
	[[ -z "$script" ]] && continue
	uses_jq=0
	uses_yq=0
	if grep -Eq '(^|[^[:alnum:]_])jq([^[:alnum:]_]|$)' "$ROOT/$script"; then
		uses_jq=1
	fi
	if grep -Eq '(^|[^[:alnum:]_])yq([^[:alnum:]_]|$)' "$ROOT/$script"; then
		uses_yq=1
	fi
	if (( uses_jq == 0 && uses_yq == 0 )); then
		continue
	fi
	grep -Fq 'lib-dependencies.sh' "$ROOT/$script" || {
		printf '%s invokes jq/yq without sourcing the shared dependency preflight\n' "$script" >&2
		exit 1
	}
	if (( uses_jq == 1 )); then
		grep -Eq 'acceptance_require_(jq|json_yaml_tools)' "$ROOT/$script" || {
			printf '%s invokes jq without a jq preflight\n' "$script" >&2
			exit 1
		}
	fi
	if (( uses_yq == 1 )); then
		grep -Eq 'acceptance_require_(yq_v4|json_yaml_tools)' "$ROOT/$script" || {
			printf '%s invokes yq without a Mike Farah yq v4 preflight\n' "$script" >&2
			exit 1
		}
	fi
done < <(find "$ROOT/scripts" -maxdepth 1 -type f -name '*.sh' -print | sort | sed "s#^$ROOT/##")

fastly_script="$ROOT/scripts/fastly-acceptance-local.sh"
grep -Fq 'fastly_acceptance_cli service list --non-interactive --quiet --json' "$fastly_script" || {
	printf 'Fastly acceptance must use a non-interactive read-only token preflight\n' >&2
	exit 1
}
if grep -Fq 'fastly whoami -' "$fastly_script"; then
	printf 'Fastly acceptance must not launch browser OAuth through whoami\n' >&2
	exit 1
fi
if grep -Fq 'MAGELIFT_FASTLY_TOKEN_NAME:-default' "$fastly_script"; then
	printf 'Fastly acceptance must not default MAGELIFT_FASTLY_TOKEN_NAME to default\n' >&2
	exit 1
fi

if find "$ROOT/scripts" -type f -name '*.sh' -exec grep -En 'yq[^[:cntrl:]]*--arg|yq[^[:cntrl:]]*--argjson' {} + >/dev/null; then
	printf 'acceptance scripts must use yq v4 expressions/env variables, not jq-style yq arguments\n' >&2
	exit 1
fi

printf 'dependency_preflight_test OK\n'

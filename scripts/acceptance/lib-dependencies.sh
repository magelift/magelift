#!/usr/bin/env bash
# Shared local-tool checks for acceptance harnesses.
#
# Keep these checks before any provider mutation. We intentionally do not
# install tools from a test script: changing a user's machine mid-certification
# is surprising, and package-manager availability differs across platforms.
# Instead, fail with the exact missing tool and an actionable install hint.

acceptance_dependency_install_hint() {
	local command_name="${1:?command name required}"
	local install_name="${2:-$command_name}"
	if command -v brew >/dev/null 2>&1; then
		printf 'brew install %s' "$install_name"
		return 0
	fi
	case "$(uname -s 2>/dev/null || printf unknown)" in
	Linux)
		if command -v apt-get >/dev/null 2>&1; then
			if [[ "$command_name" == yq ]]; then
				printf 'install Mike Farah yq v4 from https://github.com/mikefarah/yq#install (the distro yq package may be a different tool)'
			else
				printf 'sudo apt-get update && sudo apt-get install %s' "$install_name"
			fi
			return 0
		fi
		if command -v dnf >/dev/null 2>&1; then
			printf 'sudo dnf install %s' "$install_name"
			return 0
		fi
		if command -v apk >/dev/null 2>&1; then
			printf 'sudo apk add %s' "$install_name"
			return 0
		fi
		;;
	Darwin)
		printf 'install Homebrew, then run: brew install %s' "$install_name"
		return 0
		;;
	esac
	case "$command_name" in
	jq) printf 'follow the official jq installation documentation: https://github.com/jqlang/jq/wiki/Installation' ;;
	yq) printf 'follow the official Mike Farah yq v4 installation documentation: https://github.com/mikefarah/yq#install' ;;
	*) printf 'follow the official %s installation documentation' "$command_name" ;;
	esac
}

acceptance_require_command() {
	local command_name="${1:?command name required}"
	local install_name="${2:-$command_name}"
	if command -v "$command_name" >/dev/null 2>&1; then
		return 0
	fi
	printf 'acceptance dependency %s is required before provisioning; install it with: %s\n' \
		"$command_name" "$(acceptance_dependency_install_hint "$command_name" "$install_name")" >&2
	return 1
}

acceptance_require_commands() {
	local requirement command_name install_name dependency_status=0
	for requirement in "$@"; do
		command_name="${requirement%%=*}"
		install_name="${requirement#*=}"
		if [[ "$install_name" == "$requirement" ]]; then
			install_name="$command_name"
		fi
		if ! acceptance_require_command "$command_name" "$install_name"; then
			dependency_status=1
		fi
	done
	return "$dependency_status"
}

acceptance_require_jq() {
	if ! acceptance_require_command jq jq; then
		return 1
	fi
	local probe
	if ! probe="$(jq -cn --arg value ok '{value:$value}' 2>/dev/null)" || [[ "$probe" != '{"value":"ok"}' ]]; then
		printf 'acceptance dependency jq is present but cannot evaluate the JSON filter and argument features used by the harness\n' >&2
		return 1
	fi
}

acceptance_require_yq_v4() {
	if ! acceptance_require_command yq yq; then
		return 1
	fi
	local version
	version="$(yq --version 2>&1 || true)"
	case "$version" in
		*'github.com/mikefarah/yq/'*'version v4.'*|*'github.com/mikefarah/yq/'*'version 4.'*) ;;
		*)
			printf 'acceptance dependency yq must be mikefarah/yq v4; found: %s\nInstall or upgrade it with: %s\n' \
				"$version" "$(acceptance_dependency_install_hint yq yq)" >&2
			return 1
			;;
	esac
	local probe
	if ! probe="$(MAGELIFT_ACCEPTANCE_DEPENDENCY_PROBE=ok yq -n -r 'strenv(MAGELIFT_ACCEPTANCE_DEPENDENCY_PROBE)' 2>/dev/null)" || [[ "$probe" != ok ]]; then
		printf 'acceptance dependency yq v4 is incompatible: strenv() expression evaluation failed\n' >&2
		return 1
	fi
	local probe_file probe_value
	if ! probe_file="$(mktemp "${TMPDIR:-/tmp}/magelift-yq-preflight.XXXXXX" 2>/dev/null)"; then
		printf 'acceptance dependency yq v4 is incompatible: could not create a temporary YAML probe file\n' >&2
		return 1
	fi
	if ! printf 'target: {}\n' >"$probe_file" || \
		! MAGELIFT_ACCEPTANCE_DEPENDENCY_PROBE=ok yq -i '.target.marker = strenv(MAGELIFT_ACCEPTANCE_DEPENDENCY_PROBE)' "$probe_file" >/dev/null 2>&1 || \
		! probe_value="$(yq -r '.target.marker // ""' "$probe_file" 2>/dev/null)" || \
		[[ "$probe_value" != ok ]]; then
		rm -f "$probe_file"
		printf 'acceptance dependency yq v4 is incompatible: in-place YAML mutation or file evaluation failed\n' >&2
		return 1
	fi
	rm -f "$probe_file"
}

acceptance_require_json_yaml_tools() {
	local dependency_status=0
	acceptance_require_jq || dependency_status=1
	acceptance_require_yq_v4 || dependency_status=1
	return "$dependency_status"
}

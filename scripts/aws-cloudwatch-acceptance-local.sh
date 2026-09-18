#!/usr/bin/env bash
# Disposable native AWS CloudWatch acceptance. One marker owns one log group,
# dashboard, disabled alarm, and custom metric probe; all resources are
# deleted through their owning APIs and checked after teardown.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_AWS_PROFILE:-default}"
region="${MAGELIFT_AWS_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-}}}"
run_id="${MAGELIFT_AWS_CLOUDWATCH_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
marker="${MAGELIFT_AWS_CLOUDWATCH_MARKER:-magelift/aws/observability/${run_id}}"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'aws CloudWatch acceptance dry-run profile=%s region=%s marker=%s; no AWS mutation invoked\n' "$profile" "${region:-unset}" "$marker"
	exit 0
fi

if [[ -z "$region" ]]; then
	region="$(aws configure get region --profile "$profile" 2>/dev/null || true)"
fi

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^[a-z]{2}(-gov)?-[a-z]+-[0-9]+$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,96}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid AWS profile, region, run ID, or CloudWatch marker\n' >&2
	exit 2
fi

digest="$(printf '%s' "$marker" | shasum -a 256 | awk '{print substr($1,1,16)}')"
log_group="/magelift/observability/$digest"
dashboard="magelift-$digest"
alarm_prefix="magelift-$digest-"
cleanup_enabled=0
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-cloudwatch-${run_id}-ttl-expired-$$"

cloudwatch_cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS CloudWatch acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	if ((cleanup_enabled == 0)); then
		exit "$status"
	fi
	AWS_PROFILE="$profile" AWS_REGION="$region" aws logs delete-log-group --log-group-name "$log_group" >/dev/null 2>&1 || true
	AWS_PROFILE="$profile" AWS_REGION="$region" aws cloudwatch delete-dashboards --dashboard-names "$dashboard" >/dev/null 2>&1 || true
	mapfile -t alarm_names < <(AWS_PROFILE="$profile" AWS_REGION="$region" aws cloudwatch describe-alarms --alarm-name-prefix "$alarm_prefix" --query 'MetricAlarms[].AlarmName' --output text 2>/dev/null | tr '\t' '\n' | sed '/^None$/d;/^$/d' || true)
	if ((${#alarm_names[@]} > 0)); then
		AWS_PROFILE="$profile" AWS_REGION="$region" aws cloudwatch delete-alarms --alarm-names "${alarm_names[@]}" >/dev/null 2>&1 || true
	fi
	for _ in 1 2 3 4 5 6; do
		local remaining=0
		if AWS_PROFILE="$profile" AWS_REGION="$region" aws logs describe-log-groups --log-group-name-prefix "$log_group" --query "logGroups[?logGroupName=='$log_group'] | length(@)" --output text 2>/dev/null | grep -qx '0'; then
			:
		else
			remaining=1
		fi
		if AWS_PROFILE="$profile" AWS_REGION="$region" aws cloudwatch list-dashboards --dashboard-name-prefix "$dashboard" --query "DashboardEntries[?DashboardName=='$dashboard'] | length(@)" --output text 2>/dev/null | grep -qx '0'; then
			:
		else
			remaining=1
		fi
		if AWS_PROFILE="$profile" AWS_REGION="$region" aws cloudwatch describe-alarms --alarm-name-prefix "$alarm_prefix" --query 'MetricAlarms | length(@)' --output text 2>/dev/null | grep -qx '0'; then
			:
		else
			remaining=1
		fi
		if ((remaining == 0)); then
			break
		fi
		sleep 5
	done
	if ((status != 0)); then
		printf 'AWS CloudWatch acceptance failed marker=%s; exact cleanup attempted\n' "$marker" >&2
	fi
	exit "$status"
}
trap cloudwatch_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

dependency_status=0
acceptance_require_commands aws go shasum || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

AWS_PROFILE="$profile" AWS_REGION="$region" aws sts get-caller-identity >/dev/null

acceptance_prepare_lifecycle
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"
cleanup_enabled=1
(cd "$ROOT" && go run ./cmd/aws-cloudwatch-acceptance --profile "$profile" --region "$region" --marker "$marker")
printf 'AWS CloudWatch acceptance resources cleaned through exact owning-service inventory marker=%s\n' "$marker"

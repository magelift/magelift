#!/usr/bin/env bash
# Credit-efficient AWS S3 recovery cell. One generated bucket is created for
# the full core backup/restore/integrity/cleanup sequence and deleted by this
# wrapper on every exit path. No project-wide bucket listing permission is
# required: the generated name is probed exactly and collisions fail closed.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_json_yaml_tools || dependency_status=1
acceptance_require_commands aws go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_AWS_RECOVERY_PROFILE:-default}"
region="${MAGELIFT_AWS_RECOVERY_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-}}}"
if [[ -z "$region" ]]; then
	region="$(aws configure get region --profile "$profile" 2>/dev/null || true)"
fi
run_id="${MAGELIFT_AWS_RECOVERY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
bucket="${MAGELIFT_AWS_RECOVERY_BUCKET:-magelift-recovery-${run_id}}"
marker="${MAGELIFT_AWS_RECOVERY_MARKER:-magelift/aws/recovery/${run_id}}"
fixture="${MAGELIFT_AWS_RECOVERY_FIXTURE:-fixture-known-content-${run_id}}"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^[A-Za-z0-9-]{1,32}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$bucket" =~ ^[a-z0-9][a-z0-9.-]{2,62}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'set valid AWS S3 recovery profile, region, run ID, bucket, marker, and fixture values\n' >&2
	exit 2
fi

export AWS_PROFILE="$profile"
export AWS_REGION="$region"
export AWS_DEFAULT_REGION="$region"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'AWS S3 recovery acceptance dry-run ok profile=%s region=%s bucket=%s marker=%s; no AWS mutation invoked\n' "$profile" "$region" "$bucket" "$marker"
	exit 0
fi

acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-recovery-${run_id}-ttl-expired-$$"

bucket_created=0
cleanup() {
	local exit_status=$?
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS S3 recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if (( bucket_created == 1 )); then
		local remove_output delete_output
		local remove_status=0 delete_status=0
		remove_output="$(aws s3 rm "s3://${bucket}/" --recursive 2>&1)" || remove_status=$?
		delete_output="$(aws s3api delete-bucket --bucket "$bucket" --region "$region" 2>&1)" || delete_status=$?
		local probe_output probe_rc=0
		probe_output="$(aws s3api head-bucket --bucket "$bucket" --region "$region" 2>&1)" || probe_rc=$?
		local not_found_pattern='404|not found|nosuchbucket'
		if (( probe_rc == 0 )) || ! grep -Eqi "$not_found_pattern" <<<"$probe_output" || { (( remove_status != 0 )) && ! grep -Eqi "$not_found_pattern" <<<"$remove_output"; } || { (( delete_status != 0 )) && ! grep -Eqi "$not_found_pattern" <<<"$delete_output"; }; then
			printf 'AWS S3 recovery bucket cleanup failed or is inconclusive bucket=%s\n' "$bucket" >&2
			exit_status=1
		fi
	fi
	if (( exit_status != 0 )); then
		printf 'AWS S3 recovery acceptance failed; exact bucket cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

probe_output=""
probe_rc=0
probe_output="$(aws s3api head-bucket --bucket "$bucket" --region "$region" 2>&1)" || probe_rc=$?
if (( probe_rc == 0 )); then
	printf 'generated AWS S3 recovery bucket already exists; refusing to adopt it: %s\n' "$bucket" >&2
	exit 2
fi
if ! grep -Eqi '404|not found|nosuchbucket' <<<"$probe_output"; then
	printf 'could not prove generated AWS S3 recovery bucket is absent: %s\n' "$bucket" >&2
	exit 2
fi

if [[ "$region" == "us-east-1" ]]; then
	aws s3api create-bucket --bucket "$bucket" --region "$region" >/dev/null
else
	aws s3api create-bucket --bucket "$bucket" --region "$region" --create-bucket-configuration "LocationConstraint=$region" >/dev/null
fi
bucket_created=1
aws s3api put-public-access-block --bucket "$bucket" --region "$region" --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true >/dev/null

(cd "$ROOT" && go run ./cmd/aws-recovery-acceptance \
	--region "$region" \
	--bucket "$bucket" \
	--marker "$marker" \
	--fixture "$fixture")

printf 'AWS S3 recovery acceptance bucket deleted through exact generated ownership=%s\n' "$bucket"

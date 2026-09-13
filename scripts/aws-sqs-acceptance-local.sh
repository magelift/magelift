#!/usr/bin/env bash
# Credit-efficient AWS SQS recovery cell. One short-retention source queue and
# one known message are created; the Go translator owns export/restore queues,
# and every queue is removed by an ownership-scoped trap before exit.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_jq || dependency_status=1
acceptance_require_commands aws go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_AWS_SQS_PROFILE:-default}"
region="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
if [[ -z "$region" ]]; then
	region="$(aws configure get region --profile "$profile" 2>/dev/null || true)"
fi
if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^[A-Za-z0-9-]{1,32}$ ]]; then
	printf 'set a valid AWS profile and region (AWS SQS profile=%s region=%s)\n' "$profile" "$region" >&2
	exit 2
fi

export AWS_PROFILE="$profile"
export AWS_REGION="$region"
export AWS_DEFAULT_REGION="$region"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'AWS SQS acceptance dry-run ok profile=%s region=%s; no AWS mutation invoked\n' "$profile" "$region"
	exit 0
fi

run_id="$(date -u +%Y%m%d%H%M%S)-$$"
suffix="$(printf '%s' "$run_id" | tr '[:upper:]' '[:lower:]')"
source_name="magelift-acceptance-sqs-${suffix}"
marker="magelift-live-sqs-${suffix}"
fixture="fixture-live-sqs-${suffix}"
source_url=""
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-sqs-${run_id}-ttl-expired-$$"

cleanup() {
	local status=$? queue owner data_class
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS SQS acceptance TTL expired; forced ownership-scoped cleanup marker=%s\n' "$marker" >&2
		status=1
	fi
	for prefix in magelift-export- magelift-restore-; do
		while IFS= read -r queue; do
			[[ -z "$queue" || "$queue" == "None" ]] && continue
			owner="$(aws sqs list-queue-tags --queue-url "$queue" --query 'Tags."magelift.io/ownership"' --output text 2>/dev/null || true)"
			data_class="$(aws sqs list-queue-tags --queue-url "$queue" --query 'Tags."magelift.io/data-class"' --output text 2>/dev/null || true)"
			if [[ "$owner" == "$marker" && "$data_class" == queue ]]; then
				aws sqs delete-queue --queue-url "$queue" >/dev/null 2>&1 || true
			fi
		done < <(aws sqs list-queues --queue-name-prefix "$prefix" --query 'QueueUrls[]' --output text 2>/dev/null | tr '\t' '\n')
	done
	if [[ -n "$source_url" ]]; then
		owner="$(aws sqs list-queue-tags --queue-url "$source_url" --query 'Tags."magelift.io/ownership"' --output text 2>/dev/null || true)"
		data_class="$(aws sqs list-queue-tags --queue-url "$source_url" --query 'Tags."magelift.io/data-class"' --output text 2>/dev/null || true)"
		if [[ "$owner" == "$marker" && "$data_class" == queue ]]; then
			aws sqs delete-queue --queue-url "$source_url" >/dev/null 2>&1 || true
		fi
	fi
	if (( status != 0 )); then
		printf 'AWS SQS acceptance failed; ownership-scoped cleanup was attempted marker=%s\n' "$marker" >&2
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

source_url="$(aws sqs create-queue \
	--queue-name "$source_name" \
	--attributes MessageRetentionPeriod=60,VisibilityTimeout=30,SqsManagedSseEnabled=true \
	--tags "magelift.io/ownership=$marker,magelift.io/data-class=queue,magelift.io/fixture=$fixture" \
	--query QueueUrl --output text)"
if [[ -z "$source_url" || "$source_url" == None ]]; then
	printf 'AWS SQS source queue creation returned no URL\n' >&2
	exit 1
fi

aws sqs send-message --queue-url "$source_url" --message-body "magelift-sqs:${fixture}:${marker}" --query MessageId --output text >/dev/null

for attempt in $(seq 1 30); do
	available="$(aws sqs get-queue-attributes --queue-url "$source_url" --attribute-names ApproximateNumberOfMessages --query 'Attributes.ApproximateNumberOfMessages' --output text 2>/dev/null || true)"
	if [[ "$available" =~ ^[1-9][0-9]*$ ]]; then
		break
	fi
	if [[ "$attempt" -eq 30 ]]; then
		printf 'AWS SQS source message did not become visible within the one-minute readiness budget\n' >&2
		exit 1
	fi
	sleep 2
done

go run ./cmd/aws-sqs-acceptance \
	--region "$region" \
	--source-url "$source_url" \
	--marker "$marker" \
	--fixture "$fixture" \
	--retention-days 1

printf 'AWS SQS acceptance resources cleaned by owning-service inventory and shell trap\n'

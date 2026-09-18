#!/usr/bin/env bash
# Disposable ECS collector acceptance. The fixture owns one public Fargate
# service, one temporary Secrets Manager value, one execution role, and one
# security group; the collector lifecycle mutates only that service revision.
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

: "${MAGELIFT_AWS_COLLECTOR_ACCEPTANCE:?set MAGELIFT_AWS_COLLECTOR_ACCEPTANCE=1 for live ECS collector acceptance}"
if [[ "$MAGELIFT_AWS_COLLECTOR_ACCEPTANCE" != "1" ]]; then
	printf 'refusing live ECS collector acceptance without MAGELIFT_AWS_COLLECTOR_ACCEPTANCE=1\n' >&2
	exit 2
fi

profile="${MAGELIFT_AWS_PROFILE:-default}"
region="${MAGELIFT_AWS_REGION:-$(aws configure get region --profile "$profile" 2>/dev/null || true)}"
newrelic_profile="${MAGELIFT_NEWRELIC_PROFILE:-default}"
account_id="${MAGELIFT_NEWRELIC_ACCOUNT_ID:-}"
endpoint="${MAGELIFT_NEWRELIC_OTLP_ENDPOINT:-https://otlp.eu01.nr-data.net}"
nerdgraph_endpoint="${MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT:-https://api.eu.newrelic.com/graphql}"
image_digest="${MAGELIFT_AWS_COLLECTOR_IMAGE_DIGEST:-docker.io/otel/opentelemetry-collector-contrib@sha256:98274b756324abdb2473fa0c898247a246091e861e61d1548f9be483198eecea}"
emitter_image="${MAGELIFT_AWS_COLLECTOR_EMITTER_IMAGE:-docker.io/curlimages/curl@sha256:4026b29997dc7c823b51c164b71e2b51e0fd95cce4601f78202c513d97da2922}"
run_id="${MAGELIFT_AWS_COLLECTOR_RUN_ID:-$(date -u +%Y%m%d-%H%M%S)-$$}"
cluster="${MAGELIFT_AWS_COLLECTOR_CLUSTER:-magelift-collector-$run_id}"
service="${MAGELIFT_AWS_COLLECTOR_SERVICE:-magelift-collector-$run_id}"
marker="${MAGELIFT_AWS_COLLECTOR_MARKER:-magelift/aws/collector/$run_id}"
subnet_id="${MAGELIFT_AWS_COLLECTOR_SUBNET:-}"
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-collector-${run_id}-ttl-expired-$$"

if [[ -z "$account_id" || ! "$account_id" =~ ^[1-9][0-9]*$ || ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^[a-z0-9-]{1,64}$ || ! "$run_id" =~ ^[A-Za-z0-9_-]{1,32}$ || ! "$cluster" =~ ^[A-Za-z0-9_-]{1,64}$ || ! "$service" =~ ^[A-Za-z0-9_-]{1,64}$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'invalid AWS profile, region, run ID, cluster, service, marker, or New Relic account\n' >&2
	exit 2
fi
if [[ ! "$endpoint" =~ ^https://[^[:space:]]+$ || ! "$nerdgraph_endpoint" =~ ^https://[^[:space:]]+$ ]]; then
	printf 'New Relic OTLP and NerdGraph endpoints must be HTTPS URLs\n' >&2
	exit 2
fi
if [[ ! "$image_digest" =~ @sha256:[0-9a-f]{64}$ || ! "$emitter_image" =~ @sha256:[0-9a-f]{64}$ ]]; then
	printf 'collector and emitter images must be immutable repository@sha256 digests\n' >&2
	exit 2
fi

export AWS_PROFILE="$profile" AWS_REGION="$region"
acceptance_prepare_lifecycle

vpc_id=""
security_group_id=""
cluster_created=0
service_created=0
base_task_definition_arn=""
family="magelift-ecs-collector-$run_id"
execution_role_name="magelift-ecs-exec-$run_id"
execution_role_arn=""
execution_role_created=0
secret_name="magelift/aws/collector/$run_id"
secret_arn=""
key_id=""
license_key=""
query_key=""
task_definition_file=""

cleanup() {
	local status=$?
	local cleanup_error=0
	local task_definition
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS ECS collector acceptance TTL expired; cleanup was forced marker=%s\n' "$marker" >&2
		status=1
	fi

	if ((service_created == 1)); then
		if ! aws ecs delete-service --cluster "$cluster" --service "$service" --force >/dev/null 2>&1; then
			printf 'ECS service deletion failed service=%s\n' "$service" >&2
			cleanup_error=1
		fi
		aws ecs wait services-inactive --cluster "$cluster" --services "$service" >/dev/null 2>&1 || true
	fi

	if [[ -n "$family" ]]; then
		while IFS= read -r task_definition; do
			[[ -z "$task_definition" || "$task_definition" == "None" ]] && continue
			aws ecs deregister-task-definition --task-definition "$task_definition" >/dev/null 2>&1 || true
		done < <(aws ecs list-task-definitions --family-prefix "${family}-magelift-" --status ACTIVE --query 'taskDefinitionArns[]' --output text 2>/dev/null | tr '\t' '\n' || true)
	fi
	if [[ -n "$base_task_definition_arn" ]]; then
		aws ecs deregister-task-definition --task-definition "$base_task_definition_arn" >/dev/null 2>&1 || true
	fi
	if ((cluster_created == 1)); then
		if ! aws ecs delete-cluster --cluster "$cluster" >/dev/null 2>&1; then
			printf 'ECS cluster deletion failed cluster=%s\n' "$cluster" >&2
			cleanup_error=1
		fi
	fi
	if [[ -n "$security_group_id" ]]; then
		for _ in 1 2 3 4 5 6; do
			if aws ec2 delete-security-group --group-id "$security_group_id" >/dev/null 2>&1; then
				security_group_id=""
				break
			fi
			sleep 5
		done
		if [[ -n "$security_group_id" ]]; then
			printf 'security group deletion did not converge group=%s\n' "$security_group_id" >&2
			cleanup_error=1
		fi
	fi
	if [[ -n "$secret_arn" ]]; then
		if ! aws secretsmanager delete-secret --secret-id "$secret_arn" --force-delete-without-recovery >/dev/null 2>&1; then
			printf 'temporary Secrets Manager secret deletion failed name=%s\n' "$secret_name" >&2
			cleanup_error=1
		fi
	fi
	if ((execution_role_created == 1)); then
		aws iam delete-role-policy --role-name "$execution_role_name" --policy-name magelift-ecs-collector >/dev/null 2>&1 || true
		for _ in 1 2 3 4 5 6; do
			if aws iam delete-role --role-name "$execution_role_name" >/dev/null 2>&1; then
				execution_role_name=""
				break
			fi
			sleep 5
		done
		if [[ -n "$execution_role_name" ]]; then
			printf 'execution role deletion did not converge role=%s\n' "$execution_role_name" >&2
			cleanup_error=1
		fi
	fi
	if [[ -n "$key_id" ]]; then
		delete_spec="$(jq -cn --arg id "$key_id" '{ingestKeyIds:[$id]}')"
		if ! newrelic --profile "$newrelic_profile" --accountId "$account_id" apiAccess apiAccessDeleteKeys --keys "$delete_spec" --format JSON --plain >/dev/null 2>&1; then
			printf 'New Relic disposable collector ingest-key revocation failed key-id=%s\n' "$key_id" >&2
			cleanup_error=1
		fi
	fi
	if [[ -n "$task_definition_file" && -e "$task_definition_file" ]]; then
		unlink "$task_definition_file"
	fi
	if [[ -e "$ttl_marker" ]]; then
		unlink "$ttl_marker"
	fi
	unset license_key query_key
	if ((cleanup_error != 0)); then
		status=1
	fi
	if ((status != 0)); then
		printf 'AWS ECS collector acceptance failed marker=%s; exact cleanup attempted\n' "$marker" >&2
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" ]]; then
	printf 'aws ECS collector acceptance dry-run ok region=%s cluster=%s service=%s marker=%s\n' "$region" "$cluster" "$service" "$marker"
	exit 0
fi

dependency_status=0
acceptance_require_commands aws newrelic go mktemp || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

aws sts get-caller-identity >/dev/null
if [[ -z "$subnet_id" ]]; then
	subnet_id="$(aws ec2 describe-subnets --filters Name=default-for-az,Values=true --output json | jq -r '.Subnets[] | select(.MapPublicIpOnLaunch == true) | .SubnetId' | head -1)"
fi
vpc_id="$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)"
if [[ -z "$subnet_id" || "$subnet_id" == "None" || -z "$vpc_id" || "$vpc_id" == "None" ]]; then
	printf 'a default public subnet and default VPC are required for the disposable Fargate fixture\n' >&2
	exit 2
fi

query_spec='query { actor { apiAccess { keySearch(query: { types: USER }) { keys { key } } } } }'
query_key="$(newrelic --profile "$newrelic_profile" --accountId "$account_id" nerdgraph query "$query_spec" --format JSON --plain | jq -r '.actor.apiAccess.keySearch.keys[] | select(.key != null and (.key | length > 0)) | .key' | head -1)"
if [[ -z "$query_key" ]]; then
	printf 'New Relic profile did not expose a usable user key for collector verification\n' >&2
	exit 2
fi

key_name="magelift-aws-collector-$run_id"
key_notes="Disposable MageLift ECS collector acceptance marker=$marker"
key_spec="$(jq -cn --argjson accountId "$account_id" --arg name "$key_name" --arg notes "$key_notes" '{ingest:[{accountId:$accountId,ingestType:"LICENSE",name:$name,notes:$notes}]}')"
created_keys="$(newrelic --profile "$newrelic_profile" --accountId "$account_id" apiAccess apiAccessCreateKeys --keys "$key_spec" --format JSON --plain)"
key_id="$(printf '%s' "$created_keys" | jq -r --arg name "$key_name" '.[] | select(.name == $name) | .id' | head -1)"
license_key="$(printf '%s' "$created_keys" | jq -r --arg name "$key_name" '.[] | select(.name == $name) | .key' | head -1)"
if [[ -z "$key_id" || -z "$license_key" ]]; then
	printf 'New Relic disposable collector ingest-key creation returned no usable identity\n' >&2
	exit 1
fi

secret_arn="$(aws secretsmanager create-secret --name "$secret_name" --description "MageLift disposable ECS collector credential" --secret-string "$license_key" --tags "Key=magelift/ownership-marker,Value=$marker" --query ARN --output text)"
if [[ -z "$secret_arn" || "$secret_arn" == "None" ]]; then
	printf 'temporary Secrets Manager secret creation returned no ARN\n' >&2
	exit 1
fi

trust_policy='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
execution_role_arn="$(aws iam create-role --role-name "$execution_role_name" --assume-role-policy-document "$trust_policy" --description "MageLift disposable ECS collector execution role" --tags "Key=magelift/ownership-marker,Value=$marker" --query Role.Arn --output text)"
if [[ -z "$execution_role_arn" || "$execution_role_arn" == "None" ]]; then
	printf 'temporary ECS execution role creation returned no ARN\n' >&2
	exit 1
fi
execution_role_created=1
policy_document="$(jq -cn --arg secret "$secret_arn" '{Version:"2012-10-17",Statement:[{Effect:"Allow",Action:["secretsmanager:DescribeSecret","secretsmanager:GetSecretValue"],Resource:$secret},{Effect:"Allow",Action:["ecr:GetAuthorizationToken"],Resource:"*"},{Effect:"Allow",Action:["ecr-public:GetAuthorizationToken"],Resource:"*"},{Effect:"Allow",Action:["sts:GetServiceBearerToken"],Resource:"*"}]}')"
aws iam put-role-policy --role-name "$execution_role_name" --policy-name magelift-ecs-collector --policy-document "$policy_document" >/dev/null
sleep 8

security_group_name="magelift-ecs-collector-$run_id"
security_group_id="$(aws ec2 create-security-group --group-name "$security_group_name" --description "MageLift disposable ECS collector acceptance" --vpc-id "$vpc_id" --query GroupId --output text)"
aws ec2 create-tags --resources "$security_group_id" --tags "Key=magelift/ownership-marker,Value=$marker" >/dev/null

aws ecs create-cluster --cluster-name "$cluster" --tags "key=magelift/ownership-marker,value=$marker" >/dev/null
cluster_created=1

emitter_command='set -eu
marker="$MAGELIFT_PROBE_MARKER"
digest_hex="$(printf "%s" "$marker" | sha256sum | cut -c1-64)"
trace_id="$(printf "%s" "$digest_hex" | cut -c1-32)"
span_id="$(printf "%s" "$digest_hex" | cut -c1-16)"
send() {
  path="$1"
  body="$2"
  /usr/bin/curl -fsS --max-time 5 -o /dev/null -H "Content-Type: application/json" --data-raw "$body" "http://127.0.0.1:4318/v1/$path" || true
}
while :; do
  now_seconds="$(date +%s)"
  start="${now_seconds}000000000"
  end="$((now_seconds + 1))000000000"
  send logs "{\"resourceLogs\":[{\"resource\":{\"attributes\":[{\"key\":\"magelift.ownership_marker\",\"value\":{\"stringValue\":\"$marker\"}}]},\"scopeLogs\":[{\"logRecords\":[{\"timeUnixNano\":\"$start\",\"body\":{\"stringValue\":\"magelift ecs collector logs\"}}]}]}]}"
  send metrics "{\"resourceMetrics\":[{\"resource\":{\"attributes\":[{\"key\":\"magelift.ownership_marker\",\"value\":{\"stringValue\":\"$marker\"}}]},\"scopeMetrics\":[{\"metrics\":[{\"name\":\"magelift.ecs.collector\",\"gauge\":{\"dataPoints\":[{\"timeUnixNano\":\"$start\",\"asDouble\":1}]}}]}]}]}"
  send traces "{\"resourceSpans\":[{\"resource\":{\"attributes\":[{\"key\":\"magelift.ownership_marker\",\"value\":{\"stringValue\":\"$marker\"}}]},\"scopeSpans\":[{\"spans\":[{\"traceId\":\"$trace_id\",\"spanId\":\"$span_id\",\"name\":\"magelift.ecs.collector\",\"startTimeUnixNano\":\"$start\",\"endTimeUnixNano\":\"$end\"}]}]}]}"
  sleep 10
done'

task_definition_file="$(mktemp "${TMPDIR:-/tmp}/magelift-aws-collector-task.XXXXXX")"
jq -n \
	--arg family "$family" \
	--arg executionRole "$execution_role_arn" \
	--arg emitterImage "$emitter_image" \
	--arg emitterCommand "$emitter_command" \
	--arg marker "$marker" \
	--argjson tags '[{"key":"magelift/ownership-marker","value":"'"$marker"'"}]' \
	'{family:$family,networkMode:"awsvpc",requiresCompatibilities:["FARGATE"],cpu:"512",memory:"1024",executionRoleArn:$executionRole,containerDefinitions:[{name:"emitter",image:$emitterImage,essential:true,entryPoint:["/bin/sh"],command:["-c",$emitterCommand],environment:[{name:"MAGELIFT_PROBE_MARKER",value:$marker}]}],tags:$tags}' >"$task_definition_file"
base_task_definition_arn="$(aws ecs register-task-definition --cli-input-json "file://$task_definition_file" --query taskDefinition.taskDefinitionArn --output text)"
if [[ -z "$base_task_definition_arn" || "$base_task_definition_arn" == "None" ]]; then
	printf 'base ECS task-definition registration returned no ARN\n' >&2
	exit 1
fi
aws ecs create-service --cluster "$cluster" --service-name "$service" --task-definition "$base_task_definition_arn" --desired-count 1 --launch-type FARGATE --network-configuration "awsvpcConfiguration={subnets=[$subnet_id],securityGroups=[$security_group_id],assignPublicIp=ENABLED}" --tags "key=magelift/ownership-marker,value=$marker" >/dev/null
service_created=1
aws ecs wait services-stable --cluster "$cluster" --services "$service"

AWS_PROFILE="$profile" AWS_REGION="$region" \
MAGELIFT_AWS_COLLECTOR_QUERY_KEY="$query_key" \
MAGELIFT_AWS_COLLECTOR_CREDENTIAL_REF="aws-secrets-manager://$secret_name" \
MAGELIFT_AWS_COLLECTOR_SECRET_ARN="$secret_arn" \
go run ./cmd/aws-collector-acceptance \
	--region "$region" \
	--cluster "$cluster" \
	--service "$service" \
	--credential-ref "aws-secrets-manager://$secret_name" \
	--secret-arn "$secret_arn" \
	--endpoint "$endpoint" \
	--nerdgraph-endpoint "$nerdgraph_endpoint" \
	--account-id "$account_id" \
	--image-digest "$image_digest" \
	--marker "$marker"
aws ecs wait services-stable --cluster "$cluster" --services "$service"

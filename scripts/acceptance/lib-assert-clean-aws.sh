#!/usr/bin/env bash
# Shared AWS leftover assertions for acceptance EXIT cleanup (ACCEPT-04).
# Query semantics match the former inline assert_clean in aws-acceptance-local.sh.
# shellcheck shell=bash

# assert_clean_aws PROJECT_TAG REGION [OWNERSHIP_MARKER]

# A live acceptance configuration can reference disposable Secrets Manager
# values that must exist before Pulumi starts (for example, an encryption key
# or broker password). Those values are not stack-owned resources, so the
# normal name scan would otherwise reject a valid run. The caller may provide
# exact, newline- or comma-separated ARNs in
# MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS. Every allowed ARN still
# has to carry the exact project and acceptance-run tags below. Nothing is
# allowed by name alone.
acceptance_secret_arn_is_predeclared() {
	local candidate="${1:?secret ARN required}" allowed
	while IFS= read -r allowed; do
		allowed="${allowed#"${allowed%%[![:space:]]*}"}"
		allowed="${allowed%"${allowed##*[![:space:]]}"}"
		[[ -n "$allowed" && "$candidate" == "$allowed" ]] && return 0
	done < <(printf '%s\n' "${MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS:-}" | tr ',' '\n')
	return 1
}

acceptance_secret_has_run_tags() {
	local arn="${1:?secret ARN required}"
	local region="${2:?region required}"
	local project_tag="${3:?project tag required}"
	local ownership_marker="${4:?ownership marker required}"
	local tags
	if ! tags=$(aws secretsmanager describe-secret --region "$region" --secret-id "$arn" --query 'Tags' --output json); then
		return 1
	fi
	jq -e --arg project "$project_tag" --arg owner "$ownership_marker" '
		(any(.[]?; .Key == "magelift:project" and .Value == $project)) and
		(any(.[]?; .Key == "magelift:managed-by" and .Value == "magelift")) and
		(any(.[]?; .Key == "magelift:purpose" and .Value == "acceptance")) and
		(any(.[]?; .Key == "magelift:acceptance-run" and .Value == $owner))
	' <<<"$tags" >/dev/null
}

acceptance_state_bucket_has_run_tags() {
	local bucket="${1:?state bucket required}"
	local region="${2:?region required}"
	local project_tag="${3:?project tag required}"
	local ownership_marker="${4:?ownership marker required}"
	local tags head_bucket_error

	# A new acceptance run has no state bucket yet. Distinguish that expected
	# absence from a foreign bucket or an authorization failure: only AWS's
	# explicit not-found responses may proceed to the create path. A 403 or
	# redirect must remain a hard ownership failure.
	if ! head_bucket_error=$(aws s3api head-bucket --bucket "$bucket" --region "$region" 2>&1); then
		if grep -Eq '(\(404\)|NoSuchBucket|Not Found|does not exist)' <<<"$head_bucket_error"; then
			return 2
		fi
		return 1
	fi
	if ! tags=$(aws s3api get-bucket-tagging --bucket "$bucket" --region "$region" --output json); then
		return 1
	fi
	jq -e --arg project "$project_tag" --arg owner "$ownership_marker" '
		(any(.TagSet[]?; .Key == "magelift:project" and .Value == $project)) and
		(any(.TagSet[]?; .Key == "magelift:managed-by" and .Value == "magelift")) and
		(any(.TagSet[]?; .Key == "magelift:purpose" and .Value == "acceptance-state")) and
		(any(.TagSet[]?; .Key == "magelift:acceptance-run" and .Value == $owner))
	' <<<"$tags" >/dev/null
}

assert_clean_aws() {
	local project_tag="${1:?project tag required}"
	local region="${2:?region required}"
	local ownership_marker="${3:-}"
	printf '+ assert_clean tag=magelift:project=%s region=%s\n' "$project_tag" "$region" >&2
	local failed=0
	local count tag_region state_bucket_name="${4:-}" state_bucket_verified=0 s3_buckets_json

	if [[ ! "$project_tag" =~ ^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$ ]]; then
		printf 'refusing unsafe AWS project tag for cleanup assertions: %s\n' "$project_tag" >&2
		return 1
	fi
	for tag_region in "$region" us-east-1; do
		local -a tag_filters=(--tag-filters "Key=magelift:project,Values=$project_tag")
		if [[ -n "$ownership_marker" ]]; then
			tag_filters+=("Key=magelift:acceptance-run,Values=$ownership_marker")
		fi
		if ! count=$(aws resourcegroupstaggingapi get-resources --region "$tag_region" "${tag_filters[@]}" --query 'length(ResourceTagMappingList)' --output text); then
			printf 'unable to query tagged AWS resources in %s\n' "$tag_region" >&2
			failed=1
		elif [[ "$count" != "0" ]]; then
			# Resource Groups Tagging API is eventually consistent and keeps
			# tombstones for deleted ECS/EC2 resources. Direct service inventories
			# below are the authoritative cost-bearing cleanup checks.
			printf 'tagging index still reports %s resource mappings in %s; checking live inventories\n' "$count" "$tag_region" >&2
		fi
	done

	count=$(aws ec2 describe-vpcs --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" --query 'length(Vpcs)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover VPCs: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-instances --region "$region" --query "length(DBInstances[?contains(DBInstanceIdentifier, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS instances: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-clusters --region "$region" --query "length(DBClusters[?contains(DBClusterIdentifier, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS clusters: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-subnet-groups --region "$region" --query "length(DBSubnetGroups[?contains(DBSubnetGroupName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS subnet groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-parameter-groups --region "$region" --query "length(DBParameterGroups[?contains(DBParameterGroupName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS parameter groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-snapshots --region "$region" --query "length(DBSnapshots[?contains(DBSnapshotIdentifier, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS snapshots: %s\n' "$count" >&2; failed=1; }

	count=$(aws elasticache describe-replication-groups --region "$region" --query "length(ReplicationGroups[?contains(ReplicationGroupId, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ElastiCache groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws elasticache describe-cache-subnet-groups --region "$region" --query "length((CacheSubnetGroups || \`[]\`)[?contains(CacheSubnetGroupName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ElastiCache subnet groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws elbv2 describe-load-balancers --region "$region" --query "length(LoadBalancers[?contains(LoadBalancerName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ALBs: %s\n' "$count" >&2; failed=1; }

	count=$(aws elbv2 describe-target-groups --region "$region" --query "length(TargetGroups[?contains(TargetGroupName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover target groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws ecs list-clusters --region "$region" --query "length(clusterArns[?contains(@, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ECS clusters: %s\n' "$count" >&2; failed=1; }

	count=$(aws iam list-roles --query "length(Roles[?contains(RoleName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover IAM roles: %s\n' "$count" >&2; failed=1; }

	if [[ "${MAGELIFT_ACCEPTANCE_EKS:-0}" == "1" ]]; then
		count=$(aws eks list-clusters --region "$region" --query "length(clusters[?contains(@, '$project_tag')])" --output text)
		[[ "$count" == "0" ]] || { printf 'leftover EKS clusters: %s\n' "$count" >&2; failed=1; }
	fi

	count=$(aws logs describe-log-groups --region "$region" --query "length(logGroups[?contains(logGroupName, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover log groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws ec2 describe-security-groups --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" --query 'length(SecurityGroups)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover security groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws ec2 describe-instances --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'length(Reservations[].Instances[])' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover live EC2 instances: %s\n' "$count" >&2; failed=1; }

	count=$(aws ec2 describe-addresses --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" --query 'length(Addresses)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover Elastic IPs: %s\n' "$count" >&2; failed=1; }

	count=$(aws ec2 describe-nat-gateways --region "$region" --filter "Name=tag:magelift:project,Values=$project_tag" --query 'length(NatGateways[?State != `deleted`])' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover NAT gateways: %s\n' "$count" >&2; failed=1; }

	count=$(aws ecs list-task-definitions --region "$region" --family-prefix "$project_tag" --query 'length(taskDefinitionArns)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ECS task definitions: %s\n' "$count" >&2; failed=1; }

	if [[ -n "$state_bucket_name" ]]; then
		local state_bucket_status=0
		acceptance_state_bucket_has_run_tags "$state_bucket_name" "$region" "$project_tag" "$ownership_marker" || state_bucket_status=$?
		if [[ "$state_bucket_status" == 0 ]]; then
			state_bucket_verified=1
			printf 'allowing exact owned AWS Pulumi state bucket %s during resume preflight\n' "$state_bucket_name" >&2
		elif [[ "$state_bucket_status" == 2 ]]; then
			printf 'configured AWS Pulumi state bucket is absent; treating this as a new acceptance run: %s\n' "$state_bucket_name" >&2
		else
			printf 'configured AWS Pulumi state bucket is not owned by this acceptance run: %s\n' "$state_bucket_name" >&2
			failed=1
		fi
	fi
	if [[ "$state_bucket_verified" == 1 ]]; then
		if ! s3_buckets_json=$(aws s3api list-buckets --output json); then
			printf 'unable to query S3 buckets for acceptance cleanup assertions\n' >&2
			failed=1
		elif ! count=$(jq --arg project "$project_tag" --arg state_bucket "$state_bucket_name" '[.Buckets[]? | select((.Name | contains($project)) and .Name != $state_bucket)] | length' <<<"$s3_buckets_json"); then
			printf 'unable to parse S3 bucket inventory for acceptance cleanup assertions\n' >&2
			failed=1
		fi
	else
		count=$(aws s3api list-buckets --query "length(Buckets[?contains(Name, '$project_tag')])" --output text)
	fi
	[[ "$count" == "0" ]] || { printf 'leftover S3 buckets containing project tag: %s\n' "$count" >&2; failed=1; }

	# CloudFront is global and its distribution/OAC APIs do not participate in
	# the regional resource-tagging inventory. Match the stable project-bearing
	# comment/origin/OAC name so an interrupted media deployment cannot evade
	# the final cost cleanup gate.
	if ! count=$(aws cloudfront list-distributions --query "length((DistributionList.Items || \`[]\`)[?contains(Comment, '$project_tag') || length((Origins.Items || \`[]\`)[?contains(DomainName, '$project_tag')]) > \`0\`])" --output text); then
		printf 'unable to query CloudFront distributions\n' >&2
		failed=1
	else
		[[ "$count" == "0" ]] || { printf 'leftover CloudFront distributions: %s\n' "$count" >&2; failed=1; }
	fi
	if ! count=$(aws cloudfront list-origin-access-controls --query "length((OriginAccessControlList.Items || \`[]\`)[?contains(Name, '$project_tag')])" --output text); then
		printf 'unable to query CloudFront origin access controls\n' >&2
		failed=1
	else
		[[ "$count" == "0" ]] || { printf 'leftover CloudFront origin access controls: %s\n' "$count" >&2; failed=1; }
	fi

	# Secrets Manager keeps planned-deletion tombstones in list-secrets until
	# the recovery window expires. They are not readable or billable, so the
	# live-resource assertion must distinguish them from an undeleted secret.
	count=$(aws secretsmanager list-secrets --region "$region" --query "length(SecretList[?contains(Name, '$project_tag') && DeletedDate == null])" --output text)
	if [[ "$count" != "0" ]]; then
		if ! secret_rows=$(aws secretsmanager list-secrets --region "$region" --query "SecretList[?contains(Name, '$project_tag') && DeletedDate == null].[ARN,Name]" --output json); then
			printf 'unable to query acceptance Secrets Manager resources\n' >&2
			failed=1
		else
			if ! secret_lines=$(jq -r '.[] | @tsv' <<<"$secret_rows"); then
				printf 'unable to parse acceptance Secrets Manager inventory\n' >&2
				failed=1
			else
				secret_rejected=0
				while IFS=$'\t' read -r secret_arn secret_name; do
					[[ -z "$secret_arn" || "$secret_arn" == "null" ]] && continue
					if [[ -n "$ownership_marker" ]] && acceptance_secret_arn_is_predeclared "$secret_arn" && acceptance_secret_has_run_tags "$secret_arn" "$region" "$project_tag" "$ownership_marker"; then
						printf 'allowing exact predeclared acceptance secret %s\n' "$secret_name" >&2
						continue
					fi
					printf 'leftover Secrets Manager secret: %s\n' "$secret_arn" >&2
					secret_rejected=1
					done <<<"$secret_lines"
				if [[ "$secret_rejected" != 0 ]]; then
					failed=1
				fi
			fi
		fi
	fi

	count=$(aws opensearchserverless list-collections --region "$region" --query "length(collectionSummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless collections: %s\n' "$count" >&2; failed=1; }

	count=$(aws opensearchserverless list-collection-groups --region "$region" --query "length(collectionGroupSummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless collection groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws opensearchserverless list-vpc-endpoints --region "$region" --query "length(vpcEndpointSummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless VPC endpoints: %s\n' "$count" >&2; failed=1; }

	count=$(aws opensearchserverless list-access-policies --region "$region" --type data --query "length(accessPolicySummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless access policies: %s\n' "$count" >&2; failed=1; }

	count=$(aws opensearchserverless list-security-policies --region "$region" --type network --query "length(securityPolicySummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless network policies: %s\n' "$count" >&2; failed=1; }

	count=$(aws opensearchserverless list-security-policies --region "$region" --type encryption --query "length(securityPolicySummaries[?contains(name, '$project_tag')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover OpenSearch Serverless encryption policies: %s\n' "$count" >&2; failed=1; }

	if [[ "$failed" != 0 ]]; then
		printf 'assert_clean FAILED: tagged or acceptance-named leftovers remain\n' >&2
		return 1
	fi
	printf 'assert_clean ok\n' >&2
	return 0
}

# Back-compat name used by aws-acceptance-local.sh (expects project_tag + region in scope).
assert_clean() {
	assert_clean_aws "$project_tag" "$region" "${MAGELIFT_ACCEPTANCE_OWNER:-}" "${state_bucket:-}"
}

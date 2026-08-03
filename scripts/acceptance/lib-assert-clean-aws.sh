#!/usr/bin/env bash
# Shared AWS leftover assertions for acceptance EXIT cleanup (ACCEPT-04).
# Query semantics match the former inline assert_clean in aws-acceptance-local.sh.
# shellcheck shell=bash

# assert_clean_aws PROJECT_TAG REGION
assert_clean_aws() {
	local project_tag="${1:?project tag required}"
	local region="${2:?region required}"
	printf '+ assert_clean tag=magelift:project=%s region=%s\n' "$project_tag" "$region" >&2
	local failed=0
	local count

	count=$(aws ec2 describe-vpcs --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" --query 'length(Vpcs)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover VPCs: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-instances --region "$region" --query "length(DBInstances[?contains(DBInstanceIdentifier, 'acceptance')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS instances: %s\n' "$count" >&2; failed=1; }

	count=$(aws rds describe-db-clusters --region "$region" --query "length(DBClusters[?contains(DBClusterIdentifier, 'acceptance')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover RDS clusters: %s\n' "$count" >&2; failed=1; }

	count=$(aws elasticache describe-replication-groups --region "$region" --query "length(ReplicationGroups[?contains(ReplicationGroupId, 'acceptance')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ElastiCache groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws elbv2 describe-load-balancers --region "$region" --query "length(LoadBalancers[?contains(LoadBalancerName, 'acceptance')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ALBs: %s\n' "$count" >&2; failed=1; }

	count=$(aws ecs list-clusters --region "$region" --query "length(clusterArns[?contains(@, 'acceptance')])" --output text)
	[[ "$count" == "0" ]] || { printf 'leftover ECS clusters: %s\n' "$count" >&2; failed=1; }

	count=$(aws logs describe-log-groups --region "$region" --log-group-name-prefix "/magelift/acceptance" --query 'length(logGroups)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover log groups: %s\n' "$count" >&2; failed=1; }

	count=$(aws ec2 describe-security-groups --region "$region" --filters "Name=tag:magelift:project,Values=$project_tag" --query 'length(SecurityGroups)' --output text)
	[[ "$count" == "0" ]] || { printf 'leftover security groups: %s\n' "$count" >&2; failed=1; }

	if [[ "$failed" != 0 ]]; then
		printf 'assert_clean FAILED: tagged or acceptance-named leftovers remain\n' >&2
		return 1
	fi
	printf 'assert_clean ok\n' >&2
	return 0
}

# Back-compat name used by aws-acceptance-local.sh (expects project_tag + region in scope).
assert_clean() {
	assert_clean_aws "$project_tag" "$region"
}

#!/usr/bin/env bash
# Local, credit-efficient AWS acceptance: preview by default, destroy on EXIT.
# Requires real AWS credentials (aws-cli / SDK default chain) and Pulumi via MageLift.
set -Eeuo pipefail

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to an acceptance configuration file}"
: "${MAGELIFT_AWS_ACCEPTANCE_DIGEST:?set MAGELIFT_AWS_ACCEPTANCE_DIGEST to a signed immutable image reference}"
: "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:?set MAGELIFT_AWS_CERTIFICATE_IDENTITY to the expected Sigstore identity}"
: "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:=https://token.actions.githubusercontent.com}"

profile="${MAGELIFT_AWS_ACCEPTANCE_PROFILE:-preview}"
case "$profile" in
preview|standard|high-availability) ;;
*)
	printf 'unsupported acceptance profile: %s (use preview unless credits allow more)\n' "$profile" >&2
	exit 2
;;
esac

if [[ "$profile" != preview && "${MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY:-}" != true ]]; then
	printf 'refusing %s without MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true\n' "$profile" >&2
	exit 2
fi

project_tag="${MAGELIFT_AWS_ACCEPTANCE_PROJECT_TAG:-acceptance}"
region="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
if [[ -z "$region" ]]; then
	printf 'set AWS_REGION (or AWS_DEFAULT_REGION) for leftover assertions\n' >&2
	exit 2
fi

config=("$MAGELIFT_BIN" --config "$MAGELIFT_CONFIG" --env "$profile" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

assert_clean() {
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

created=0
cleanup() {
	local status=$?
	if [[ "$created" == 1 && "${MAGELIFT_AWS_ACCEPTANCE_KEEP:-false}" != true ]]; then
		printf '+ magelift destroy --yes (EXIT trap)\n' >&2
		"${config[@]}" destroy --yes || printf 'acceptance cleanup failed for %s; inspect with aws-cli and destroy manually\n' "$profile" >&2
		assert_clean || status=1
	fi
	exit "$status"
}
trap cleanup EXIT

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf 'aws acceptance start profile=%s at=%s\n' "$profile" "$started_at" >&2

run config validate
run doctor
run login
run preview
run promote \
	--digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" \
	--certificate-identity "$MAGELIFT_AWS_CERTIFICATE_IDENTITY" \
	--certificate-oidc-issuer "$MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER"
created=1
run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes
run outputs
run health --mode runtime

printf 'aws acceptance ok profile=%s; destroy + assert_clean run on EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true\n' "$profile" >&2

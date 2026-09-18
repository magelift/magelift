#!/usr/bin/env bash
# Cost-bounded AWS RDS/Aurora recovery acceptance. The wrapper owns one
# generated source resource and one generated subnet group. The Go cell owns
# the manual snapshot and isolated restore; this trap is the last-resort
# cleanup for every marker-owned RDS output and then removes the source and
# subnet group.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

dependency_status=0
acceptance_require_json_yaml_tools || dependency_status=1
acceptance_require_commands aws curl go || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

profile="${MAGELIFT_AWS_RDS_RECOVERY_PROFILE:-default}"
region="${MAGELIFT_AWS_RDS_RECOVERY_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-}}}"
if [[ -z "$region" ]]; then
	region="$(aws configure get region --profile "$profile" 2>/dev/null || true)"
fi
run_id="${MAGELIFT_AWS_RDS_RECOVERY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
source_kind="${MAGELIFT_AWS_RDS_RECOVERY_SOURCE_KIND:-instance}"
source_instance="${MAGELIFT_AWS_RDS_RECOVERY_INSTANCE:-magelift-rds-${run_id}}"
if [[ "$source_kind" == "cluster" ]]; then
	source_instance="${MAGELIFT_AWS_RDS_RECOVERY_CLUSTER:-magelift-aurora-${run_id}}"
fi
source_member="${source_instance}-instance"
subnet_group="${MAGELIFT_AWS_RDS_RECOVERY_SUBNET_GROUP:-magelift-rds-subnet-${run_id}}"
security_group="${MAGELIFT_AWS_RDS_RECOVERY_SECURITY_GROUP:-magelift-rds-sg-${run_id}}"
source_class="${MAGELIFT_AWS_RDS_RECOVERY_SOURCE_CLASS:-db.t3.micro}"
restore_class="${MAGELIFT_AWS_RDS_RECOVERY_RESTORE_CLASS:-db.t3.micro}"
engine_version="${MAGELIFT_AWS_RDS_RECOVERY_ENGINE_VERSION:-8.0.46}"
if [[ "$source_kind" == "cluster" ]]; then
	source_class="${MAGELIFT_AWS_RDS_RECOVERY_SOURCE_CLASS:-db.t3.medium}"
	restore_class="${MAGELIFT_AWS_RDS_RECOVERY_RESTORE_CLASS:-db.t3.medium}"
	engine_version="${MAGELIFT_AWS_RDS_RECOVERY_ENGINE_VERSION:-8.0.mysql_aurora.3.08.2}"
fi
multi_az="${MAGELIFT_AWS_RDS_RECOVERY_MULTI_AZ:-0}"
marker="${MAGELIFT_AWS_RDS_RECOVERY_MARKER:-magelift/aws/rds-recovery/${run_id}}"
fixture="${MAGELIFT_AWS_RDS_RECOVERY_FIXTURE:-fixture-rds-control-plane-${run_id}}"
explicit_subnet_ids="${MAGELIFT_AWS_RDS_RECOVERY_SUBNET_IDS:-}"
# Aurora caps the master password at 41 characters. Keep enough run identity
# for disposable isolation while staying inside both RDS and Aurora limits.
master_password="MageLift-${run_id:0:20}-Rds9"

if [[ ! "$profile" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$region" =~ ^[A-Za-z0-9-]{1,32}$ || ! "$run_id" =~ ^[A-Za-z0-9._-]{1,48}$ || ! "$source_kind" =~ ^(instance|cluster)$ || ! "$source_instance" =~ ^[a-z][a-z0-9-]{0,62}$ || ! "$source_member" =~ ^[a-z][a-z0-9-]{0,62}$ || ! "$subnet_group" =~ ^[a-z][a-z0-9-]{1,254}$ || ! "$security_group" =~ ^[A-Za-z0-9._-]{1,255}$ || ! "$source_class" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$restore_class" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$engine_version" =~ ^[A-Za-z0-9._-]{1,64}$ || ! "$multi_az" =~ ^(0|1|true|false)$ || ! "$marker" =~ ^[A-Za-z0-9._/-]{1,128}$ || ! "$fixture" =~ ^[A-Za-z0-9._/-]{1,128}$ ]]; then
	printf 'set valid AWS RDS/Aurora recovery profile, source kind, region, run ID, generated names, classes, engine version, marker, and fixture values\n' >&2
	exit 2
fi
if [[ -n "$explicit_subnet_ids" && ! "$explicit_subnet_ids" =~ ^subnet-[A-Za-z0-9]+([[:space:],]+subnet-[A-Za-z0-9]+)*$ ]]; then
	printf 'MAGELIFT_AWS_RDS_RECOVERY_SUBNET_IDS must contain one or more subnet IDs separated by spaces or commas\n' >&2
	exit 2
fi

export AWS_PROFILE="$profile"
export AWS_REGION="$region"
export AWS_DEFAULT_REGION="$region"

if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == true ]]; then
	printf 'AWS database recovery acceptance dry-run ok profile=%s region=%s sourceKind=%s source=%s subnetGroup=%s sourceClass=%s restoreClass=%s engineVersion=%s multiAZ=%s marker=%s; no AWS mutation invoked\n' \
		"$profile" "$region" "$source_kind" "$source_instance" "$subnet_group" "$source_class" "$restore_class" "$engine_version" "$multi_az" "$marker"
	exit 0
fi

export MAGELIFT_ACCEPTANCE_TTL_SECONDS="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-7200}"
acceptance_prepare_lifecycle
ttl_marker="${TMPDIR:-/tmp}/magelift-aws-rds-recovery-${run_id}-ttl-expired-$$"

source_claimed=0
subnet_group_claimed=0
security_group_claimed=0
security_group_id=""

resource_owned() {
	local tags_json="$1"
	if jq -e --arg marker "$marker" '
		if ((.TagList // null) | type) != "array" then error("TagList is not an array") else any(.TagList[]; .Key == "magelift.io/ownership" and .Value == $marker) end
	' <<<"$tags_json" >/dev/null 2>&1; then
		return 0
	fi
	if jq -e '((.TagList // null) | type) == "array"' <<<"$tags_json" >/dev/null 2>&1; then
		return 1
	fi
	return 2
}

not_found_error() {
	grep -Eqi 'notfound|not found|does not exist|404|nosuch' <<<"$1"
}

owned_instance_count() {
	local instances_json id arn tags_json count=0 preserve_instance="$source_instance"
	if [[ "$source_kind" == "cluster" ]]; then
		preserve_instance="$source_member"
	fi
	if ! instances_json="$(aws rds describe-db-instances --region "$region" --output json 2>&1)"; then
		printf 'RDS instance inventory failed: %s\n' "$instances_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS instance ownership inventory failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if resource_owned "$tags_json"; then
			[[ "$id" == "$preserve_instance" ]] || count=$((count + 1))
		else
			case "$?" in
			2) printf 'RDS instance ownership inventory was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
	done <<<"$(jq -r '.DBInstances[]? | [(.DBInstanceIdentifier // ""), (.DBInstanceArn // "")] | @tsv' <<<"$instances_json")"
	printf '%s\n' "$count"
}

owned_snapshot_count() {
	local snapshots_json id arn tags_json count=0
	if ! snapshots_json="$(aws rds describe-db-snapshots --region "$region" --snapshot-type manual --output json 2>&1)"; then
		printf 'RDS DB snapshot inventory failed: %s\n' "$snapshots_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS DB snapshot ownership inventory failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if resource_owned "$tags_json"; then
			count=$((count + 1))
		else
			case "$?" in
			2) printf 'RDS DB snapshot ownership inventory was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
	done <<<"$(jq -r '.DBSnapshots[]? | [(.DBSnapshotIdentifier // ""), (.DBSnapshotArn // "")] | @tsv' <<<"$snapshots_json")"
	if ! snapshots_json="$(aws rds describe-db-cluster-snapshots --region "$region" --snapshot-type manual --output json 2>&1)"; then
		printf 'RDS cluster snapshot inventory failed: %s\n' "$snapshots_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS cluster snapshot ownership inventory failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if resource_owned "$tags_json"; then
			count=$((count + 1))
		else
			case "$?" in
			2) printf 'RDS cluster snapshot ownership inventory was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
	done <<<"$(jq -r '.DBClusterSnapshots[]? | [(.DBClusterSnapshotIdentifier // ""), (.DBClusterSnapshotArn // "")] | @tsv' <<<"$snapshots_json")"
	printf '%s\n' "$count"
}

delete_owned_instances() {
	local instances_json id arn tags_json output preserve_instance="$source_instance"
	if [[ "$source_kind" == "cluster" ]]; then
		preserve_instance="$source_member"
	fi
	if ! instances_json="$(aws rds describe-db-instances --region "$region" --output json 2>&1)"; then
		printf 'RDS instance cleanup inventory failed: %s\n' "$instances_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" || "$id" == "$preserve_instance" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS instance cleanup ownership check failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if ! resource_owned "$tags_json"; then
			case "$?" in
			1) continue ;;
			*) printf 'RDS instance cleanup ownership data was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
		aws rds modify-db-instance --region "$region" --db-instance-identifier "$id" --no-deletion-protection --apply-immediately >/dev/null 2>&1 || true
		if ! output="$(aws rds delete-db-instance --region "$region" --db-instance-identifier "$id" --skip-final-snapshot --delete-automated-backups 2>&1)"; then
			if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress' <<<"$output"; then
				printf 'owned RDS restore deletion failed for %s: %s\n' "$id" "$output" >&2
				return 1
			fi
		fi
	done <<<"$(jq -r '.DBInstances[]? | [(.DBInstanceIdentifier // ""), (.DBInstanceArn // "")] | @tsv' <<<"$instances_json")"
}

owned_cluster_count() {
	local clusters_json id arn tags_json count=0
	if ! clusters_json="$(aws rds describe-db-clusters --region "$region" --output json 2>&1)"; then
		printf 'RDS cluster inventory failed: %s\n' "$clusters_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" || "$id" == "$source_instance" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS cluster ownership inventory failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if resource_owned "$tags_json"; then
			count=$((count + 1))
		else
			case "$?" in
			2) printf 'RDS cluster ownership inventory was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
	done <<<"$(jq -r '.DBClusters[]? | [(.DBClusterIdentifier // ""), (.DBClusterArn // "")] | @tsv' <<<"$clusters_json")"
	printf '%s\n' "$count"
}

delete_owned_clusters() {
	local clusters_json id arn tags_json output
	if ! clusters_json="$(aws rds describe-db-clusters --region "$region" --output json 2>&1)"; then
		printf 'RDS cluster cleanup inventory failed: %s\n' "$clusters_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" || "$id" == "$source_instance" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS cluster cleanup ownership check failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if ! resource_owned "$tags_json"; then
			case "$?" in
			1) continue ;;
			*) printf 'RDS cluster cleanup ownership data was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
		aws rds modify-db-cluster --region "$region" --db-cluster-identifier "$id" --no-deletion-protection --apply-immediately >/dev/null 2>&1 || true
		if ! output="$(aws rds delete-db-cluster --region "$region" --db-cluster-identifier "$id" --skip-final-snapshot 2>&1)"; then
			if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress|member' <<<"$output"; then
				printf 'owned RDS cluster deletion failed for %s: %s\n' "$id" "$output" >&2
				return 1
			fi
		fi
	done <<<"$(jq -r '.DBClusters[]? | [(.DBClusterIdentifier // ""), (.DBClusterArn // "")] | @tsv' <<<"$clusters_json")"
}

delete_owned_snapshots() {
	local snapshots_json id arn tags_json output
	if ! snapshots_json="$(aws rds describe-db-snapshots --region "$region" --snapshot-type manual --output json 2>&1)"; then
		printf 'RDS DB snapshot cleanup inventory failed: %s\n' "$snapshots_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS DB snapshot cleanup ownership check failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if ! resource_owned "$tags_json"; then
			case "$?" in
			1) continue ;;
			*) printf 'RDS DB snapshot cleanup ownership data was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
		if ! output="$(aws rds delete-db-snapshot --region "$region" --db-snapshot-identifier "$id" 2>&1)"; then
			if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress' <<<"$output"; then
				printf 'owned RDS DB snapshot deletion failed for %s: %s\n' "$id" "$output" >&2
				return 1
			fi
		fi
	done <<<"$(jq -r '.DBSnapshots[]? | [(.DBSnapshotIdentifier // ""), (.DBSnapshotArn // "")] | @tsv' <<<"$snapshots_json")"

	if ! snapshots_json="$(aws rds describe-db-cluster-snapshots --region "$region" --snapshot-type manual --output json 2>&1)"; then
		printf 'RDS cluster snapshot cleanup inventory failed: %s\n' "$snapshots_json" >&2
		return 1
	fi
	while IFS=$'\t' read -r id arn; do
		[[ -z "$id" ]] && continue
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'RDS cluster snapshot cleanup ownership check failed for %s: %s\n' "$id" "$tags_json" >&2
			return 1
		fi
		if ! resource_owned "$tags_json"; then
			case "$?" in
			1) continue ;;
			*) printf 'RDS cluster snapshot cleanup ownership data was malformed for %s\n' "$id" >&2; return 1 ;;
			esac
		fi
		if ! output="$(aws rds delete-db-cluster-snapshot --region "$region" --db-cluster-snapshot-identifier "$id" 2>&1)"; then
			if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress' <<<"$output"; then
				printf 'owned RDS cluster snapshot deletion failed for %s: %s\n' "$id" "$output" >&2
				return 1
			fi
		fi
	done <<<"$(jq -r '.DBClusterSnapshots[]? | [(.DBClusterSnapshotIdentifier // ""), (.DBClusterSnapshotArn // "")] | @tsv' <<<"$snapshots_json")"
}

cleanup_recovery_outputs() {
	local attempt instance_count cluster_count snapshot_count
	for attempt in $(seq 1 90); do
		delete_owned_instances || return 1
		delete_owned_clusters || return 1
		delete_owned_snapshots || return 1
		if ! instance_count="$(owned_instance_count)" || ! cluster_count="$(owned_cluster_count)" || ! snapshot_count="$(owned_snapshot_count)"; then
			return 1
		fi
		if [[ "$instance_count" == 0 && "$cluster_count" == 0 && "$snapshot_count" == 0 ]]; then
			return 0
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 10
		fi
	done
	printf 'owned RDS recovery outputs did not reach zero before cleanup timeout instances=%s clusters=%s snapshots=%s\n' "$instance_count" "$cluster_count" "$snapshot_count" >&2
	return 1
}

delete_source_instance() {
	if [[ "$source_kind" == "cluster" ]]; then
		delete_source_cluster
		return $?
	fi
	local attempt source_json arn tags_json output status
	for attempt in $(seq 1 90); do
		source_json="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_instance" --output json 2>&1)"
		status=$?
		if (( status != 0 )); then
			if not_found_error "$source_json"; then
				return 0
			fi
			printf 'could not inspect generated RDS source during cleanup: %s\n' "$source_json" >&2
			return 1
		fi
		arn="$(jq -r '.DBInstances[0].DBInstanceArn // empty' <<<"$source_json")"
		if [[ -z "$arn" ]]; then
			printf 'generated RDS source inventory returned no ARN\n' >&2
			return 1
		fi
		if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>&1)"; then
			printf 'could not inspect generated RDS source ownership: %s\n' "$tags_json" >&2
			return 1
		fi
		if ! resource_owned "$tags_json"; then
			printf 'refusing to delete RDS source because its ownership marker is not exact: %s\n' "$source_instance" >&2
			return 1
		fi
		aws rds modify-db-instance --region "$region" --db-instance-identifier "$source_instance" --no-deletion-protection --apply-immediately >/dev/null 2>&1 || true
		if ! output="$(aws rds delete-db-instance --region "$region" --db-instance-identifier "$source_instance" --skip-final-snapshot --delete-automated-backups 2>&1)"; then
			if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress' <<<"$output"; then
				printf 'generated RDS source deletion failed: %s\n' "$output" >&2
				return 1
			fi
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 10
		fi
	done
	printf 'generated RDS source did not reach terminal absence before cleanup timeout: %s\n' "$source_instance" >&2
	return 1
}

delete_source_cluster() {
	local attempt member_json member_status member_state member_arn member_tags cluster_json cluster_status cluster_state cluster_arn cluster_tags output
	for attempt in $(seq 1 90); do
		member_json="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_member" --output json 2>&1)"
		member_status=$?
		if (( member_status == 0 )); then
			member_state="$(jq -r '.DBInstances[0].DBInstanceStatus // empty' <<<"$member_json")"
			member_arn="$(jq -r '.DBInstances[0].DBInstanceArn // empty' <<<"$member_json")"
			if [[ -z "$member_arn" ]]; then
				printf 'generated Aurora source inventory returned no member ARN\n' >&2
				return 1
			fi
			if [[ "$member_state" != "deleting" ]]; then
				if ! member_tags="$(aws rds list-tags-for-resource --region "$region" --resource-name "$member_arn" --output json 2>&1)"; then
					printf 'could not inspect generated Aurora source member ownership: %s\n' "$member_tags" >&2
					return 1
				fi
				if ! resource_owned "$member_tags"; then
					printf 'refusing to delete Aurora source member because its ownership marker is not exact: %s\n' "$source_member" >&2
					return 1
				fi
				aws rds modify-db-instance --region "$region" --db-instance-identifier "$source_member" --no-deletion-protection --apply-immediately >/dev/null 2>&1 || true
				if ! output="$(aws rds delete-db-instance --region "$region" --db-instance-identifier "$source_member" --skip-final-snapshot --delete-automated-backups 2>&1)"; then
					if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress' <<<"$output"; then
						printf 'generated Aurora source member deletion failed: %s\n' "$output" >&2
						return 1
					fi
				fi
			fi
		elif ! not_found_error "$member_json"; then
			printf 'could not inspect generated Aurora source member during cleanup: %s\n' "$member_json" >&2
			return 1
		fi

		cluster_json="$(aws rds describe-db-clusters --region "$region" --db-cluster-identifier "$source_instance" --output json 2>&1)"
		cluster_status=$?
		if (( cluster_status != 0 )); then
			if not_found_error "$cluster_json" && ! aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_member" --output json >/dev/null 2>&1; then
				return 0
			fi
			if ! not_found_error "$cluster_json"; then
				printf 'could not inspect generated Aurora source during cleanup: %s\n' "$cluster_json" >&2
				return 1
			fi
		else
			cluster_state="$(jq -r '.DBClusters[0].Status // empty' <<<"$cluster_json")"
			cluster_arn="$(jq -r '.DBClusters[0].DBClusterArn // empty' <<<"$cluster_json")"
			if [[ -z "$cluster_arn" ]]; then
				printf 'generated Aurora source inventory returned no cluster ARN\n' >&2
				return 1
			fi
			if [[ "$cluster_state" != "deleting" ]]; then
				if ! cluster_tags="$(aws rds list-tags-for-resource --region "$region" --resource-name "$cluster_arn" --output json 2>&1)"; then
					printf 'could not inspect generated Aurora source ownership: %s\n' "$cluster_tags" >&2
					return 1
				fi
				if ! resource_owned "$cluster_tags"; then
					printf 'refusing to delete Aurora source because its ownership marker is not exact: %s\n' "$source_instance" >&2
					return 1
				fi
				aws rds modify-db-cluster --region "$region" --db-cluster-identifier "$source_instance" --no-deletion-protection --apply-immediately >/dev/null 2>&1 || true
				if ! output="$(aws rds delete-db-cluster --region "$region" --db-cluster-identifier "$source_instance" --skip-final-snapshot 2>&1)"; then
					if ! not_found_error "$output" && ! grep -Eqi 'invalid.*state|deleting|in progress|member' <<<"$output"; then
						printf 'generated Aurora source deletion failed: %s\n' "$output" >&2
						return 1
					fi
				fi
			fi
		fi
		if [[ "$attempt" -lt 90 ]]; then
			sleep 10
		fi
	done
	printf 'generated Aurora source did not reach terminal absence before cleanup: %s\n' "$source_instance" >&2
	return 1
}

delete_subnet_group() {
	local output status tags_json subnet_arn
	output="$(aws rds describe-db-subnet-groups --region "$region" --db-subnet-group-name "$subnet_group" --output json 2>&1)"
	status=$?
	if (( status != 0 )); then
		not_found_error "$output" && return 0
		printf 'could not inspect generated RDS subnet group during cleanup: %s\n' "$output" >&2
		return 1
	fi
	subnet_arn="$(jq -r '.DBSubnetGroups[0].DBSubnetGroupArn // empty' <<<"$output")"
	if [[ -z "$subnet_arn" ]]; then
		printf 'generated RDS subnet group inventory returned no ARN\n' >&2
		return 1
	fi
	if ! tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$subnet_arn" --output json 2>&1)"; then
		printf 'could not inspect generated RDS subnet group ownership: %s\n' "$tags_json" >&2
		return 1
	fi
	if ! resource_owned "$tags_json"; then
		printf 'refusing to delete RDS subnet group because its ownership marker is not exact: %s\n' "$subnet_group" >&2
		return 1
	fi
	if ! output="$(aws rds delete-db-subnet-group --region "$region" --db-subnet-group-name "$subnet_group" 2>&1)"; then
		if ! not_found_error "$output"; then
			printf 'generated RDS subnet group deletion failed: %s\n' "$output" >&2
			return 1
		fi
	fi
	for attempt in $(seq 1 30); do
		output="$(aws rds describe-db-subnet-groups --region "$region" --db-subnet-group-name "$subnet_group" --output json 2>&1)"
		status=$?
		if (( status != 0 )) && not_found_error "$output"; then
			return 0
		fi
		if [[ "$attempt" -lt 30 ]]; then
			sleep 5
		fi
	done
	printf 'generated RDS subnet group remains after cleanup: %s\n' "$subnet_group" >&2
	return 1
}

security_group_owned() {
	local tags_json="$1"
	jq -e --arg marker "$marker" '((.SecurityGroups[0].Tags // null) | type) == "array" and any(.SecurityGroups[0].Tags[]; .Key == "magelift.io/ownership" and .Value == $marker)' <<<"$tags_json" >/dev/null 2>&1
}

delete_security_group() {
	local attempt output status tags_json
	[[ -z "$security_group_id" ]] && return 0
	for attempt in $(seq 1 30); do
		output="$(aws ec2 describe-security-groups --region "$region" --group-ids "$security_group_id" --output json 2>&1)"
		status=$?
		if (( status != 0 )); then
			if not_found_error "$output"; then
				return 0
			fi
			printf 'could not inspect generated RDS security group during cleanup: %s\n' "$output" >&2
			return 1
		fi
		tags_json="$output"
		if ! security_group_owned "$tags_json"; then
			printf 'refusing to delete RDS security group because its ownership marker is not exact: %s\n' "$security_group_id" >&2
			return 1
		fi
		if output="$(aws ec2 delete-security-group --region "$region" --group-id "$security_group_id" 2>&1)"; then
			return 0
		fi
		if ! grep -Eqi 'dependency|in use|does not exist|not found|404' <<<"$output"; then
			printf 'generated RDS security group deletion failed: %s\n' "$output" >&2
			return 1
		fi
		if [[ "$attempt" -lt 30 ]]; then
			sleep 10
		fi
	done
	printf 'generated RDS security group remained after cleanup: %s\n' "$security_group_id" >&2
	return 1
}

cleanup() {
	local exit_status=$? cleanup_status=0
	set +e
	acceptance_stop_ttl_watchdog || true
	if acceptance_ttl_expired; then
		printf 'AWS RDS recovery acceptance TTL expired; forced exact cleanup marker=%s\n' "$marker" >&2
		exit_status=1
	fi
	if (( source_claimed == 1 )); then
		cleanup_recovery_outputs || cleanup_status=1
		delete_source_instance || cleanup_status=1
	fi
	if (( subnet_group_claimed == 1 )); then
		delete_subnet_group || cleanup_status=1
	fi
	if (( security_group_claimed == 1 )); then
		delete_security_group || cleanup_status=1
	fi
	if (( cleanup_status != 0 )); then
		printf 'AWS RDS recovery acceptance cleanup was inconclusive; inspect exact marker=%s before retrying\n' "$marker" >&2
		exit_status=1
	fi
	exit "$exit_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"

if [[ -n "$explicit_subnet_ids" ]]; then
	read -r -a subnet_array <<<"$(printf '%s' "$explicit_subnet_ids" | tr ',' ' ')"
else
	vpc_json="$(aws ec2 describe-vpcs --region "$region" --filters Name=isDefault,Values=true --output json 2>&1)" || {
		printf 'could not inspect the default AWS VPC; set MAGELIFT_AWS_RDS_RECOVERY_SUBNET_IDS explicitly: %s\n' "$vpc_json" >&2
		exit 2
	}
	vpc_id="$(jq -r '(.Vpcs // []) | map(select(.IsDefault == true)) | if length == 1 then .[0].VpcId else empty end' <<<"$vpc_json")"
	if [[ -z "$vpc_id" ]]; then
		printf 'AWS account must have exactly one default VPC or MAGELIFT_AWS_RDS_RECOVERY_SUBNET_IDS must be set\n' >&2
		exit 2
	fi
	subnet_json="$(aws ec2 describe-subnets --region "$region" --filters "Name=vpc-id,Values=$vpc_id" Name=state,Values=available --output json 2>&1)" || {
		printf 'could not inspect default VPC subnets: %s\n' "$subnet_json" >&2
		exit 2
	}
	read -r -a subnet_array <<<"$(jq -r '(.Subnets // []) | sort_by(.AvailabilityZone, .SubnetId) | group_by(.AvailabilityZone) | map(.[0]) | .[0:3] | .[].SubnetId' <<<"$subnet_json" | tr '\n' ' ')"
fi
if (( ${#subnet_array[@]} < 2 )); then
	printf 'RDS recovery needs at least two available subnets in distinct AZs; found %s\n' "${#subnet_array[@]}" >&2
	exit 2
fi
subnet_details="$(aws ec2 describe-subnets --region "$region" --subnet-ids "${subnet_array[@]}" --output json 2>&1)" || {
	printf 'could not validate selected RDS subnets: %s\n' "$subnet_details" >&2
	exit 2
}
if ! jq -e '([.Subnets[]?.AvailabilityZone] | unique | length) >= 2' <<<"$subnet_details" >/dev/null; then
	printf 'selected RDS subnets do not span at least two availability zones\n' >&2
	exit 2
fi
vpc_id="$(jq -r '([.Subnets[]?.VpcId] | unique | if length == 1 then .[0] else empty end)' <<<"$subnet_details")"
if [[ -z "$vpc_id" || ! "$vpc_id" =~ ^vpc-[A-Za-z0-9]+$ ]]; then
	printf 'selected RDS subnets must belong to exactly one VPC\n' >&2
	exit 2
fi

caller_ipv4="$(curl -4 -fsS --max-time 15 https://api.ipify.org 2>/dev/null || true)"
if [[ ! "$caller_ipv4" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
	printf 'could not determine a usable runner public IPv4 address for the temporary RDS security group\n' >&2
	exit 2
fi

source_probe_status=0
if [[ "$source_kind" == "cluster" ]]; then
	source_probe="$(aws rds describe-db-clusters --region "$region" --db-cluster-identifier "$source_instance" --output json 2>&1)" || source_probe_status=$?
else
	source_probe="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_instance" --output json 2>&1)" || source_probe_status=$?
fi
if (( source_probe_status == 0 )); then
	printf 'generated AWS database source already exists; refusing to adopt it: %s\n' "$source_instance" >&2
	exit 2
fi
if ! not_found_error "$source_probe"; then
	printf 'could not prove generated AWS database source is absent: %s\n' "$source_probe" >&2
	exit 2
fi
if [[ "$source_kind" == "cluster" ]]; then
	member_probe_status=0
	member_probe="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_member" --output json 2>&1)" || member_probe_status=$?
	if (( member_probe_status == 0 )); then
		printf 'generated AWS Aurora source member already exists; refusing to adopt it: %s\n' "$source_member" >&2
		exit 2
	fi
	if ! not_found_error "$member_probe"; then
		printf 'could not prove generated AWS Aurora source member is absent: %s\n' "$member_probe" >&2
		exit 2
	fi
fi

subnet_probe_status=0
subnet_probe="$(aws rds describe-db-subnet-groups --region "$region" --db-subnet-group-name "$subnet_group" --output json 2>&1)" || subnet_probe_status=$?
if (( subnet_probe_status == 0 )); then
	printf 'generated AWS RDS subnet group already exists; refusing to adopt it: %s\n' "$subnet_group" >&2
	exit 2
fi
if ! not_found_error "$subnet_probe"; then
	printf 'could not prove generated AWS RDS subnet group is absent: %s\n' "$subnet_probe" >&2
	exit 2
fi

security_group_probe="$(aws ec2 describe-security-groups --region "$region" --filters "Name=vpc-id,Values=$vpc_id" "Name=group-name,Values=$security_group" --output json 2>&1)" || {
	printf 'could not inspect generated AWS RDS security group name: %s\n' "$security_group_probe" >&2
	exit 2
}
if jq -e '(.SecurityGroups // []) | length > 0' <<<"$security_group_probe" >/dev/null 2>&1; then
	printf 'generated AWS RDS security group name already exists; refusing to adopt it: %s\n' "$security_group" >&2
	exit 2
fi

security_group_id="$(aws ec2 create-security-group \
	--region "$region" \
	--group-name "$security_group" \
	--description "MageLift disposable RDS recovery ${run_id}" \
	--vpc-id "$vpc_id" \
	--query GroupId \
	--output text 2>&1)" || {
	printf 'could not create generated AWS RDS security group: %s\n' "$security_group_id" >&2
	exit 1
}
if [[ ! "$security_group_id" =~ ^sg-[A-Za-z0-9]+$ ]]; then
	printf 'AWS RDS security group creation returned an invalid ID: %s\n' "$security_group_id" >&2
	exit 1
fi
security_group_claimed=1
aws ec2 create-tags --region "$region" --resources "$security_group_id" --tags \
	"Key=magelift.io/ownership,Value=$marker" \
	"Key=magelift.io/data-class,Value=database" \
	"Key=magelift.io/fixture,Value=$fixture" >/dev/null
aws ec2 authorize-security-group-ingress --region "$region" --group-id "$security_group_id" --protocol tcp --port 3306 --cidr "${caller_ipv4}/32" >/dev/null

subnet_group_claimed=1
aws rds create-db-subnet-group \
	--region "$region" \
	--db-subnet-group-name "$subnet_group" \
	--db-subnet-group-description "MageLift disposable RDS recovery ${run_id}" \
	--subnet-ids "${subnet_array[@]}" \
	--tags "Key=magelift.io/ownership,Value=$marker" "Key=magelift.io/data-class,Value=database" "Key=magelift.io/fixture,Value=$fixture" \
	--output json >/dev/null

source_claimed=1

if [[ "$source_kind" == "cluster" ]]; then
	aws rds create-db-cluster \
		--region "$region" \
		--db-cluster-identifier "$source_instance" \
		--engine aurora-mysql \
		--engine-version "$engine_version" \
		--database-name magelift_recovery \
		--backup-retention-period 1 \
		--db-subnet-group-name "$subnet_group" \
		--master-username magelift \
		--master-user-password "$master_password" \
		--storage-encrypted \
		--vpc-security-group-ids "$security_group_id" \
		--deletion-protection \
		--copy-tags-to-snapshot \
		--tags "Key=magelift.io/ownership,Value=$marker" "Key=magelift.io/data-class,Value=database" "Key=magelift.io/fixture,Value=$fixture" \
		--output json >/dev/null
	aws rds create-db-instance \
		--region "$region" \
		--db-instance-identifier "$source_member" \
		--db-instance-class "$source_class" \
		--engine aurora-mysql \
		--db-cluster-identifier "$source_instance" \
		--db-subnet-group-name "$subnet_group" \
		--publicly-accessible \
		--tags "Key=magelift.io/ownership,Value=$marker" "Key=magelift.io/data-class,Value=database" "Key=magelift.io/fixture,Value=$fixture" \
		--output json >/dev/null
else
	create_args=(
		--region "$region"
		--db-instance-identifier "$source_instance"
		--engine mysql
		--engine-version "$engine_version"
		--db-instance-class "$source_class"
		--allocated-storage 20
		--storage-type gp3
		--storage-encrypted
		--backup-retention-period 1
		--db-subnet-group-name "$subnet_group"
		--master-username magelift
		--master-user-password "$master_password"
		--publicly-accessible
		--vpc-security-group-ids "$security_group_id"
		--deletion-protection
		--copy-tags-to-snapshot
		--tags "Key=magelift.io/ownership,Value=$marker" "Key=magelift.io/data-class,Value=database" "Key=magelift.io/fixture,Value=$fixture"
		--output json
	)
	if [[ "$multi_az" == 1 || "$multi_az" == true ]]; then
		create_args+=(--multi-az)
	else
		create_args+=(--no-multi-az)
	fi
	aws rds create-db-instance "${create_args[@]}" >/dev/null
fi

ready=0
for attempt in $(seq 1 120); do
	if [[ "$source_kind" == "cluster" ]]; then
		cluster_json="$(aws rds describe-db-clusters --region "$region" --db-cluster-identifier "$source_instance" --output json 2>/dev/null || true)"
		cluster_arn="$(jq -r '.DBClusters[0].DBClusterArn // empty' <<<"$cluster_json" 2>/dev/null || true)"
		member_json="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_member" --output json 2>/dev/null || true)"
		member_arn="$(jq -r '.DBInstances[0].DBInstanceArn // empty' <<<"$member_json" 2>/dev/null || true)"
		if [[ -n "$cluster_arn" && -n "$member_arn" ]]; then
			cluster_tags="$(aws rds list-tags-for-resource --region "$region" --resource-name "$cluster_arn" --output json 2>/dev/null || true)"
			member_tags="$(aws rds list-tags-for-resource --region "$region" --resource-name "$member_arn" --output json 2>/dev/null || true)"
			if resource_owned "$cluster_tags" && resource_owned "$member_tags" && jq -e '(.DBClusters[0].Status == "available") and (.DBClusters[0].StorageEncrypted == true) and (.DBClusters[0].DeletionProtection == true)' <<<"$cluster_json" >/dev/null 2>&1 && jq -e '(.DBInstances[0].DBInstanceStatus == "available") and (.DBInstances[0].PubliclyAccessible == true)' <<<"$member_json" >/dev/null 2>&1; then
				ready=1
				break
			fi
		fi
	else
		instance_json="$(aws rds describe-db-instances --region "$region" --db-instance-identifier "$source_instance" --output json 2>/dev/null || true)"
		arn="$(jq -r '.DBInstances[0].DBInstanceArn // empty' <<<"$instance_json" 2>/dev/null || true)"
		if [[ -n "$arn" ]]; then
			tags_json="$(aws rds list-tags-for-resource --region "$region" --resource-name "$arn" --output json 2>/dev/null || true)"
			if resource_owned "$tags_json" && jq -e '(.DBInstances[0].DBInstanceStatus == "available") and (.DBInstances[0].StorageEncrypted == true) and (.DBInstances[0].DeletionProtection == true) and (.DBInstances[0].PubliclyAccessible == true)' <<<"$instance_json" >/dev/null 2>&1; then
				ready=1
				break
			fi
		fi
	fi
	if [[ "$attempt" -lt 120 ]]; then
		sleep 10
	fi
done
if (( ready != 1 )); then
	printf 'generated AWS database source did not become available, encrypted, deletion-protected, and correctly owned\n' >&2
	exit 1
fi

(cd "$ROOT" && MAGELIFT_AWS_RDS_ROOT_PASSWORD="$master_password" go run ./cmd/aws-database-recovery-acceptance \
	--profile "$profile" \
	--region "$region" \
	--source-kind "$source_kind" \
	--instance "$source_instance" \
	--restore-subnet-group "$subnet_group" \
	--restore-security-group-id "$security_group_id" \
	--restore-class "$restore_class" \
	--marker "$marker" \
	--fixture "$fixture" \
	--mysql-image "${MAGELIFT_AWS_RDS_MYSQL_IMAGE:-mysql:8.4}")

printf 'AWS database recovery acceptance source, restore, snapshot, and subnet group were cleaned through exact ownership marker=%s sourceKind=%s\n' "$marker" "$source_kind"

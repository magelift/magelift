#!/usr/bin/env bash
# Local, credit-efficient AWS acceptance: multi-cell catalog on one logical stack.
# Dry-run (MAGELIFT_ACCEPTANCE_DRY_RUN=1): fixture path; no Pulumi/AWS mutate.
# Live: preview by default; one create-once then catalog cell updates; destroy on
# EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=acceptance/lib-checkpoint.sh
source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
# shellcheck source=acceptance/lib-evidence.sh
source "$ROOT/scripts/acceptance/lib-evidence.sh"
# shellcheck source=acceptance/lib-lifecycle.sh
source "$ROOT/scripts/acceptance/lib-lifecycle.sh"
# shellcheck source=acceptance/lib-dependencies.sh
source "$ROOT/scripts/acceptance/lib-dependencies.sh"
# shellcheck source=acceptance/lib-assert-clean-aws.sh
source "$ROOT/scripts/acceptance/lib-assert-clean-aws.sh"
# shellcheck source=acceptance/lib-cosign.sh
source "$ROOT/scripts/acceptance/lib-cosign.sh"
# shellcheck source=acceptance/lib-campaign-isolation.sh
source "$ROOT/scripts/acceptance/lib-campaign-isolation.sh"

json_dependency_status=0
acceptance_campaign_go_memlimit
acceptance_require_json_yaml_tools || json_dependency_status=1

PROFILE="${MAGELIFT_AWS_ACCEPTANCE_PROFILE:-preview}"
case "$PROFILE" in
preview|standard|high-availability) ;;
*)
	printf 'unsupported acceptance profile: %s (use preview unless credits allow more)\n' "$PROFILE" >&2
	exit 2
;;
esac

CELL_CATALOG="${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-$ROOT/scripts/acceptance/cells-aws-${PROFILE}.txt}"
DRY_RUN="${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}"

dry_run_checkpoint_path() {
	local path="${1:?checkpoint path required}"
	case "$path" in
	*.json) printf '%s-dry-run.json' "${path%.json}" ;;
	*) printf '%s-dry-run' "$path" ;;
	esac
}

acceptance_paths() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_CHECKPOINT:-}" ]]; then
		export ACCEPTANCE_CHECKPOINT="$MAGELIFT_ACCEPTANCE_CHECKPOINT"
		if [[ "$DRY_RUN" == 1 || "$DRY_RUN" == true ]]; then
			ACCEPTANCE_CHECKPOINT="$(dry_run_checkpoint_path "$ACCEPTANCE_CHECKPOINT")"
			export ACCEPTANCE_CHECKPOINT
		fi
	elif [[ "${ACCEPTANCE_CHECKPOINT:-}" != /* && "${ACCEPTANCE_CHECKPOINT:-}" != *aws-matrix* ]]; then
		if [[ "$DRY_RUN" == 1 || "$DRY_RUN" == true ]]; then
			export ACCEPTANCE_CHECKPOINT=".magelift/aws-matrix-dry-run/acceptance-checkpoint.json"
		else
			export ACCEPTANCE_CHECKPOINT=".magelift/aws-matrix/acceptance-checkpoint.json"
		fi
	fi
	if [[ -n "${MAGELIFT_ACCEPTANCE_EVIDENCE:-}" ]]; then
		export ACCEPTANCE_EVIDENCE="$MAGELIFT_ACCEPTANCE_EVIDENCE"
	elif [[ "${ACCEPTANCE_EVIDENCE:-}" != /* && "${ACCEPTANCE_EVIDENCE:-}" != *aws-matrix* ]]; then
		export ACCEPTANCE_EVIDENCE=".magelift/aws-matrix/matrix-results.md"
	fi
	acceptance_checkpoint_bind_run_id
}

load_cells() {
	local line
	CELLS=()
	if [[ ! -f "$CELL_CATALOG" ]]; then
		printf 'acceptance profile %s has no default cell catalog; set MAGELIFT_ACCEPTANCE_CELL_CATALOG explicitly: %s\n' "$PROFILE" "$CELL_CATALOG" >&2
		exit 2
	fi
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
		CELLS+=("$line")
	done <"$CELL_CATALOG"
	if [[ ${#CELLS[@]} -eq 0 ]]; then
		printf 'no cells in catalog: %s\n' "$CELL_CATALOG" >&2
		exit 2
	fi
}

dry_run_cell_loop() {
	local cell provider account date_s duration started created_once=0
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-aws}"
	account="${MAGELIFT_ACCEPTANCE_ACCOUNT:-dry-run}"
	acceptance_paths
	load_cells
	acceptance_checkpoint_load
	acceptance_evidence_ensure

	printf 'aws acceptance dry-run start cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi

		if [[ "$created_once" -eq 0 ]]; then
			printf 'acceptance create-once\n' >&2
			created_once=1
		fi
		printf 'acceptance cell-update cell=%s\n' "$cell" >&2

		started=$(date +%s)
		# Fixture success; no magelift / aws mutate.
		duration="$(( $(date +%s) - started ))s"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "PASS" "$duration" "$provider" "$account" "$date_s"
		append_shared_row "$cell" "PASS" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "PASS"
		printf 'acceptance cell-done cell=%s result=PASS\n' "$cell" >&2
	done

	printf 'aws acceptance dry-run ok; no AWS create invoked\n' >&2
}

if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
	if (( json_dependency_status != 0 )); then
		exit 2
	fi
	dry_run_cell_loop
	exit 0
fi

dependency_status="$json_dependency_status"
acceptance_require_commands aws=awscli pulumi docker || dependency_status=1
if (( dependency_status != 0 )); then
	exit 2
fi

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to an acceptance configuration file}"
: "${MAGELIFT_AWS_ACCEPTANCE_DIGEST:?set MAGELIFT_AWS_ACCEPTANCE_DIGEST to a signed immutable image reference}"
acceptance_require_certificate_identity "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:-}" "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:-}"

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

region="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
if [[ -z "$region" ]]; then
	printf 'set AWS_REGION (or AWS_DEFAULT_REGION) for leftover assertions\n' >&2
	exit 2
fi
project_tag="${MAGELIFT_AWS_ACCEPTANCE_PROJECT_TAG:-}"
if [[ -z "$project_tag" ]]; then
	printf 'set MAGELIFT_AWS_ACCEPTANCE_PROJECT_TAG to the exact disposable stack project label\n' >&2
	exit 2
fi
if ! acceptance_campaign_require_prefix aws "$project_tag"; then
	exit 2
fi
if ! acceptance_campaign_require_isolated_backend "${MAGELIFT_AWS_ACCEPTANCE_BACKEND_URL:-}"; then
	exit 2
fi

# AWS acceptance is deliberately pinned to the refreshed default profile. Do
# not let an unrelated AWS_PROFILE make the cleanup scan or Pulumi provider
# operate in another account.
export AWS_PROFILE=default

verify_acceptance_digest() {
	if [[ "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" == *sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef* ]]; then
		printf 'MAGELIFT_AWS_ACCEPTANCE_DIGEST must be a pullable OCI digest for live runs\n' >&2
		return 1
	fi
	if ! command -v docker >/dev/null 2>&1 || ! docker buildx imagetools inspect "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" >/dev/null 2>&1; then
		printf 'AWS acceptance image digest is not pullable: %s\n' "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" >&2
		return 1
	fi
}

if ! verify_acceptance_digest; then
	exit 2
fi

if [[ "${MAGELIFT_AWS_ACCEPTANCE_KEEP:-false}" == true || "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-false}" == true ]] && \
	[[ -z "${MAGELIFT_AWS_ACCEPTANCE_DIR:-}" && -z "${PULUMI_CONFIG_PASSPHRASE:-}" && -z "${PULUMI_CONFIG_PASSPHRASE_FILE:-}" ]]; then
	printf 'retained AWS acceptance sessions require MAGELIFT_AWS_ACCEPTANCE_DIR so the Pulumi passphrase survives a new shell\n' >&2
	exit 2
fi

# Working copy of config so cell updates never mutate the caller's MAGELIFT_CONFIG.
# A stable directory is required when a retained stack is resumed in another
# shell, because it holds the Pulumi passphrase and the last generated config.
if [[ -n "${MAGELIFT_AWS_ACCEPTANCE_DIR:-}" ]]; then
LIVE_WORKDIR="$MAGELIFT_AWS_ACCEPTANCE_DIR"
	mkdir -p "$LIVE_WORKDIR"
else
	LIVE_WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/magelift-aws-acceptance.XXXXXX")
fi
LIVE_CONFIG="$LIVE_WORKDIR/magelift.yaml"
cp "$MAGELIFT_CONFIG" "$LIVE_CONFIG"
configured_project="$(yq -r ".environments.\"$profile\".project.name // .project.name // \"\"" "$LIVE_CONFIG")"
if [[ -z "$configured_project" || "$configured_project" != "$project_tag" ]]; then
	printf 'MAGELIFT_AWS_ACCEPTANCE_PROJECT_TAG must exactly match the configured project.name (%s); got %s\n' "$configured_project" "$project_tag" >&2
	exit 2
fi
target_runtime="$(yq -r ".environments.\"$profile\".target.runtime // .target.runtime // \"ecs-fargate\"" "$LIVE_CONFIG")"

validate_configured_seed_dump() {
	local dump_path
	dump_path=$(yq -r ".environments.\"$profile\".seedDump // \"\"" "$LIVE_CONFIG")
	if [[ -z "$dump_path" ]]; then
		return 0
	fi
	if [[ "$dump_path" != /* ]]; then
		dump_path="$(cd "$(dirname "$MAGELIFT_CONFIG")" && pwd)/$dump_path"
	fi
	acceptance_validate_magento_seed_dump "$dump_path"
}

if ! validate_configured_seed_dump; then
	exit 2
fi

if [[ "$target_runtime" == "eks" ]]; then
	export MAGELIFT_ACCEPTANCE_EKS=1
	if [[ -z "${MAGELIFT_ACCEPTANCE_CELL_CATALOG:-}" ]]; then
		CELL_CATALOG="$ROOT/scripts/acceptance/cells-aws-eks-${profile}.txt"
	fi
fi
state_bucket=""
state_bucket_owned=0

# Do not inherit a user's global Pulumi backend selection. The AWS acceptance
# bucket is account- and region-scoped, and a stale PULUMI_BACKEND_URL or
# ~/.pulumi/credentials.json entry can point at a deleted bucket before the
# harness has a chance to record ownership or run its cleanup path. An
# explicitly named acceptance backend is an opt-out from disposable-bucket
# ownership; every other run gets a deterministic bucket that this process can
# create and delete.
state_account="${MAGELIFT_ACCEPTANCE_ACCOUNT:-$(aws sts get-caller-identity --query Account --output text)}"
state_project="${MAGELIFT_AWS_ACCEPTANCE_STATE_PROJECT:-$project_tag}"
if [[ -n "${MAGELIFT_AWS_ACCEPTANCE_BACKEND_URL:-}" ]]; then
	export PULUMI_BACKEND_URL="$MAGELIFT_AWS_ACCEPTANCE_BACKEND_URL"
	printf '+ using explicitly configured AWS Pulumi backend %s\n' "$PULUMI_BACKEND_URL"
else
	state_bucket="${MAGELIFT_AWS_ACCEPTANCE_STATE_BUCKET:-magelift-${state_account}-${region}-${state_project}-${profile}-state}"
	export PULUMI_BACKEND_URL="s3://${state_bucket}?region=${region}&awssdk=v2"
	printf '+ using AWS Pulumi backend %s\n' "$PULUMI_BACKEND_URL"
fi

if [[ -z "${PULUMI_CONFIG_PASSPHRASE:-}" && -z "${PULUMI_CONFIG_PASSPHRASE_FILE:-}" ]]; then
	PULUMI_CONFIG_PASSPHRASE_FILE="$LIVE_WORKDIR/pulumi-passphrase"
	if [[ ! -s "$PULUMI_CONFIG_PASSPHRASE_FILE" ]]; then
		if ! openssl rand -hex 32 >"$PULUMI_CONFIG_PASSPHRASE_FILE"; then
			printf 'unable to create the temporary Pulumi passphrase file\n' >&2
			exit 2
		fi
		printf '+ created the AWS acceptance Pulumi passphrase file\n'
	else
		printf '+ reusing the AWS acceptance Pulumi passphrase file\n'
	fi
	chmod 600 "$PULUMI_CONFIG_PASSPHRASE_FILE"
	export PULUMI_CONFIG_PASSPHRASE_FILE
	printf '+ using a temporary Pulumi passphrase file (value not logged)\n'
fi

cleanup_workdir() {
	rm -rf "$LIVE_WORKDIR"
}

config=("$MAGELIFT_BIN" --config "$LIVE_CONFIG" --env "$profile" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

MAGELIFT_ACCEPTANCE_OWNER="${MAGELIFT_AWS_ACCEPTANCE_OWNER:-magelift-acceptance-${project_tag}-${profile}}"
export MAGELIFT_ACCEPTANCE_OWNER
export MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile"
apply_acceptance_labels() {
	if ! command -v yq >/dev/null 2>&1; then
		printf 'yq is required to add the exact acceptance ownership marker\n' >&2
		return 2
	fi
	yq -i '.target.aws.labels."magelift:acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
	yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.labels."magelift:acceptance-run" = strenv(MAGELIFT_ACCEPTANCE_OWNER)' "$LIVE_CONFIG"
	# The acceptance TTL is a safety boundary for interrupted runs. It is not a
	# substitute for the EXIT destroy and direct inventory assertion.
	yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].expiresAt = strenv(ACCEPTANCE_EXPIRES_AT)' "$LIVE_CONFIG"
}

ensure_state_bucket() {
	if [[ -z "$state_bucket" ]]; then
		return 0
	fi

	local owner_marker existing_owner state_kms_arn tags_json
	owner_marker="$MAGELIFT_ACCEPTANCE_OWNER"
	if aws s3api head-bucket --bucket "$state_bucket" --region "$region" >/dev/null 2>&1; then
		existing_owner=$(aws s3api get-bucket-tagging --bucket "$state_bucket" --region "$region" --query 'TagSet[?Key==`magelift:acceptance-run`].Value | [0]' --output text 2>/dev/null || true)
		if [[ "$existing_owner" != "$owner_marker" ]]; then
			printf 'AWS acceptance state bucket exists without this run ownership marker: %s\n' "$state_bucket" >&2
			return 1
		fi
		state_bucket_owned=1
		printf '+ reusing this run AWS Pulumi state bucket %s\n' "$state_bucket"
		return 0
	fi

	printf '+ creating AWS Pulumi state bucket %s\n' "$state_bucket"
	if [[ "$region" == "us-east-1" ]]; then
		aws s3api create-bucket --bucket "$state_bucket" --region "$region" >/dev/null
	else
		aws s3api create-bucket --bucket "$state_bucket" --region "$region" --create-bucket-configuration "LocationConstraint=$region" >/dev/null
	fi
	state_bucket_owned=1
	aws s3api put-public-access-block --bucket "$state_bucket" --region "$region" --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
	aws s3api put-bucket-ownership-controls --bucket "$state_bucket" --region "$region" --ownership-controls 'Rules=[{ObjectOwnership=BucketOwnerEnforced}]'
	aws s3api put-bucket-versioning --bucket "$state_bucket" --region "$region" --versioning-configuration Status=Enabled
	state_kms_arn=$(yq -r '.target.aws.kmsKeyArn // ""' "$LIVE_CONFIG")
	if [[ -n "$state_kms_arn" ]]; then
		aws s3api put-bucket-encryption --bucket "$state_bucket" --region "$region" --server-side-encryption-configuration "{\"Rules\":[{\"ApplyServerSideEncryptionByDefault\":{\"SSEAlgorithm\":\"aws:kms\",\"KMSMasterKeyID\":\"$state_kms_arn\"},\"BucketKeyEnabled\":true}]}"
	else
		aws s3api put-bucket-encryption --bucket "$state_bucket" --region "$region" --server-side-encryption-configuration '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
	fi
	tags_json=$(jq -cn --arg project "$state_project" --arg environment "$profile" --arg owner "$owner_marker" '{TagSet:[{Key:"magelift:project",Value:$project},{Key:"magelift:environment",Value:$environment},{Key:"magelift:managed-by",Value:"magelift"},{Key:"magelift:purpose",Value:"acceptance-state"},{Key:"magelift:acceptance-run",Value:$owner}]}' )
	aws s3api put-bucket-tagging --bucket "$state_bucket" --region "$region" --tagging "$tags_json"
	wait_for_state_bucket
}

wait_for_state_bucket() {
	local timeout="${MAGELIFT_AWS_ACCEPTANCE_STATE_BUCKET_READY_TIMEOUT_SECS:-60}"
	local interval="${MAGELIFT_AWS_ACCEPTANCE_STATE_BUCKET_READY_INTERVAL_SECS:-1}"
	local started now
	started=$(date +%s)
	while ! aws s3api head-bucket --bucket "$state_bucket" --region "$region" >/dev/null 2>&1 || \
		! aws s3api list-objects-v2 --bucket "$state_bucket" --region "$region" --max-keys 1 >/dev/null 2>&1; do
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf 'AWS acceptance state bucket did not become readable within %ss: %s\n' "$timeout" "$state_bucket" >&2
			return 1
		fi
		sleep "$interval"
	done
	printf '+ AWS Pulumi state bucket is readable %s\n' "$state_bucket"
}

# Seed a disposable AWS EKS database through a VPC-adjacent client pod. The
# local machine cannot reach the RDS endpoint, and the application image does
# not ship a MySQL client. The database grant Job is part of the EKS runtime;
# this helper only imports the configured dump and records the seed journal.
decode_base64() {
	if base64 --decode </dev/null >/dev/null 2>&1; then
		base64 --decode
	else
		base64 -D
	fi
}

# Magelift uses automation.NewInlineStackWithBackend(..., stackName, "magelift", backendURL).
# `magelift outputs` prints the unqualified stack name; Pulumi --stack needs org/project/stack.
qualify_aws_pulumi_stack_ref() {
	local stack_ref="${1:?stack ref required}"
	local ref
	if [[ "$stack_ref" == */* ]]; then
		printf '%s' "$stack_ref"
		return 0
	fi
	if command -v jq >/dev/null 2>&1 && command -v pulumi >/dev/null 2>&1; then
		ref="$(PULUMI_BACKEND_URL="${PULUMI_BACKEND_URL:-}" pulumi stack ls --all --json 2>/dev/null | jq -r --arg name "$stack_ref" '
			.[] | select(.name == $name or (.name | endswith("/" + $name))) | .name' | head -n 1)"
		if [[ -n "$ref" && "$ref" != "null" ]]; then
			printf '%s' "$ref"
			return 0
		fi
	fi
	printf 'organization/magelift/%s' "$stack_ref"
}

seed_eks_database() {
	local dump_path outputs_file stack_ref db_host db_name db_secret_name db_user db_pass
	local kc_path dump_pod seed_image
	dump_path=$(yq -r ".environments.\"$profile\".seedDump // \"\"" "$LIVE_CONFIG")
	if [[ -z "$dump_path" ]]; then
		return 0
	fi
	if [[ "$dump_path" != /* ]]; then
		dump_path="$(cd "$(dirname "$MAGELIFT_CONFIG")" && pwd)/$dump_path"
	fi
	if [[ ! -r "$dump_path" ]]; then
		printf 'configured AWS EKS seed dump is not readable: %s\n' "$dump_path" >&2
		return 1
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v pulumi >/dev/null 2>&1; then
		printf 'AWS EKS seed import requires kubectl and pulumi on PATH\n' >&2
		return 1
	fi

	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if ! run outputs >"$outputs_file"; then
		printf 'unable to read AWS EKS outputs before seed import\n' >&2
		return 1
	fi
	stack_ref=$(jq -r '.stack // empty' "$outputs_file")
	db_host=$(jq -r '.outputs.databaseWriter // empty' "$outputs_file")
	db_name=$(yq -r ".target.aws.databaseName // .environments.\"$profile\".target.aws.databaseName // \"magento\"" "$LIVE_CONFIG")
	db_secret_name=$(jq -r '.outputs.databaseSecretName // empty' "$outputs_file")
	if [[ -z "$stack_ref" || -z "$db_host" || -z "$db_name" || -z "$db_secret_name" ]]; then
		printf 'AWS EKS outputs lack stack, database endpoint, database name, or database Secret for seed import\n' >&2
		return 1
	fi

	kc_path="$LIVE_WORKDIR/eks-seed.kubeconfig"
	stack_ref="$(qualify_aws_pulumi_stack_ref "$stack_ref")"
	if ! PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" \
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi stack output kubeconfig --stack "$stack_ref" --show-secrets >"$kc_path"; then
		printf 'AWS EKS seed import could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"
	if ! db_user="$(kubectl --kubeconfig="$kc_path" get secret "$db_secret_name" -o jsonpath='{.data.username}' | decode_base64)" || \
		! db_pass="$(kubectl --kubeconfig="$kc_path" get secret "$db_secret_name" -o jsonpath='{.data.password}' | decode_base64)"; then
		printf 'AWS EKS seed import could not read database credentials Secret\n' >&2
		return 1
	fi
	if [[ -z "$db_user" || -z "$db_pass" ]]; then
		printf 'AWS EKS database credentials Secret is incomplete\n' >&2
		return 1
	fi

	MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile" MAGELIFT_ACCEPTANCE_SEED_DUMP="$dump_path" \
		yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].seedDump = strenv(MAGELIFT_ACCEPTANCE_SEED_DUMP)' "$LIVE_CONFIG"
	mkdir -p "$LIVE_WORKDIR/.magelift/seed-dumps"
	python3 - "$LIVE_WORKDIR" "$profile" "$dump_path" <<'PY'
import datetime
import json
import pathlib
import sys

root, environment, dump_path = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3]
journal = root / ".magelift" / "seed-dumps" / f"{environment}.json"
journal.write_text(json.dumps({
    "status": "recorded",
    "dumpPath": dump_path,
    "updatedAt": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
}, indent=2) + "\n")
print(f"seed dump journal recorded at {journal}")
PY

	seed_image="${MAGELIFT_AWS_ACCEPTANCE_SEED_IMAGE:-public.ecr.aws/docker/library/mysql:8.4}"
	dump_pod="magelift-${profile}-seed-import"
	kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=true >/dev/null 2>&1 || true
	if ! kubectl --kubeconfig="$kc_path" run "$dump_pod" --image="$seed_image" --restart=Never --command -- sleep 1800; then
		printf 'AWS EKS seed import could not start mysql client pod\n' >&2
		return 1
	fi
	if ! kubectl --kubeconfig="$kc_path" wait --for=condition=Ready "pod/$dump_pod" --timeout=180s; then
		printf 'AWS EKS mysql client pod did not become ready\n' >&2
		return 1
	fi
	if ! MAGELIFT_DUMPIMPORT_RUNNER=kube \
		MAGELIFT_DUMPIMPORT_HOST="$db_host" \
		MAGELIFT_DUMPIMPORT_PASSWORD="$db_pass" \
		MAGELIFT_DUMPIMPORT_USER="$db_user" \
		MAGELIFT_DUMPIMPORT_DATABASE="$db_name" \
		MAGELIFT_DUMPIMPORT_POD="$dump_pod" \
		MAGELIFT_DUMPIMPORT_KUBECONFIG="$kc_path" \
		"${config[@]}" env import-dump "$profile" 2>&1 | tee "$LIVE_WORKDIR/eks-seed-import.json"; then
		kubectl --kubeconfig="$kc_path" logs "$dump_pod" >"$LIVE_WORKDIR/eks-seed-import-pod.log" 2>&1 || true
		printf 'AWS EKS seed import failed\n' >&2
		return 1
	fi
	kubectl --kubeconfig="$kc_path" delete pod "$dump_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
	printf 'AWS EKS seed import completed database=%s\n' "$db_name" >&2
}

# Seed a disposable AWS database through the private ECS network. The local
# machine cannot reach the RDS endpoint, and the application image deliberately
# does not ship a MySQL client. A short-lived public MySQL client task consumes
# a presigned object from the already-owned state bucket, then is deregistered.
seed_database_if_configured() {
	local dump_path cluster task_definition base_definition seed_definition seed_family
	local outputs_file seed_key dump_url db_host db_name db_secret_arn container_name
	local seed_image seed_command seed_task_definition seed_task exit_code stopped_reason
	local network_config started now placement_args provider
	if [[ "$target_runtime" == "eks" ]]; then
		seed_eks_database
		return $?
	fi
	dump_path=$(yq -r ".environments.\"$profile\".seedDump // \"\"" "$LIVE_CONFIG")
	if [[ -z "$dump_path" ]]; then
		return 0
	fi
	if [[ "$dump_path" != /* ]]; then
		dump_path="$(cd "$(dirname "$MAGELIFT_CONFIG")" && pwd)/$dump_path"
	fi
	if [[ ! -r "$dump_path" ]]; then
		printf 'configured AWS seed dump is not readable: %s\n' "$dump_path" >&2
		return 1
	fi
	if [[ -z "$state_bucket" || "$state_bucket_owned" != 1 ]]; then
		printf 'AWS seed import requires the harness-owned state bucket\n' >&2
		return 1
	fi

	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if ! run outputs >"$outputs_file"; then
		printf 'unable to read AWS outputs before seed import\n' >&2
		return 1
	fi
	cluster=$(jq -r '.outputs.clusterName // empty' "$outputs_file")
	task_definition=$(jq -r '.outputs.deployTaskDefinitionArn // empty' "$outputs_file")
	db_host=$(jq -r '.outputs.databaseWriter // empty' "$outputs_file")
	db_name=$(yq -r ".target.aws.databaseName // .environments.\"$profile\".target.aws.databaseName // \"magento\"" "$LIVE_CONFIG")
	if [[ -z "$cluster" || -z "$task_definition" || -z "$db_host" || -z "$db_name" ]]; then
		printf 'AWS outputs lack cluster, deploy task, database endpoint, or database name for seed import\n' >&2
		return 1
	fi

	base_definition="$LIVE_WORKDIR/seed-base-task-definition.json"
	if ! aws ecs describe-task-definition --region "$region" --task-definition "$task_definition" --query 'taskDefinition' --output json >"$base_definition"; then
		printf 'unable to read the AWS deploy task definition for seed import\n' >&2
		return 1
	fi
	db_secret_arn=$(jq -r '.containerDefinitions[0].secrets[]? | select(.name == "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD") | .valueFrom' "$base_definition" | sed 's/:password::$//' | head -n 1)
	container_name=$(jq -r '.containerDefinitions[0].name // "deploy"' "$base_definition")
	if [[ -z "$db_secret_arn" ]]; then
		printf 'AWS deploy task definition does not expose the managed database secret\n' >&2
		return 1
	fi

	seed_key=".magelift-acceptance/seed/${ACCEPTANCE_EVIDENCE_RUN_ID}.sql.gz"
	if ! aws s3 cp "$dump_path" "s3://${state_bucket}/${seed_key}" --region "$region" --no-progress >/dev/null; then
		printf 'unable to upload the AWS seed dump to the disposable state bucket\n' >&2
		return 1
	fi
	dump_url=$(aws s3 presign "s3://${state_bucket}/${seed_key}" --region "$region" --expires-in 3600)
	seed_image="${MAGELIFT_AWS_ACCEPTANCE_SEED_IMAGE:-public.ecr.aws/docker/library/mysql:8.4}"
	seed_command='set -eu; if ! command -v curl >/dev/null 2>&1; then if command -v microdnf >/dev/null 2>&1; then microdnf install -y curl; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl; elif command -v apt-get >/dev/null 2>&1; then apt-get update; DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl; else echo "seed import needs curl" >&2; exit 127; fi; fi; dump_file=$(mktemp); raw_file=$(mktemp); sql_file=$(mktemp); trap "rm -f $dump_file $raw_file $sql_file" EXIT; curl -fsSL "$DUMP_URL" -o "$dump_file"; gzip -dc "$dump_file" > "$raw_file"; sed -E "s/DEFINER=[^[:space:]]+[[:space:]]*//g" "$raw_file" > "$sql_file"; MYSQL_PWD="$DB_PASSWORD" mysql --protocol=TCP -h "$DB_HOST" -P 3306 -u "$DB_USER" "$DB_NAME" < "$sql_file"'

	seed_family="${project_tag}-${profile}-seed-import-$(date -u +%Y%m%d%H%M%S)"
	seed_definition="$LIVE_WORKDIR/seed-task-definition.json"
	if ! jq --arg family "$seed_family" --arg image "$seed_image" --arg name "$container_name" --arg command "$seed_command" --arg dump_url "$dump_url" --arg db_host "$db_host" --arg db_name "$db_name" --arg db_secret "$db_secret_arn" -f "$ROOT/scripts/acceptance/aws-seed-task-definition.jq" "$base_definition" >"$seed_definition"; then
		printf 'unable to build the temporary AWS seed task definition\n' >&2
		return 1
	fi
	seed_task_definition=$(aws ecs register-task-definition --region "$region" --cli-input-json "file://$seed_definition" --query 'taskDefinition.taskDefinitionArn' --output text)
	if [[ -z "$seed_task_definition" || "$seed_task_definition" == None ]]; then
		printf 'AWS seed task definition registration returned no ARN\n' >&2
		return 1
	fi
	network_config=$(jq -cn --argjson subnets "$(jq -c '.outputs.privateSubnetIds' "$outputs_file")" --arg security_group "$(jq -r '.outputs.securityGroupId' "$outputs_file")" '{awsvpcConfiguration:{subnets:$subnets,securityGroups:[$security_group],assignPublicIp:"DISABLED"}}')
	# Managed Instances and EC2 Auto Scaling clusters do not offer Fargate.
	# CreateCapacityProvider already bound the only usable provider.
	placement_args=(--launch-type FARGATE)
	case "$(configured_compute_mode)" in
	managed-instances|ec2-asg)
		provider="$(aws ecs describe-clusters --region "$region" --clusters "$cluster" --query 'clusters[0].capacityProviders[0]' --output text)"
		if [[ -z "$provider" || "$provider" == None ]]; then
			aws ecs deregister-task-definition --region "$region" --task-definition "$seed_task_definition" >/dev/null 2>&1 || true
			printf 'AWS seed import needs a cluster capacity provider for computeMode %s\n' "$(configured_compute_mode)" >&2
			return 1
		fi
		placement_args=(--capacity-provider-strategy "capacityProvider=${provider},weight=1")
		;;
	esac
	seed_task=$(aws ecs run-task --region "$region" --cluster "$cluster" --task-definition "$seed_task_definition" "${placement_args[@]}" --network-configuration "$network_config" --query 'tasks[0].taskArn' --output text)
	if [[ -z "$seed_task" || "$seed_task" == None ]]; then
		aws ecs deregister-task-definition --region "$region" --task-definition "$seed_task_definition" >/dev/null 2>&1 || true
		printf 'AWS seed task did not start\n' >&2
		return 1
	fi
	printf '+ waiting for AWS seed task\n' >&2
	started=$(date +%s)
	while true; do
		exit_code=$(aws ecs describe-tasks --region "$region" --cluster "$cluster" --tasks "$seed_task" --query 'tasks[0].containers[0].exitCode' --output text 2>/dev/null || true)
		if [[ "$exit_code" != None && -n "$exit_code" ]]; then
			break
		fi
		now=$(date +%s)
		if (( now - started >= ${MAGELIFT_AWS_ACCEPTANCE_SEED_TIMEOUT_SECS:-1200} )); then
			aws ecs stop-task --region "$region" --cluster "$cluster" --task "$seed_task" --reason 'MageLift acceptance seed timeout' >/dev/null 2>&1 || true
			aws ecs deregister-task-definition --region "$region" --task-definition "$seed_task_definition" >/dev/null 2>&1 || true
			printf 'AWS seed task timed out\n' >&2
			return 1
		fi
		sleep 10
	done
	stopped_reason=$(aws ecs describe-tasks --region "$region" --cluster "$cluster" --tasks "$seed_task" --query 'tasks[0].stoppedReason' --output text 2>/dev/null || true)
	if [[ "$exit_code" != 0 ]]; then
		printf 'AWS seed task failed: exit_code=%s reason=%s\n' "$exit_code" "${stopped_reason:-not reported by ECS}" >&2
		aws ecs deregister-task-definition --region "$region" --task-definition "$seed_task_definition" >/dev/null 2>&1 || true
		return 1
	fi
	aws ecs deregister-task-definition --region "$region" --task-definition "$seed_task_definition" >/dev/null 2>&1 || true
	aws s3api delete-object --bucket "$state_bucket" --key "$seed_key" --region "$region" >/dev/null 2>&1 || true
	printf 'AWS seed import completed database=%s\n' "$db_name" >&2
}

# Managed services and load balancers can report healthy before the runtime is
# actually ready. Keep this bounded and reuse the same check for create-once
# and cell updates.
run_runtime_health_with_retry() {
	local timeout="${MAGELIFT_AWS_ACCEPTANCE_RUNTIME_TIMEOUT_SECS:-600}"
	local interval="${MAGELIFT_AWS_ACCEPTANCE_RUNTIME_INTERVAL_SECS:-15}"
	local started now
	started=$(date +%s)
	while true; do
		if [[ "$target_runtime" == "eks" ]]; then
			aws_apply_magento_storefront_base_url || true
		fi
		if run health --mode runtime; then
			return 0
		fi
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf 'AWS runtime health did not become ready after %ss\n' "$timeout" >&2
			return 1
		fi
		printf 'AWS runtime health is not ready; retrying in %ss\n' "$interval" >&2
		sleep "$interval"
	done
}

aws_ipv4_is_public() {
	local host="${1:-}"
	[[ -n "$host" ]] || return 1
	python3 - "$host" <<'PY'
import socket
import sys

host = sys.argv[1]
try:
    ip = socket.getaddrinfo(host, None, socket.AF_INET)[0][4][0]
except OSError:
    sys.exit(1)
parts = [int(part) for part in ip.split(".")]
private = (
    parts[0] == 10
    or parts[0] == 127
    or (parts[0] == 192 and parts[1] == 168)
    or (parts[0] == 172 and 16 <= parts[1] <= 31)
    or (parts[0] == 169 and parts[1] == 254)
)
sys.exit(0 if not private else 1)
PY
}

aws_eks_web_service_name() {
	local outputs_file svc
	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if [[ ! -f "$outputs_file" ]] && ! run outputs >"$outputs_file"; then
		return 1
	fi
	svc=$(jq -r '.outputs.serviceName // .serviceName // empty' "$outputs_file")
	[[ -n "$svc" && "$svc" != "null" ]] || return 1
	printf '%s' "$svc"
}

aws_eks_kubeconfig_path() {
	local kc_path outputs_file stack_ref
	kc_path="$LIVE_WORKDIR/eks-seed.kubeconfig"
	if [[ -f "$kc_path" ]]; then
		printf '%s' "$kc_path"
		return 0
	fi
	kc_path="$LIVE_WORKDIR/eks-storefront.kubeconfig"
	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	stack_ref=$(jq -r '.stack // empty' "$outputs_file")
	[[ -n "$stack_ref" ]] || return 1
	stack_ref="$(qualify_aws_pulumi_stack_ref "$stack_ref")"
	if ! PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" \
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi stack output kubeconfig --stack "$stack_ref" --show-secrets >"$kc_path"; then
		return 1
	fi
	chmod 600 "$kc_path"
	printf '%s' "$kc_path"
}

aws_eks_public_storefront_host() {
	local kc_path svc host
	kc_path="$(aws_eks_kubeconfig_path)" || return 1
	svc="$(aws_eks_web_service_name)" || return 1
	host="$(kubectl --kubeconfig="$kc_path" get svc "$svc" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
	if [[ -z "$host" ]]; then
		host="$(kubectl --kubeconfig="$kc_path" get svc "$svc" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
	fi
	[[ -n "$host" ]] || return 1
	aws_ipv4_is_public "$host" || return 1
	printf '%s' "$host"
}

# Seed dumps keep Magento base_url on localhost. CheckRuntime GETs the Service
# LoadBalancer and treats a 302 to localhost as unhealthy.
aws_apply_magento_storefront_base_url() {
	local host base_url
	[[ "$target_runtime" == "eks" ]] || return 0
	host="$(aws_eks_public_storefront_host)" || return 1
	if [[ "${eks_storefront_base_url_applied:-}" == "$host" ]]; then
		return 0
	fi
	base_url="http://${host}/"
	printf '+ setting Magento storefront base_url=%q (public origin URL, not a secret)\n' "$base_url" >&2
	if ! run exec --service web -- bin/magento --no-ansi config:set web/unsecure/base_url "$base_url" 2>&1 | tee "$LIVE_WORKDIR/cell-storefront-base-url-unsecure.log"; then
		printf 'could not set Magento web/unsecure/base_url\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi config:set web/secure/base_url "$base_url" 2>&1 | tee "$LIVE_WORKDIR/cell-storefront-base-url-secure.log"; then
		printf 'could not set Magento web/secure/base_url\n' >&2
		return 1
	fi
	if ! run exec --service web -- bin/magento --no-ansi cache:flush 2>&1 | tee "$LIVE_WORKDIR/cell-storefront-base-url-cache-flush.log"; then
		printf 'could not flush Magento cache after storefront base_url\n' >&2
		return 1
	fi
	eks_storefront_base_url_applied="$host"
}

# Inject one exact EC2 failure into a self-managed EKS ASG. The node provider
# identity, cluster tag, ASG membership, and InService replacement are all
# checked before a terminate call so this cannot become a project-wide or
# name-prefix cleanup path.
run_aws_eks_node_loss_cell() {
	local timeout_secs poll_interval elapsed fault_started measured_rto
	local outputs_file stack_ref cluster_name kc_path nodes_json ready_before ready_after
	local ready_zones_before ready_zones_after ready_nodes_before node_record node_name provider_id zone
	local instance_id instance_json instance_state asg_name asg_json asg_min asg_desired asg_max
	local replacement_instance replacement_node replacement_instance_json replacement_state
	local old_ready before_node_names old_instance_state
	if [[ "$target_runtime" != "eks" || "$(configured_compute_mode)" != "self-managed" ]]; then
		printf 'resilience:node-loss requires EKS computeMode:self-managed\n' >&2
		return 2
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'resilience:node-loss requires kubectl and jq on PATH\n' >&2
		return 2
	fi
	timeout_secs="${MAGELIFT_AWS_NODE_LOSS_TIMEOUT_SECS:-900}"
	poll_interval="${MAGELIFT_AWS_NODE_LOSS_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:node-loss timeout and poll interval must be positive integers\n' >&2
		return 2
	fi

	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if ! run outputs >"$outputs_file"; then
		printf 'resilience:node-loss could not read stack outputs\n' >&2
		return 1
	fi
	stack_ref="$(jq -r '.stack // empty' "$outputs_file")"
	cluster_name="$(jq -r '.outputs.clusterName // empty' "$outputs_file")"
	if [[ -z "$stack_ref" || -z "$cluster_name" ]]; then
		printf 'resilience:node-loss outputs lack stack or cluster name\n' >&2
		return 1
	fi
	stack_ref="$(qualify_aws_pulumi_stack_ref "$stack_ref")"
	kc_path="$LIVE_WORKDIR/node-loss.kubeconfig"
	if ! PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" \
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi stack output kubeconfig --stack "$stack_ref" --show-secrets >"$kc_path"; then
		printf 'resilience:node-loss could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"

	if ! nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json)"; then
		printf 'resilience:node-loss could not list EKS nodes\n' >&2
		return 1
	fi
	printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/cell-node-loss-nodes-before.json"
	ready_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length')"
	ready_zones_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length')"
	ready_nodes_before="$(printf '%s' "$nodes_json" | jq -c '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name] | sort')"
	before_node_names="$ready_nodes_before"
	if [[ ! "$ready_before" =~ ^[0-9]+$ || "$ready_before" -lt 2 || ! "$ready_zones_before" =~ ^[0-9]+$ || "$ready_zones_before" -lt 2 ]]; then
		printf 'resilience:node-loss requires at least two Ready self-managed nodes across two zones (ready=%s zones=%s)\n' "$ready_before" "$ready_zones_before" >&2
		return 1
	fi
	node_record="$(printf '%s' "$nodes_json" | jq -r '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | [.metadata.name, .spec.providerID, .metadata.labels["topology.kubernetes.io/zone"] // ""]] | sort_by(.[0]) | .[0] | @tsv')"
	IFS=$'\t' read -r node_name provider_id zone <<<"$node_record"
	if [[ -z "$node_name" || -z "$provider_id" || -z "$zone" || "$provider_id" != aws:///* ]]; then
		printf 'resilience:node-loss selected node lacks an AWS provider identity (node=%s)\n' "$node_name" >&2
		return 1
	fi
	instance_id="${provider_id##*/}"
	if [[ ! "$instance_id" =~ ^i-[0-9a-f]{8,}$ || ! "$zone" =~ ^[a-z0-9-]+$ ]]; then
		printf 'resilience:node-loss selected provider identity is invalid (node=%s instance=%s zone=%s)\n' "$node_name" "$instance_id" "$zone" >&2
		return 1
	fi
	if ! instance_json="$(aws ec2 describe-instances --region "$region" --instance-ids "$instance_id" --output json)"; then
		printf 'resilience:node-loss could not verify exact backing EC2 instance=%s\n' "$instance_id" >&2
		return 1
	fi
	instance_state="$(printf '%s' "$instance_json" | jq -r '.Reservations[0].Instances[0].State.Name // empty')"
	if [[ "$instance_state" != running ]]; then
		printf 'resilience:node-loss backing instance is not running: %s=%s\n' "$instance_id" "$instance_state" >&2
		return 1
	fi
	if ! printf '%s' "$instance_json" | jq -e --arg cluster "$cluster_name" '
		.Reservations[0].Instances[0].Tags // []
		| any(.[]; .Key == "aws:eks:cluster-name" and .Value == $cluster)' >/dev/null; then
		printf 'resilience:node-loss backing instance is not tagged for cluster=%s\n' "$cluster_name" >&2
		return 1
	fi
	asg_name="$(aws autoscaling describe-auto-scaling-instances --region "$region" --instance-ids "$instance_id" --query 'AutoScalingInstances[0].AutoScalingGroupName' --output text)"
	if [[ -z "$asg_name" || "$asg_name" == None || "$asg_name" != "$cluster_name"*self-managed* ]]; then
		printf 'resilience:node-loss backing instance is not in the self-managed ASG: %s\n' "$asg_name" >&2
		return 1
	fi
	if ! asg_json="$(aws autoscaling describe-auto-scaling-groups --region "$region" --auto-scaling-group-names "$asg_name" --output json)"; then
		printf 'resilience:node-loss could not read ASG=%s\n' "$asg_name" >&2
		return 1
	fi
	asg_min="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].MinSize // 0')"
	asg_desired="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].DesiredCapacity // 0')"
	asg_max="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].MaxSize // 0')"
	if [[ ! "$asg_min" =~ ^[0-9]+$ || ! "$asg_desired" =~ ^[0-9]+$ || ! "$asg_max" =~ ^[0-9]+$ || "$asg_min" -lt 2 || "$asg_desired" -lt 2 || "$asg_max" -lt "$asg_desired" ]]; then
		printf 'resilience:node-loss requires an ASG with min/desired/max >= 2/%s/%s (observed %s/%s/%s)\n' "$asg_desired" "$asg_max" "$asg_min" "$asg_desired" "$asg_max" >&2
		return 1
	fi
	if ! printf '%s' "$asg_json" | jq -e --arg instance "$instance_id" '[.AutoScalingGroups[0].Instances[] | select(.InstanceId == $instance and .LifecycleState == "InService" and .HealthStatus == "Healthy")] | length == 1' >/dev/null; then
		printf 'resilience:node-loss selected instance is not a healthy InService ASG member: %s\n' "$instance_id" >&2
		return 1
	fi

	jq -n --arg node "$node_name" --arg providerID "$provider_id" --arg instance "$instance_id" --arg zone "$zone" \
		--arg asg "$asg_name" --argjson ready "$ready_before" --argjson zones "$ready_zones_before" \
		'{node:$node,providerID:$providerID,instanceID:$instance,zone:$zone,asg:$asg,readyNodes:$ready,readyZones:$zones}' \
		>"$LIVE_WORKDIR/cell-node-loss-injection.json"
	fault_started="$(date +%s)"
	printf '+ resilience:node-loss terminating exact EC2 instance=%s node=%s zone=%s asg=%s\n' "$instance_id" "$node_name" "$zone" "$asg_name" >&2
	if ! aws ec2 terminate-instances --region "$region" --instance-ids "$instance_id" --output json >"$LIVE_WORKDIR/cell-node-loss-termination.json"; then
		printf 'resilience:node-loss exact EC2 termination failed: %s\n' "$instance_id" >&2
		return 1
	fi

	replacement_instance=""
	replacement_node=""
	replacement_instance_json='{}'
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		asg_json="$(aws autoscaling describe-auto-scaling-groups --region "$region" --auto-scaling-group-names "$asg_name" --output json 2>/dev/null || printf '{"AutoScalingGroups":[]}')"
		replacement_instance="$(printf '%s' "$asg_json" | jq -r --arg old "$instance_id" '.AutoScalingGroups[0].Instances[]? | select(.InstanceId != $old and .LifecycleState == "InService" and .HealthStatus == "Healthy") | .InstanceId' | head -n 1)"
		if [[ -n "$replacement_instance" ]]; then
			replacement_instance_json="$(aws ec2 describe-instances --region "$region" --instance-ids "$replacement_instance" --output json 2>/dev/null || printf '{}')"
			replacement_state="$(printf '%s' "$replacement_instance_json" | jq -r '.Reservations[0].Instances[0].State.Name // empty')"
		else
			replacement_state=""
		fi
		nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json 2>/dev/null || printf '{"items":[]}')"
		ready_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length' 2>/dev/null || printf '0')"
		ready_zones_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length' 2>/dev/null || printf '0')"
		old_ready="$(printf '%s' "$nodes_json" | jq -r --arg old "$node_name" '[.items[] | select(.metadata.name == $old) | .status.conditions[]? | select(.type == "Ready" and .status == "True")] | length' 2>/dev/null || printf '0')"
		if [[ -n "$replacement_instance" && "$replacement_state" == running ]]; then
			replacement_node="$(printf '%s' "$nodes_json" | jq -r --arg old "$node_name" --arg oldProvider "$provider_id" --arg replacement "$replacement_instance" --argjson before "$before_node_names" '
				.items[]
				| select(.metadata.name != $old and .spec.providerID != $oldProvider)
				| select(.spec.providerID | endswith("/" + $replacement))
				| select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
				| select(.metadata.name as $current | (any($before[]; . == $current) | not))
				| .metadata.name' 2>/dev/null | head -n 1)"
		fi
		if [[ "$ready_after" =~ ^[0-9]+$ && "$ready_after" -ge "$ready_before" && "$ready_zones_after" =~ ^[0-9]+$ && "$ready_zones_after" -ge "$ready_zones_before" && -n "$replacement_node" ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/failure-node-loss-nodes.json"
			printf 'replacementInstance=%s\nreplacementState=%s\noldReady=%s\nreadyAfter=%s\nzonesAfter=%s\n' \
				"$replacement_instance" "$replacement_state" "$old_ready" "$ready_after" "$ready_zones_after" >"$LIVE_WORKDIR/failure-node-loss.log"
			printf 'resilience:node-loss timed out replacementInstance=%s replacementNode=%s ready=%s zones=%s oldReady=%s\n' \
				"$replacement_instance" "$replacement_node" "$ready_after" "$ready_zones_after" "$old_ready" >&2
			return 1
		fi
		sleep "$poll_interval"
	done

	printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/cell-node-loss-nodes-after.json"
	printf '%s\n' "$replacement_instance_json" >"$LIVE_WORKDIR/cell-node-loss-instance-after.json"
	if ! run_runtime_health_with_retry; then
		printf 'resilience:node-loss runtime health failed after replacement node became Ready\n' >&2
		return 1
	fi
	measured_rto="$(( $(date +%s) - fault_started ))"
	old_instance_state="$(aws ec2 describe-instances --region "$region" --instance-ids "$instance_id" --query 'Reservations[0].Instances[0].State.Name' --output text 2>/dev/null || printf 'not-found')"
	jq -n --arg scenario 'resilience:node-loss' --arg injection 'ec2-terminate-instances' \
		--arg deletedNode "$node_name" --arg deletedInstance "$instance_id" --arg deletedZone "$zone" \
		--arg replacementNode "$replacement_node" --arg replacementInstance "$replacement_instance" \
		--arg oldInstanceState "$old_instance_state" --argjson readyBefore "$ready_before" --argjson readyAfter "$ready_after" \
		--argjson zonesBefore "$ready_zones_before" --argjson zonesAfter "$ready_zones_after" --argjson measuredRTOSeconds "$measured_rto" \
		'{scenario:$scenario,injection:$injection,injectionVerified:true,deletedNode:$deletedNode,deletedInstance:$deletedInstance,deletedZone:$deletedZone,replacementNode:$replacementNode,replacementInstance:$replacementInstance,oldInstanceState:$oldInstanceState,readyNodesBefore:$readyBefore,readyNodesAfter:$readyAfter,readyZonesBefore:$zonesBefore,readyZonesAfter:$zonesAfter,traffic:"runtime-health-verified",measuredRTOSeconds:$measuredRTOSeconds}' \
		>"$LIVE_WORKDIR/cell-node-loss-proof.json"
	printf 'resilience:node-loss PASS deletedNode=%s deletedInstance=%s replacementNode=%s replacementInstance=%s ready=%s zones=%s measuredRTOSeconds=%s traffic=runtime-health-verified\n' \
		"$node_name" "$instance_id" "$replacement_node" "$replacement_instance" "$ready_after" "$ready_zones_after" "$measured_rto"
}

# Inject an exact declared-zone loss simulation into a self-managed EKS ASG.
# Every Ready node in one selected zone is identity-checked before its backing
# EC2 instance is terminated. The cell requires an observed zero-Ready gap in
# that zone, then proves distinct ASG/EC2/Kubernetes replacements restore the
# original Ready count and declared-zone count. This is a bounded provider-API
# simulation, not a claim that AWS physically made the Availability Zone fail.
run_aws_eks_zone_loss_cell() {
	local timeout_secs poll_interval elapsed fault_started measured_rto
	local outputs_file stack_ref cluster_name kc_path nodes_json ready_before ready_after
	local ready_zones_before ready_zones_after before_node_names target_zone target_records target_count
	local target_record target_node_name target_provider_id target_label_zone target_provider_zone
	local target_instance target_instance_json target_instance_state target_instance_zone target_asg provider_extra
	local asg_name asg_json asg_min asg_desired asg_max old_ids_json
	local replacement_instance replacement_node replacement_instance_json replacement_state
	local target_before_json='[]' injected_targets='[]' target_after_json='[]'
	local target_zone_ready_after all_replaced zone_degraded_observed_json=false
	local used_replacement_ids_json
	if [[ "$target_runtime" != "eks" || "$(configured_compute_mode)" != "self-managed" ]]; then
		printf 'resilience:zone-loss requires EKS computeMode:self-managed\n' >&2
		return 2
	fi
	if ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'resilience:zone-loss requires kubectl and jq on PATH\n' >&2
		return 2
	fi
	timeout_secs="${MAGELIFT_AWS_ZONE_LOSS_TIMEOUT_SECS:-1200}"
	poll_interval="${MAGELIFT_AWS_ZONE_LOSS_POLL_INTERVAL_SECS:-5}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:zone-loss timeout and poll interval must be positive integers\n' >&2
		return 2
	fi

	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if ! run outputs >"$outputs_file"; then
		printf 'resilience:zone-loss could not read stack outputs\n' >&2
		return 1
	fi
	stack_ref="$(jq -r '.stack // empty' "$outputs_file")"
	cluster_name="$(jq -r '.outputs.clusterName // empty' "$outputs_file")"
	if [[ -z "$stack_ref" || -z "$cluster_name" ]]; then
		printf 'resilience:zone-loss outputs lack stack or cluster name\n' >&2
		return 1
	fi
	stack_ref="$(qualify_aws_pulumi_stack_ref "$stack_ref")"
	kc_path="$LIVE_WORKDIR/zone-loss.kubeconfig"
	if ! PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" \
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi stack output kubeconfig --stack "$stack_ref" --show-secrets >"$kc_path"; then
		printf 'resilience:zone-loss could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"

	if ! nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json)"; then
		printf 'resilience:zone-loss could not list EKS nodes\n' >&2
		return 1
	fi
	printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/cell-zone-loss-nodes-before.json"
	ready_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length')"
	ready_zones_before="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length')"
	before_node_names="$(printf '%s' "$nodes_json" | jq -c '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name] | sort')"
	if [[ ! "$ready_before" =~ ^[0-9]+$ || "$ready_before" -lt 2 || ! "$ready_zones_before" =~ ^[0-9]+$ || "$ready_zones_before" -lt 2 ]]; then
		printf 'resilience:zone-loss requires at least two Ready self-managed nodes across two zones (ready=%s zones=%s)\n' "$ready_before" "$ready_zones_before" >&2
		return 1
	fi
	target_zone="$(printf '%s' "$nodes_json" | jq -r '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | map(select(length > 0)) | unique | sort | .[0]')"
	if [[ -z "$target_zone" || "$target_zone" == null ]]; then
		printf 'resilience:zone-loss could not select a declared Ready zone\n' >&2
		return 1
	fi
	target_records="$(printf '%s' "$nodes_json" | jq -c --arg zone "$target_zone" '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | select(.metadata.labels["topology.kubernetes.io/zone"] == $zone) | {nodeName:.metadata.name,providerID:.spec.providerID,zone:.metadata.labels["topology.kubernetes.io/zone"]}] | sort_by(.nodeName)')"
	target_count="$(printf '%s' "$target_records" | jq 'length')"
	if [[ ! "$target_count" =~ ^[1-9][0-9]*$ ]]; then
		printf 'resilience:zone-loss selected zone has no Ready nodes (zone=%s)\n' "$target_zone" >&2
		return 1
	fi

	asg_name=""
	while IFS= read -r target_record; do
		target_node_name="$(printf '%s' "$target_record" | jq -r '.nodeName')"
		target_provider_id="$(printf '%s' "$target_record" | jq -r '.providerID')"
		target_label_zone="$(printf '%s' "$target_record" | jq -r '.zone')"
		if [[ -z "$target_node_name" || -z "$target_provider_id" || -z "$target_label_zone" || "$target_provider_id" != aws:///* ]]; then
			printf 'resilience:zone-loss selected node has no supported AWS provider identity (node=%s)\n' "$target_node_name" >&2
			return 1
		fi
		provider_path="${target_provider_id#aws:///}"
		IFS=/ read -r target_provider_zone target_instance provider_extra <<<"$provider_path"
		if [[ -n "${provider_extra:-}" || -z "$target_provider_zone" || -z "$target_instance" || "$target_provider_zone" != "$target_label_zone" || "$target_provider_zone" != "$target_zone" || ! "$target_instance" =~ ^i-[0-9a-f]{8,}$ ]]; then
			printf 'resilience:zone-loss selected provider identity does not match its declared zone (node=%s providerID=%s)\n' "$target_node_name" "$target_provider_id" >&2
			return 1
		fi
		if ! target_instance_json="$(aws ec2 describe-instances --region "$region" --instance-ids "$target_instance" --output json)"; then
			printf 'resilience:zone-loss could not verify exact backing EC2 instance=%s\n' "$target_instance" >&2
			return 1
		fi
		target_instance_state="$(printf '%s' "$target_instance_json" | jq -r '.Reservations[0].Instances[0].State.Name // empty')"
		target_instance_zone="$(printf '%s' "$target_instance_json" | jq -r '.Reservations[0].Instances[0].Placement.AvailabilityZone // empty')"
		if [[ "$target_instance_state" != running || "$target_instance_zone" != "$target_zone" ]]; then
			printf 'resilience:zone-loss backing instance is not an exact running instance in selected zone (instance=%s state=%s zone=%s)\n' "$target_instance" "$target_instance_state" "$target_instance_zone" >&2
			return 1
		fi
		if ! printf '%s' "$target_instance_json" | jq -e --arg cluster "$cluster_name" '.Reservations[0].Instances[0].Tags // [] | any(.[]; .Key == "aws:eks:cluster-name" and .Value == $cluster)' >/dev/null; then
			printf 'resilience:zone-loss backing instance is not tagged for cluster=%s\n' "$cluster_name" >&2
			return 1
		fi
		target_asg="$(aws autoscaling describe-auto-scaling-instances --region "$region" --instance-ids "$target_instance" --query 'AutoScalingInstances[0].AutoScalingGroupName' --output text)"
		if [[ -z "$target_asg" || "$target_asg" == None || "$target_asg" != "$cluster_name"*self-managed* ]]; then
			printf 'resilience:zone-loss backing instance is not in the self-managed ASG: %s\n' "$target_asg" >&2
			return 1
		fi
		if [[ -z "$asg_name" ]]; then
			asg_name="$target_asg"
		elif [[ "$asg_name" != "$target_asg" ]]; then
			printf 'resilience:zone-loss selected zone spans multiple ASGs (%s and %s)\n' "$asg_name" "$target_asg" >&2
			return 1
		fi
		target_before_json="$(jq -cn --argjson targets "$target_before_json" --argjson target "$target_record" --arg instanceIDBefore "$target_instance" --arg providerZone "$target_provider_zone" --arg asg "$target_asg" '$targets + [($target + {instanceIDBefore:$instanceIDBefore,providerZone:$providerZone,asg:$asg})]')"
	done < <(printf '%s' "$target_records" | jq -c '.[]')
	printf '%s\n' "$target_before_json" >"$LIVE_WORKDIR/cell-zone-loss-targets-before.json"

	if ! asg_json="$(aws autoscaling describe-auto-scaling-groups --region "$region" --auto-scaling-group-names "$asg_name" --output json)"; then
		printf 'resilience:zone-loss could not read ASG=%s\n' "$asg_name" >&2
		return 1
	fi
	asg_min="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].MinSize // 0')"
	asg_desired="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].DesiredCapacity // 0')"
	asg_max="$(printf '%s' "$asg_json" | jq -r '.AutoScalingGroups[0].MaxSize // 0')"
	if [[ ! "$asg_min" =~ ^[0-9]+$ || ! "$asg_desired" =~ ^[0-9]+$ || ! "$asg_max" =~ ^[0-9]+$ || "$asg_min" -lt 2 || "$asg_desired" -lt 2 || "$asg_max" -lt "$asg_desired" ]]; then
		printf 'resilience:zone-loss requires ASG min/desired/max >= 2/%s/%s (observed %s/%s/%s)\n' "$asg_desired" "$asg_max" "$asg_min" "$asg_desired" "$asg_max" >&2
		return 1
	fi
	old_ids_json="$(printf '%s' "$target_before_json" | jq -c '[.[].instanceIDBefore]')"
	while IFS= read -r target_record; do
		target_instance="$(printf '%s' "$target_record" | jq -r '.instanceIDBefore')"
		if ! printf '%s' "$asg_json" | jq -e --arg instance "$target_instance" '[.AutoScalingGroups[0].Instances[]? | select(.InstanceId == $instance and .LifecycleState == "InService" and .HealthStatus == "Healthy")] | length == 1' >/dev/null; then
			printf 'resilience:zone-loss selected instance is not a healthy InService ASG member: %s\n' "$target_instance" >&2
			return 1
		fi
	done < <(printf '%s' "$target_before_json" | jq -c '.[]')

	jq -n --arg scenario 'resilience:zone-loss' --arg selectedZone "$target_zone" --arg asg "$asg_name" --argjson targets "$target_before_json" --argjson readyNodes "$ready_before" --argjson readyZones "$ready_zones_before" '{scenario:$scenario,selectedZone:$selectedZone,asg:$asg,injection:"ec2-terminate-instances-all-ready-nodes-in-zone",state:"prepared",targets:$targets,readyNodes:$readyNodes,readyZones:$readyZones}' >"$LIVE_WORKDIR/cell-zone-loss-injection.json"
	fault_started="$(date +%s)"
	while IFS= read -r target_record; do
		target_instance="$(printf '%s' "$target_record" | jq -r '.instanceIDBefore')"
		target_node_name="$(printf '%s' "$target_record" | jq -r '.nodeName')"
		printf '+ resilience:zone-loss terminating exact EC2 instance=%s node=%s zone=%s asg=%s\n' "$target_instance" "$target_node_name" "$target_zone" "$asg_name" >&2
		if ! aws ec2 terminate-instances --region "$region" --instance-ids "$target_instance" --output json >"$LIVE_WORKDIR/cell-zone-loss-termination-${target_instance}.json"; then
			printf 'resilience:zone-loss exact EC2 termination failed: %s\n' "$target_instance" >&2
			return 1
		fi
		injected_targets="$(jq -cn --argjson targets "$injected_targets" --argjson target "$target_record" '$targets + [$target]')"
		jq -n --arg scenario 'resilience:zone-loss' --arg selectedZone "$target_zone" --arg asg "$asg_name" --argjson targets "$target_before_json" --argjson injectedTargets "$injected_targets" '{scenario:$scenario,selectedZone:$selectedZone,asg:$asg,injection:"ec2-terminate-instances-all-ready-nodes-in-zone",state:"injected",targets:$targets,injectedTargets:$injectedTargets}' >"$LIVE_WORKDIR/cell-zone-loss-injection.json"
	done < <(printf '%s' "$target_before_json" | jq -c '.[]')

	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		asg_json="$(aws autoscaling describe-auto-scaling-groups --region "$region" --auto-scaling-group-names "$asg_name" --output json 2>/dev/null || printf '{"AutoScalingGroups":[]}')"
		nodes_json="$(kubectl --kubeconfig="$kc_path" get nodes -o json 2>/dev/null || printf '{"items":[]}')"
		ready_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length' 2>/dev/null || printf '0')"
		ready_zones_after="$(printf '%s' "$nodes_json" | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.labels["topology.kubernetes.io/zone"] // empty] | unique | length' 2>/dev/null || printf '0')"
		target_zone_ready_after="$(printf '%s' "$nodes_json" | jq --arg zone "$target_zone" '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | select(.metadata.labels["topology.kubernetes.io/zone"] == $zone)] | length' 2>/dev/null || printf '0')"
		if [[ "$target_zone_ready_after" == 0 ]]; then
			zone_degraded_observed_json=true
		fi
		all_replaced=1
		target_after_json='[]'
		used_replacement_ids_json='[]'
		while IFS= read -r target_record; do
			target_instance="$(printf '%s' "$target_record" | jq -r '.instanceIDBefore')"
			target_node_name="$(printf '%s' "$target_record" | jq -r '.nodeName')"
			target_provider_id="$(printf '%s' "$target_record" | jq -r '.providerID')"
			target_provider_zone="$(printf '%s' "$target_record" | jq -r '.providerZone')"
			replacement_instance="$(printf '%s' "$asg_json" | jq -r --argjson old "$old_ids_json" --argjson used "$used_replacement_ids_json" --arg zone "$target_provider_zone" '.AutoScalingGroups[0].Instances[]? | select(.AvailabilityZone == $zone and .LifecycleState == "InService" and .HealthStatus == "Healthy") | select((.InstanceId as $id | any($old[]; . == $id) | not) and (.InstanceId as $id | any($used[]; . == $id) | not)) | .InstanceId' | head -n 1)"
			replacement_node=""
			replacement_state=""
			replacement_instance_json='{}'
			if [[ -n "$replacement_instance" ]]; then
				replacement_instance_json="$(aws ec2 describe-instances --region "$region" --instance-ids "$replacement_instance" --output json 2>/dev/null || printf '{}')"
				replacement_state="$(printf '%s' "$replacement_instance_json" | jq -r '.Reservations[0].Instances[0].State.Name // empty')"
				if [[ "$replacement_state" == running ]]; then
					replacement_node="$(printf '%s' "$nodes_json" | jq -r --arg old "$target_node_name" --arg oldProvider "$target_provider_id" --arg replacement "$replacement_instance" --arg zone "$target_provider_zone" --argjson before "$before_node_names" '.items[] | select(.metadata.name != $old and .spec.providerID != $oldProvider) | select(.spec.providerID | endswith("/" + $replacement)) | select(.metadata.labels["topology.kubernetes.io/zone"] == $zone) | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | select(.metadata.name as $current | (any($before[]; . == $current) | not)) | .metadata.name' 2>/dev/null | head -n 1)"
				fi
			fi
			if [[ -z "$replacement_instance" || "$replacement_state" != running || -z "$replacement_node" ]]; then
				all_replaced=0
			else
				used_replacement_ids_json="$(jq -cn --argjson used "$used_replacement_ids_json" --arg id "$replacement_instance" '$used + [$id]')"
			fi
			target_after_json="$(jq -cn --argjson targets "$target_after_json" --argjson target "$target_record" --arg instanceIDAfter "$replacement_instance" --arg nodeNameAfter "$replacement_node" --arg stateAfter "$replacement_state" '$targets + [($target + {instanceIDAfter:$instanceIDAfter,nodeNameAfter:$nodeNameAfter,stateAfter:$stateAfter})]')"
		done < <(printf '%s' "$target_before_json" | jq -c '.[]')
		if [[ "$ready_after" =~ ^[0-9]+$ && "$ready_after" -ge "$ready_before" && "$ready_zones_after" =~ ^[0-9]+$ && "$ready_zones_after" -ge "$ready_zones_before" && "$target_zone_ready_after" =~ ^[0-9]+$ && "$target_zone_ready_after" -ge "$target_count" && "$all_replaced" == 1 && "$zone_degraded_observed_json" == true ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/failure-zone-loss-nodes.json"
			printf '%s\n' "$target_after_json" >"$LIVE_WORKDIR/failure-zone-loss-targets.json"
			printf 'resilience:zone-loss timed out selectedZone=%s targets=%s ready=%s zones=%s targetZoneReady=%s allReplaced=%s zoneGapObserved=%s\n' "$target_zone" "$target_count" "$ready_after" "$ready_zones_after" "$target_zone_ready_after" "$all_replaced" "$zone_degraded_observed_json" >&2
			return 1
		fi
		sleep "$poll_interval"
	done

	printf '%s\n' "$nodes_json" >"$LIVE_WORKDIR/cell-zone-loss-nodes-after.json"
	printf '%s\n' "$target_after_json" >"$LIVE_WORKDIR/cell-zone-loss-targets-after.json"
	if ! run_runtime_health_with_retry; then
		printf 'resilience:zone-loss runtime health failed after selected-zone recovery\n' >&2
		return 1
	fi
	measured_rto="$(( $(date +%s) - fault_started ))"
	jq -n --arg scenario 'resilience:zone-loss' --arg selectedZone "$target_zone" --arg asg "$asg_name" --arg injection 'ec2-terminate-instances-all-ready-nodes-in-zone' --argjson targetsBefore "$target_before_json" --argjson targetsAfter "$target_after_json" --argjson targetCount "$target_count" --argjson readyBefore "$ready_before" --argjson readyAfter "$ready_after" --argjson zonesBefore "$ready_zones_before" --argjson zonesAfter "$ready_zones_after" --argjson targetZoneReadyAfter "$target_zone_ready_after" --argjson zoneReadyGapObserved "$zone_degraded_observed_json" --argjson measuredRTOSeconds "$measured_rto" '{scenario:$scenario,injection:$injection,injectionVerified:true,selectedZone:$selectedZone,asg:$asg,zoneReadyGapObserved:$zoneReadyGapObserved,targetsBefore:$targetsBefore,targetsAfter:$targetsAfter,targetCount:$targetCount,readyNodesBefore:$readyBefore,readyNodesAfter:$readyAfter,readyZonesBefore:$zonesBefore,readyZonesAfter:$zonesAfter,targetZoneReadyAfter:$targetZoneReadyAfter,traffic:"runtime-health-verified",measuredRTOSeconds:$measuredRTOSeconds}' >"$LIVE_WORKDIR/cell-zone-loss-proof.json"
	printf 'resilience:zone-loss PASS deletedZone=%s deletedInstances=%s zoneReadyGapObserved=%s recoveredReady=%s/%s recoveredZones=%s/%s measuredRTOSeconds=%s traffic=runtime-health-verified\n' "$target_zone" "$(printf '%s' "$old_ids_json" | jq -r 'join(",")')" "$zone_degraded_observed_json" "$ready_after" "$ready_before" "$ready_zones_after" "$ready_zones_before" "$measured_rto"
}

# Verify the AWS-managed EKS CloudWatch path against the owning APIs. The
# add-on's Fluent Bit application log and Container Insights cluster metric
# are separate delivery assertions; the EKS control-plane audit stream is a
# third, provider-owned signal. This deliberately does not claim OTel custom
# configuration, alert firing, redaction policy, or SLO delivery.
run_aws_eks_cloudwatch_observability_cell() {
	local timeout_secs poll_interval elapsed started_at started_ms metric_start_time
	local outputs_file stack_ref cluster_name kc_path addon_json addon_status
	local daemonsets_json daemonset_contract daemonset_count audit_enabled
	local probe_name probe_image probe_message application_log_group performance_log_group
	local control_plane_log_group probe_started
	local app_events_json app_event_count metric_list_json metric_count metric_query
	local metric_data_json metric_value_count audit_streams_json audit_stream_count
	local audit_events_json audit_event_count measured_seconds
	if [[ "$target_runtime" != "eks" || "$(configured_compute_mode)" != "self-managed" ]]; then
		printf 'observability:cloudwatch requires EKS computeMode:self-managed\n' >&2
		return 2
	fi
	if ! command -v aws >/dev/null 2>&1 || ! command -v kubectl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		printf 'observability:cloudwatch requires aws, kubectl, and jq on PATH\n' >&2
		return 2
	fi
	timeout_secs="${MAGELIFT_AWS_EKS_OBSERVABILITY_TIMEOUT_SECS:-900}"
	poll_interval="${MAGELIFT_AWS_EKS_OBSERVABILITY_POLL_INTERVAL_SECS:-10}"
	if [[ ! "$timeout_secs" =~ ^[1-9][0-9]*$ || ! "$poll_interval" =~ ^[1-9][0-9]*$ ]]; then
		printf 'observability:cloudwatch timeout and poll interval must be positive integers\n' >&2
		return 2
	fi

	outputs_file="$LIVE_WORKDIR/infra-outputs.json"
	if ! run outputs >"$outputs_file"; then
		printf 'observability:cloudwatch could not read stack outputs\n' >&2
		return 1
	fi
	stack_ref="$(jq -r '.stack // empty' "$outputs_file")"
	cluster_name="$(jq -r '.outputs.clusterName // empty' "$outputs_file")"
	if [[ -z "$stack_ref" || -z "$cluster_name" ]]; then
		printf 'observability:cloudwatch outputs lack stack or cluster name\n' >&2
		return 1
	fi
	stack_ref="$(qualify_aws_pulumi_stack_ref "$stack_ref")"
	kc_path="$LIVE_WORKDIR/cloudwatch-observability.kubeconfig"
	if ! PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" \
		PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" \
		pulumi stack output kubeconfig --stack "$stack_ref" --show-secrets >"$kc_path"; then
		printf 'observability:cloudwatch could not read kubeconfig output\n' >&2
		return 1
	fi
	chmod 600 "$kc_path"

	if ! addon_json="$(aws eks describe-addon --region "$region" --cluster-name "$cluster_name" --addon-name amazon-cloudwatch-observability --output json)"; then
		printf 'observability:cloudwatch could not describe the EKS add-on\n' >&2
		return 1
	fi
	printf '%s\n' "$addon_json" >"$LIVE_WORKDIR/cell-cloudwatch-addon.json"
	addon_status="$(printf '%s' "$addon_json" | jq -r '.addon.status // empty')"
	if [[ "$addon_status" != ACTIVE ]]; then
		printf 'observability:cloudwatch EKS add-on is not ACTIVE: %s\n' "$addon_status" >&2
		return 1
	fi

	if ! daemonsets_json="$(kubectl --kubeconfig="$kc_path" get daemonsets -n amazon-cloudwatch -o json)"; then
		printf 'observability:cloudwatch could not list amazon-cloudwatch DaemonSets\n' >&2
		return 1
	fi
	daemonset_contract="$(printf '%s' "$daemonsets_json" | jq -c '[.items[] | select(.metadata.name | test("^(cloudwatch-agent|fluent-bit)$")) | {name:.metadata.name,desired:(.status.desiredNumberScheduled // 0),ready:(.status.numberReady // 0)}] | sort_by(.name)')"
	printf '%s\n' "$daemonset_contract" >"$LIVE_WORKDIR/cell-cloudwatch-daemonsets.json"
	daemonset_count="$(printf '%s' "$daemonset_contract" | jq 'length')"
	if [[ "$daemonset_count" -lt 2 ]] || ! printf '%s' "$daemonset_contract" | jq -e 'any(.[]; .name | test("cloudwatch-agent")) and any(.[]; .name | test("fluent-bit")) and all(.[]; .desired > 0 and .ready >= .desired)' >/dev/null; then
		printf 'observability:cloudwatch agent/Fluent Bit DaemonSets are not fully ready: %s\n' "$daemonset_contract" >&2
		return 1
	fi

	audit_enabled="$(aws eks describe-cluster --region "$region" --name "$cluster_name" --query 'cluster.logging.clusterLogging[?enabled==`true`].types[]' --output json | jq 'any(.[]; . == "audit")')"
	if [[ "$audit_enabled" != true ]]; then
		printf 'observability:cloudwatch EKS audit control-plane logging is not enabled\n' >&2
		return 1
	fi

	probe_name="$(printf '%s-cw-probe' "$project_tag" | tr '[:upper:]_' '[:lower:]-' | tr -cd 'a-z0-9-')"
	probe_name="${probe_name:0:55}"
	probe_image="${MAGELIFT_AWS_ACCEPTANCE_SEED_IMAGE:-public.ecr.aws/docker/library/mysql:8.4}"
	probe_message="MAGELIFT_EKS_CLOUDWATCH_PROBE_${project_tag}_$(date -u +%Y%m%d%H%M%S)"
	application_log_group="/aws/containerinsights/$cluster_name/application"
	performance_log_group="/aws/containerinsights/$cluster_name/performance"
	control_plane_log_group="/aws/eks/$cluster_name/cluster"
	started_at="$(date +%s)"
	started_ms="$((started_at * 1000))"
	metric_start_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	kubectl --kubeconfig="$kc_path" delete pod "$probe_name" --ignore-not-found --wait=true >/dev/null 2>&1 || true
	if ! kubectl --kubeconfig="$kc_path" run "$probe_name" --image="$probe_image" --restart=Never --command -- /bin/sh -c "printf '%s\\n' '$probe_message'; sleep 600"; then
		printf 'observability:cloudwatch could not start the marker pod\n' >&2
		return 1
	fi
	if ! kubectl --kubeconfig="$kc_path" wait --for=condition=Ready "pod/$probe_name" --timeout=180s; then
		kubectl --kubeconfig="$kc_path" delete pod "$probe_name" --ignore-not-found --wait=true >/dev/null 2>&1 || true
		printf 'observability:cloudwatch marker pod did not become Ready\n' >&2
		return 1
	fi
	probe_started="$(date +%s)"
	metric_query="$(jq -cn --arg cluster "$cluster_name" '[{Id:"cluster_nodes",MetricStat:{Metric:{Namespace:"ContainerInsights",MetricName:"cluster_node_count",Dimensions:[{Name:"ClusterName",Value:$cluster}]},Period:60,Stat:"Average"},ReturnData:true}]')"
	for ((elapsed = 0; elapsed <= timeout_secs; elapsed += poll_interval)); do
		app_events_json="$(aws logs filter-log-events --region "$region" --log-group-name "$application_log_group" --start-time "$started_ms" --filter-pattern "$probe_message" --output json 2>/dev/null || printf '{"events":[]}')"
		app_event_count="$(printf '%s' "$app_events_json" | jq --arg marker "$probe_message" '[.events[]? | select((.message // "") | contains($marker))] | length' 2>/dev/null || printf '0')"
		metric_list_json="$(aws cloudwatch list-metrics --region "$region" --namespace ContainerInsights --metric-name cluster_node_count --dimensions "Name=ClusterName,Value=$cluster_name" --output json 2>/dev/null || printf '{"Metrics":[]}')"
		metric_count="$(printf '%s' "$metric_list_json" | jq '[.Metrics[]?] | length' 2>/dev/null || printf '0')"
		metric_data_json="$(aws cloudwatch get-metric-data --region "$region" --metric-data-queries "$metric_query" --start-time "$metric_start_time" --end-time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --output json 2>/dev/null || printf '{"MetricDataResults":[]}')"
		metric_value_count="$(printf '%s' "$metric_data_json" | jq '[.MetricDataResults[]?.Values[]?] | length' 2>/dev/null || printf '0')"
		audit_streams_json="$(aws logs describe-log-streams --region "$region" --log-group-name "$control_plane_log_group" --log-stream-name-prefix kube-apiserver-audit --output json 2>/dev/null || printf '{"logStreams":[]}')"
		audit_stream_count="$(printf '%s' "$audit_streams_json" | jq '[.logStreams[]?] | length' 2>/dev/null || printf '0')"
		audit_events_json="$(aws logs filter-log-events --region "$region" --log-group-name "$control_plane_log_group" --start-time "$started_ms" --output json 2>/dev/null || printf '{"events":[]}')"
		audit_event_count="$(printf '%s' "$audit_events_json" | jq '[.events[]?] | length' 2>/dev/null || printf '0')"
		# Generate an API audit event on every poll while the marker pod remains
		# alive; the exact audit stream is checked above rather than accepting only
		# the existence of the control-plane log group.
		kubectl --kubeconfig="$kc_path" get pod "$probe_name" -o name >/dev/null 2>&1 || true
		if [[ "$app_event_count" =~ ^[1-9][0-9]*$ && "$metric_count" =~ ^[1-9][0-9]*$ && "$metric_value_count" =~ ^[1-9][0-9]*$ && "$audit_stream_count" =~ ^[1-9][0-9]*$ && "$audit_event_count" =~ ^[1-9][0-9]*$ ]]; then
			break
		fi
		if (( elapsed >= timeout_secs )); then
			kubectl --kubeconfig="$kc_path" delete pod "$probe_name" --ignore-not-found --wait=true >/dev/null 2>&1 || true
			printf '%s\n' "$app_events_json" >"$LIVE_WORKDIR/failure-cloudwatch-application-events.json"
			printf '%s\n' "$metric_list_json" >"$LIVE_WORKDIR/failure-cloudwatch-metrics.json"
			printf '%s\n' "$audit_events_json" >"$LIVE_WORKDIR/failure-cloudwatch-audit-events.json"
			printf 'observability:cloudwatch timed out appEvents=%s metrics=%s metricValues=%s auditStreams=%s auditEvents=%s\n' "$app_event_count" "$metric_count" "$metric_value_count" "$audit_stream_count" "$audit_event_count" >&2
			return 1
		fi
		sleep "$poll_interval"
	done

	kubectl --kubeconfig="$kc_path" logs "$probe_name" >"$LIVE_WORKDIR/cell-cloudwatch-marker-pod.log"
	kubectl --kubeconfig="$kc_path" delete pod "$probe_name" --ignore-not-found --wait=true >/dev/null 2>&1 || true
	measured_seconds="$(( $(date +%s) - probe_started ))"
	printf '%s\n' "$app_events_json" >"$LIVE_WORKDIR/cell-cloudwatch-application-events.json"
	printf '%s\n' "$metric_list_json" >"$LIVE_WORKDIR/cell-cloudwatch-metrics.json"
	printf '%s\n' "$metric_data_json" >"$LIVE_WORKDIR/cell-cloudwatch-metric-data.json"
	printf '%s\n' "$audit_streams_json" >"$LIVE_WORKDIR/cell-cloudwatch-audit-streams.json"
	printf '%s\n' "$audit_events_json" >"$LIVE_WORKDIR/cell-cloudwatch-audit-events.json"
	jqsafe="$(jq -n --arg scenario 'observability:cloudwatch' --arg addonStatus "$addon_status" --arg applicationLogGroup "$application_log_group" --arg performanceLogGroup "$performance_log_group" --arg controlPlaneLogGroup "$control_plane_log_group" --arg probeName "$probe_name" --arg probeMessage "$probe_message" --argjson daemonsets "$daemonset_contract" --argjson applicationEvents "$app_event_count" --argjson metrics "$metric_count" --argjson metricValues "$metric_value_count" --argjson auditStreams "$audit_stream_count" --argjson auditEvents "$audit_event_count" --argjson measuredSeconds "$measured_seconds" '{scenario:$scenario,addon:"amazon-cloudwatch-observability",addonStatus:$addonStatus,daemonsets:$daemonsets,containerLogProbe:{podName:$probeName,message:$probeMessage,applicationLogGroup:$applicationLogGroup,events:$applicationEvents},containerInsightsMetric:{namespace:"ContainerInsights",metricName:"cluster_node_count",metricCount:$metrics,dataPoints:$metricValues},controlPlaneAudit:{logGroup:$controlPlaneLogGroup,auditStreams:$auditStreams,events:$auditEvents},performanceLogGroup:$performanceLogGroup,redaction:"not-claimed",alertDelivery:"not-claimed",sloDelivery:"not-claimed",measuredDeliverySeconds:$measuredSeconds}')"
	printf '%s\n' "$jqsafe" >"$LIVE_WORKDIR/cell-cloudwatch-observability-proof.json"
	printf 'observability:cloudwatch PASS addon=%s appLogEvents=%s containerInsightsMetrics=%s metricValues=%s auditStreams=%s auditEvents=%s measuredDeliverySeconds=%s\n' "$addon_status" "$app_event_count" "$metric_count" "$metric_value_count" "$audit_stream_count" "$audit_event_count" "$measured_seconds"
}

wait_for_clean() {
	local timeout="${MAGELIFT_AWS_ACCEPTANCE_CLEANUP_TIMEOUT_SECS:-900}"
	local interval="${MAGELIFT_AWS_ACCEPTANCE_CLEANUP_INTERVAL_SECS:-15}"
	local started now
	started=$(date +%s)
	while ! assert_clean; do
		now=$(date +%s)
		if (( now - started >= timeout )); then
			printf 'AWS cleanup polling timed out after %ss\n' "$timeout" >&2
			return 1
		fi
		printf 'AWS resources are still deleting; retrying cleanup scan in %ss\n' "$interval" >&2
		sleep "$interval"
	done
}

cleanup_acceptance_log_groups() {
	local log_group
	while IFS= read -r log_group; do
		if [[ -n "$log_group" && "$log_group" != None ]]; then
			printf '+ deleting AWS acceptance log group %s\n' "$log_group" >&2
			aws logs delete-log-group --region "$region" --log-group-name "$log_group"
		fi
	done < <(aws logs describe-log-groups --region "$region" --query "logGroups[?contains(logGroupName, '$project_tag')]" --output json | jq -r '.[].logGroupName')
}

cleanup_acceptance_cache_subnet_groups() {
	local subnet_group
	while IFS= read -r subnet_group; do
		if [[ -n "$subnet_group" && "$subnet_group" != None ]]; then
			printf '+ deleting AWS acceptance ElastiCache subnet group %s\n' "$subnet_group" >&2
			aws elasticache delete-cache-subnet-group --region "$region" --cache-subnet-group-name "$subnet_group"
		fi
	done < <(aws elasticache describe-cache-subnet-groups --region "$region" --query "(CacheSubnetGroups || \`[]\`)[?contains(CacheSubnetGroupName, '$project_tag')].CacheSubnetGroupName" --output text | tr '\t' '\n')
}

cleanup_acceptance_prerequisite_secrets() {
	if [[ "${MAGELIFT_AWS_ACCEPTANCE_DELETE_PREREQUISITE_SECRETS:-false}" != true ]]; then
		return 0
	fi
	if [[ -z "${MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS:-}" ]]; then
		printf 'refusing prerequisite-secret cleanup without exact ARN allowlist\n' >&2
		return 1
	fi

	local secret_arn
	while IFS= read -r secret_arn; do
		secret_arn="${secret_arn#"${secret_arn%%[![:space:]]*}"}"
		secret_arn="${secret_arn%"${secret_arn##*[![:space:]]}"}"
		[[ -z "$secret_arn" ]] && continue
		if ! acceptance_secret_arn_is_predeclared "$secret_arn" || ! acceptance_secret_has_run_tags "$secret_arn" "$region" "$project_tag" "$MAGELIFT_ACCEPTANCE_OWNER"; then
			printf 'refusing to delete unverified prerequisite secret %s\n' "$secret_arn" >&2
			return 1
		fi
		printf '+ force deleting AWS acceptance prerequisite secret %s\n' "$secret_arn" >&2
		if ! aws secretsmanager delete-secret --region "$region" --secret-id "$secret_arn" --force-delete-without-recovery >/dev/null; then
			printf 'failed to delete AWS acceptance prerequisite secret %s\n' "$secret_arn" >&2
			return 1
		fi
	done < <(printf '%s\n' "$MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS" | tr ',' '\n')
}

created=0
cleanup() {
	local status=$?
	acceptance_stop_ttl_watchdog || true
	local cleanup_result=PASS
	local cleanup_recorded=0
	if acceptance_ttl_expired; then
		export MAGELIFT_ACCEPTANCE_CLEANUP_REASON="acceptance TTL expired; forced cleanup"
		MAGELIFT_AWS_ACCEPTANCE_KEEP=false
	fi
	if [[ "${MAGELIFT_AWS_ACCEPTANCE_KEEP:-false}" == true ]]; then
		if [[ "$created" == 1 || "$state_bucket_owned" == 1 ]]; then
			MAGELIFT_ACCEPTANCE_CLEANUP_REASON="stack retention was explicitly requested" append_shared_cleanup "SKIP" "$profile" "magelift-run=${ACCEPTANCE_EVIDENCE_RUN_ID}" ""
		fi
		printf '+ retaining AWS acceptance workdir %s for resume\n' "$LIVE_WORKDIR" >&2
		exit "$status"
	elif [[ "$created" == 1 ]]; then
		printf '+ magelift destroy --yes (EXIT trap)\n' >&2
		# Acceptance cleanup is already serialized by this process and must remain
		# able to run when a partially-created stack cannot acquire the normal
		# provider lock (for example, after its disposable lock bucket vanished).
		if ! "${config[@]}" destroy --yes --skip-lock; then
			printf 'acceptance cleanup failed for %s; inspect with aws-cli and destroy manually\n' "$profile" >&2
			cleanup_result=FAIL
		fi
		# Remove the disposable Pulumi backend before the final resource scan. The
		# bucket is deliberately tagged with this run's ownership marker, so
		# asserting tagged-resource emptiness before deleting it can never pass.
		if [[ "$state_bucket_owned" == 1 ]]; then
			printf '+ deleting AWS Pulumi state bucket %s (EXIT trap)\n' "$state_bucket" >&2
			if ! cleanup_state_bucket; then
				cleanup_result=FAIL
				status=1
			fi
		fi
		if ! cleanup_acceptance_log_groups; then
			cleanup_result=FAIL
			status=1
		fi
		if ! cleanup_acceptance_cache_subnet_groups; then
			cleanup_result=FAIL
			status=1
		fi
		if ! cleanup_acceptance_prerequisite_secrets; then
			cleanup_result=FAIL
			status=1
		fi
		if ! wait_for_clean; then
			cleanup_result=FAIL
			status=1
		fi
		cleanup_recorded=1
	fi
	if [[ "$state_bucket_owned" == 1 ]]; then
		printf '+ deleting AWS Pulumi state bucket %s (EXIT trap)\n' "$state_bucket" >&2
		if ! cleanup_state_bucket; then
			cleanup_result=FAIL
			status=1
		fi
		cleanup_recorded=1
	fi
	if [[ "$cleanup_recorded" == 1 ]]; then
		append_shared_cleanup "$cleanup_result" "$profile" "magelift-run=${ACCEPTANCE_EVIDENCE_RUN_ID}" ""
	fi
	cleanup_workdir
	exit "$status"
}
trap cleanup EXIT

cleanup_state_bucket() {
	local payload object_count
	if [[ -z "$state_bucket" || "$state_bucket_owned" != 1 ]]; then
		return 0
	fi
	while true; do
		# S3 accepts at most 1,000 objects per DeleteObjects request. The
		# Pulumi history can exceed that even for a short-lived acceptance run.
		if ! payload=$(aws s3api list-object-versions --bucket "$state_bucket" --region "$region" --output json 2>/dev/null | jq -c '{Objects: ([.Versions[]?, .DeleteMarkers[]?] | map({Key,VersionId}) | .[:1000]), Quiet: true}'); then
			if ! aws s3api head-bucket --bucket "$state_bucket" --region "$region" >/dev/null 2>&1; then
				state_bucket_owned=0
				return 0
			fi
			return 1
		fi
		object_count=$(jq -r '.Objects | length' <<<"$payload")
		if [[ "$object_count" == 0 ]]; then
			break
		fi
		aws s3api delete-objects --bucket "$state_bucket" --region "$region" --delete "$payload" >/dev/null
	done
	aws s3api delete-bucket --bucket "$state_bucket" --region "$region"
	state_bucket_owned=0
}

# --- live helpers ----------------------------------------------------------------

# Resume only when the caller explicitly opts into the retained-stack path.
should_skip_create_once() {
	local resume="${MAGELIFT_AWS_ACCEPTANCE_RESUME:-0}"
	if [[ "$resume" == "1" || "$resume" == "true" ]]; then
		return 0
	fi
	return 1
}

# A retained stack is intentionally not clean. Resume therefore needs a
# narrower ownership check than a fresh-run cleanup assertion, but it must not
# turn an arbitrary project-shaped stack into an accepted resume target.
assert_resume_scope() {
	local stack_ref export_json owned_count foreign_count tagged_json tag_region
	if [[ "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-false}" != true && "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-0}" != 1 ]]; then
		printf 'assert_resume_scope called without MAGELIFT_AWS_ACCEPTANCE_RESUME=true\n' >&2
		return 1
	fi
	acceptance_checkpoint_ensure
	if [[ -z "${ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}" || ! -s "$ACCEPTANCE_CHECKPOINT" ]]; then
		printf 'AWS acceptance resume requires a checkpoint with a configuration fingerprint\n' >&2
		return 1
	fi
	if ! jq -e --arg fingerprint "$ACCEPTANCE_CHECKPOINT_FINGERPRINT" --arg stack "$MAGELIFT_ACCEPTANCE_STACK_ID" --arg digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" '
		.fingerprint == $fingerprint and (
			(.cells | type == "object" and (.cells | length) == 0) or
			([.cells[]? | select(.stackId == $stack and .artifactDigest == $digest)] | length) > 0
		)
	' "$ACCEPTANCE_CHECKPOINT" >/dev/null 2>&1; then
		printf 'AWS acceptance resume checkpoint does not match the configured stack, digest, and fingerprint\n' >&2
		return 1
	fi
	if [[ -z "$state_bucket" ]] || ! acceptance_state_bucket_has_run_tags "$state_bucket" "$region" "$project_tag" "$MAGELIFT_ACCEPTANCE_OWNER"; then
		printf 'AWS acceptance resume requires the exact owned Pulumi state bucket\n' >&2
		return 1
	fi
	stack_ref="$(qualify_aws_pulumi_stack_ref "$MAGELIFT_ACCEPTANCE_STACK_ID")"
	if ! export_json="$(PULUMI_BACKEND_URL="$PULUMI_BACKEND_URL" PULUMI_CONFIG_PASSPHRASE_FILE="${PULUMI_CONFIG_PASSPHRASE_FILE:-}" pulumi stack export --stack "$stack_ref" 2>/dev/null)" || \
		! jq -e '[.deployment.resources[]? | select(.type != "pulumi:pulumi:Stack")] | length > 0' <<<"$export_json" >/dev/null 2>&1; then
		printf 'AWS acceptance resume requires a non-empty Pulumi stack owned by this run: %s\n' "$stack_ref" >&2
		return 1
	fi
	owned_count=0
	for tag_region in "$region" us-east-1; do
		if ! tagged_json="$(aws resourcegroupstaggingapi get-resources --region "$tag_region" --tag-filters "Key=magelift:project,Values=$project_tag" --output json)"; then
			printf 'AWS acceptance resume could not inspect tagged resources in %s\n' "$tag_region" >&2
			return 1
		fi
		if ! foreign_count="$(jq --arg owner "$MAGELIFT_ACCEPTANCE_OWNER" '[.ResourceTagMappingList[]? | select(any(.Tags[]?; .Key == "magelift:acceptance-run" and .Value != $owner))] | length' <<<"$tagged_json")" || [[ "$foreign_count" != 0 ]]; then
			printf 'AWS acceptance resume found same-project resources outside ownership marker in %s\n' "$tag_region" >&2
			return 1
		fi
		owned_count=$((owned_count + $(jq --arg owner "$MAGELIFT_ACCEPTANCE_OWNER" '[.ResourceTagMappingList[]? | select(any(.Tags[]?; .Key == "magelift:acceptance-run" and .Value == $owner))] | length' <<<"$tagged_json")))
	done
	if [[ "$owned_count" == 0 ]]; then
		printf 'AWS acceptance resume found no tagged resources for ownership marker %s\n' "$MAGELIFT_ACCEPTANCE_OWNER" >&2
		return 1
	fi
	printf 'AWS acceptance resume scope verified stack=%s ownedTaggedResources=%s\n' "$stack_ref" "$owned_count" >&2
}

# Cell IDs are catalogKey:value (for example queueMode:ecs-rabbitmq). No CLI
# mutate API is needed; patch the target (+ environment) catalog in the temp
# YAML copy, then redeploy the same digest.
apply_cell_config() {
	local cell="${1:?cell id required}"
	local key value catalog_path
	if [[ "$cell" != *:* ]]; then
		printf 'invalid cell id (want key:value): %s\n' "$cell" >&2
		return 2
	fi
	key="${cell%%:*}"
	value="${cell#*:}"
	case "$key" in
	computeMode)
		if [[ "$target_runtime" == "eks" ]]; then
			case "$value" in
			auto-mode|managed-node-groups|self-managed|fargate) ;;
			*) printf 'unsupported EKS computeMode cell value: %s\n' "$value" >&2; return 2 ;;
			esac
		else
			case "$value" in
			fargate|fargate-spot|ec2-asg|managed-instances) ;;
			*) printf 'unsupported ECS computeMode cell value: %s\n' "$value" >&2; return 2 ;;
			esac
		fi
		if [[ "$(configured_compute_mode)" != "$value" ]]; then
			printf 'refusing architecture cell %s: selected YAML computeMode is %s; architecture cells require a separate cold run\n' \
				"$cell" "$(configured_compute_mode)" >&2
			return 2
		fi
		printf 'acceptance architecture cell %s matches YAML; no catalog patch\n' "$cell" >&2
		return 0
		;;
	queueMode)
		if [[ "$target_runtime" == "eks" ]]; then
			case "$value" in database|rabbitmq) ;; *) printf 'unsupported EKS queueMode cell value: %s\n' "$value" >&2; return 2 ;; esac
		else
			case "$value" in
			db|ecs-rabbitmq|ecs-artemis) ;;
			amazon-mq)
				if [[ "${MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY:-}" != true ]]; then
					printf 'refusing cell %s (amazon-mq excluded from free-tier harness; set MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true)\n' "$cell" >&2
					return 2
				fi
				;;
			*)
				printf 'unsupported queueMode cell value: %s\n' "$value" >&2
				return 2
				;;
			esac
		fi
		;;
	searchMode)
		if [[ "$target_runtime" == "eks" ]]; then
			case "$value" in opensearch|disabled) ;; *) printf 'unsupported EKS searchMode cell value: %s\n' "$value" >&2; return 2 ;; esac
		else
			case "$value" in
			disabled|serverless|provisioned) ;;
			*) printf 'unsupported ECS searchMode cell value: %s\n' "$value" >&2; return 2 ;;
			esac
		fi
		;;
	databaseEngine)
		if [[ "$target_runtime" == "eks" ]]; then
			printf 'databaseEngine cells are ECS catalog cells; EKS uses the shared RDS/Aurora graph from YAML\n' >&2
			return 2
		fi
		case "$value" in
		rds-mysql|rds-mariadb|aurora-mysql) ;;
		*) printf 'unsupported databaseEngine cell value: %s\n' "$value" >&2; return 2 ;;
		esac
		;;
	ha)
		if [[ "$value" != multi-az ]]; then
			printf 'unsupported ha cell value: %s (want multi-az)\n' "$value" >&2
			return 2
		fi
		if [[ "$target_runtime" == "eks" ]]; then
			printf 'ha:multi-az is an ECS Fargate fingerprint; EKS HA uses MAGELIFT_AWS_ACCEPTANCE_PROFILE=high-availability\n' >&2
			return 2
		fi
		if ! command -v yq >/dev/null 2>&1; then
			printf 'yq is required to patch catalog cells into the temp config (brew install yq)\n' >&2
			return 2
		fi
		yq -i '.target.aws.natTopology = "multi-az" | .target.aws.natReplacementMode = "auto-scaling"' "$LIVE_CONFIG"
		yq -i '.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.natTopology = "multi-az" | .environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.natReplacementMode = "auto-scaling"' "$LIVE_CONFIG"
		printf 'acceptance patched ha=multi-az into temp config\n' >&2
		return 0
		;;
	resilience)
		if [[ "$value" != node-loss && "$value" != zone-loss ]]; then
			printf 'unsupported EKS resilience cell value: %s\n' "$value" >&2
			return 2
		fi
		if [[ "$target_runtime" != eks || "$(configured_compute_mode)" != self-managed ]]; then
			printf 'resilience:%s requires EKS computeMode:self-managed\n' "$value" >&2
			return 2
		fi
		printf 'acceptance validated %s against the retained self-managed stack\n' "$cell" >&2
		return 0
		;;
	observability)
		if [[ "$value" != cloudwatch ]]; then
			printf 'unsupported EKS observability cell value: %s\n' "$value" >&2
			return 2
		fi
		if [[ "$target_runtime" != eks || "$(configured_compute_mode)" != self-managed ]]; then
			printf 'observability:%s requires EKS computeMode:self-managed\n' "$value" >&2
			return 2
		fi
		printf 'acceptance validated %s against the retained self-managed stack\n' "$cell" >&2
		return 0
		;;
	*)
		printf 'unsupported cell key %s (harness supports computeMode, queueMode, searchMode, databaseEngine, ha, resilience, and observability)\n' "$key" >&2
		return 2
		;;
	esac

	if ! command -v yq >/dev/null 2>&1; then
		printf 'yq is required to patch catalog cells into the temp config (brew install yq)\n' >&2
		return 2
	fi

	# Prefer the environment-scoped catalog so inherited defaults do not shadow
	# the cell. EKS keeps its Kubernetes workload choices under catalog.eks;
	# ECS keeps its managed-service choices directly under catalog.
	if [[ "$target_runtime" == "eks" ]]; then
		catalog_path="eks.${key}"
	else
		catalog_path="$key"
	fi
	MAGELIFT_ACCEPTANCE_CELL_VALUE="$value" MAGELIFT_ACCEPTANCE_CATALOG_PATH="$catalog_path" \
		yq -i 'setpath(["target", "aws", "catalog"] + (strenv(MAGELIFT_ACCEPTANCE_CATALOG_PATH) | split(".")); strenv(MAGELIFT_ACCEPTANCE_CELL_VALUE))' "$LIVE_CONFIG"
	MAGELIFT_ACCEPTANCE_CELL_VALUE="$value" MAGELIFT_ACCEPTANCE_CATALOG_PATH="$catalog_path" \
		yq -i 'setpath(["environments", strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT), "target", "aws", "catalog"] + (strenv(MAGELIFT_ACCEPTANCE_CATALOG_PATH) | split(".")); strenv(MAGELIFT_ACCEPTANCE_CELL_VALUE))' "$LIVE_CONFIG"
	printf 'acceptance patched %s=%s into temp config\n' "$key" "$value" >&2
}

configured_queue_mode() {
	local value
	if [[ "$target_runtime" == "eks" ]]; then
		MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile" \
			value=$(yq -r \
				'(.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.catalog.eks.queueMode // .target.aws.catalog.eks.queueMode // "")' \
				"$LIVE_CONFIG")
	else
		MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile" \
			value=$(yq -r \
				'(.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.catalog.queueMode // .target.aws.catalog.queueMode // "")' \
				"$LIVE_CONFIG")
	fi
	printf '%s' "$value"
}

configured_compute_mode() {
	local value
	if [[ "$target_runtime" == "eks" ]]; then
		MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile" \
			value=$(yq -r \
				'(.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.catalog.eks.computeMode // .target.aws.catalog.eks.computeMode // "auto-mode")' \
				"$LIVE_CONFIG")
	else
		MAGELIFT_ACCEPTANCE_ENVIRONMENT="$profile" \
			value=$(yq -r \
				'(.environments[strenv(MAGELIFT_ACCEPTANCE_ENVIRONMENT)].target.aws.catalog.fargate.computeMode // .target.aws.catalog.fargate.computeMode // "fargate")' \
				"$LIVE_CONFIG")
	fi
	printf '%s' "$value"
}

record_create_once_baseline() {
	local duration="${1:?create-once duration required}" first_cell configured value provider account date_s
	load_cells
	first_cell="${CELLS[0]:-}"
	case "$first_cell" in
	computeMode:*)
		configured="$(configured_compute_mode)"
		value="${first_cell#computeMode:}"
		;;
	queueMode:*)
		configured="$(configured_queue_mode)"
		value="${first_cell#queueMode:}"
		;;
	*)
		return 0
		;;
	esac
	if [[ -z "$configured" || "$configured" != "$value" ]]; then
		printf 'acceptance create-once baseline not recorded: configured queueMode=%s first cell=%s\n' "$configured" "$first_cell" >&2
		return 0
	fi
	if cell_done "$first_cell"; then
		return 0
	fi
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-aws}"
	account="$(acceptance_account_id)"
	date_s="$(date -u +%Y-%m-%d)"
	export MAGELIFT_ACCEPTANCE_SESSION_MODE=baseline
	export MAGELIFT_ACCEPTANCE_STACK_ID="${project_tag}-${profile}-aws-${target_runtime}"
	export MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS="$duration"
	acceptance_evidence_ensure
	append_row "$first_cell" "PASS" "${duration}s" "$provider" "$account" "$date_s"
	append_shared_row "$first_cell" "PASS" "${duration}s" "$provider" "$account" "$date_s"
	record_cell "$first_cell" "PASS"
	printf 'acceptance create-once baseline cell=%s result=PASS duration=%ss\n' "$first_cell" "$duration" >&2
}

acceptance_account_id() {
	if [[ -n "${MAGELIFT_ACCEPTANCE_ACCOUNT:-}" ]]; then
		printf '%s' "$MAGELIFT_ACCEPTANCE_ACCOUNT"
		return 0
	fi
	aws sts get-caller-identity --query Account --output text 2>/dev/null || printf 'unknown'
}

replay_passed_cell_patches() {
	local cell key
	load_cells
	acceptance_checkpoint_load
	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			key="${cell%%:*}"
			if [[ "$key" == "computeMode" ]]; then
				printf 'acceptance resume: skip replay cell=%s (architecture identity, already in YAML)\n' "$cell" >&2
				continue
			fi
			printf 'acceptance resume: replay patch cell=%s\n' "$cell" >&2
			if ! apply_cell_config "$cell"; then
				printf 'acceptance resume: failed to replay patch cell=%s\n' "$cell" >&2
				return 1
			fi
		fi
	done
}

live_cell_loop() {
	local cell provider account date_s duration started result rc first_cell session_mode cell_count
	provider="${MAGELIFT_ACCEPTANCE_PROVIDER:-aws}"
	account="$(acceptance_account_id)"
	load_cells
	first_cell="${CELLS[0]:-}"
	acceptance_checkpoint_load
	acceptance_evidence_ensure
	cell_count="$(jq '.cells | length' "$ACCEPTANCE_CHECKPOINT")"

	printf 'aws acceptance live cell loop cells=%d catalog=%s\n' "${#CELLS[@]}" "$CELL_CATALOG" >&2

	for cell in "${CELLS[@]}"; do
		if cell_done "$cell"; then
			printf 'acceptance skip cell=%s (checkpoint)\n' "$cell" >&2
			continue
		fi

		printf 'acceptance cell-update cell=%s\n' "$cell" >&2
		if [[ "$cell_count" == 0 ]]; then
			session_mode=baseline
		else
			session_mode=reused
		fi
		export MAGELIFT_ACCEPTANCE_SESSION_MODE="$session_mode"
		started=$(date +%s)
		result=PASS
		rc=0
		if ! apply_cell_config "$cell"; then
			result=FAIL
			rc=1
		elif [[ "$cell" == resilience:node-loss ]]; then
			if ! run_aws_eks_node_loss_cell; then
				result=FAIL
				rc=1
			fi
		elif [[ "$cell" == resilience:zone-loss ]]; then
			if ! run_aws_eks_zone_loss_cell; then
				result=FAIL
				rc=1
			fi
		elif [[ "$cell" == observability:cloudwatch ]]; then
			if ! run_aws_eks_cloudwatch_observability_cell; then
				result=FAIL
				rc=1
			fi
		elif [[ "$cell" == computeMode:* ]] && cell_done "$first_cell"; then
			# Compute mode is an architecture boundary. The first architecture cell
			# is recorded by create-once; a different mode is rejected by
			# apply_cell_config instead of being treated as a warm transition.
			:
		elif [[ "$cell" == queueMode:* || "$cell" == searchMode:* ]] && cell_done "$first_cell"; then
			# Service-mode cells change the runtime graph, not Magento's schema. The
			# create-once baseline already ran the candidate migration, so keep
			# subsequent cells to one Pulumi update plus explicit health checks.
			if ! run deploy --infra-only --yes; then
				result=FAIL
				rc=1
			elif ! run outputs; then
				result=FAIL
				rc=1
			elif ! run_runtime_health_with_retry; then
				result=FAIL
				rc=1
			fi
		elif ! run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes; then
			result=FAIL
			rc=1
		elif ! run outputs; then
			result=FAIL
			rc=1
		elif [[ "$cell" == databaseEngine:* ]] && ! seed_database_if_configured; then
			# Engine replacement creates an empty writer. Magento migrate ran on the
			# previous database before Pulumi swapped endpoints, so re-seed before health.
			result=FAIL
			rc=1
		elif ! run_runtime_health_with_retry; then
			result=FAIL
			rc=1
		fi

		duration="$(( $(date +%s) - started ))s"
		export MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS="${duration%s}"
		date_s=$(date -u +%Y-%m-%d)
		append_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		append_shared_row "$cell" "$result" "$duration" "$provider" "$account" "$date_s"
		record_cell "$cell" "$result"
		cell_count=$((cell_count + 1))
		printf 'acceptance cell-done cell=%s result=%s\n' "$cell" "$result" >&2
		if [[ "$rc" -ne 0 ]]; then
			printf 'acceptance cell failed; recorded FAIL for resume (re-run with KEEP/RESUME)\n' >&2
			return 1
		fi
	done
}

# --- live main -------------------------------------------------------------------

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf 'aws acceptance start profile=%s at=%s\n' "$profile" "$started_at" >&2

acceptance_paths
acceptance_prepare_lifecycle
ACCEPTANCE_TTL_MARKER_FILE="$LIVE_WORKDIR/acceptance-ttl-expired"
acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ACCEPTANCE_TTL_MARKER_FILE"
apply_acceptance_labels
ACCEPTANCE_CHECKPOINT_FINGERPRINT="$({
	printf 'provider=aws\nprofile=%s\nregion=%s\nproject=%s\n' "$profile" "$region" "$project_tag"
	yq -o=json 'del(.target.aws.labels."magelift:acceptance-run", .environments[]?.expiresAt, .environments[]?.target.aws.labels."magelift:acceptance-run")' "$LIVE_CONFIG"
	printf 'edition=%s\nphp=%s\ncomposer=%s\nextensions=%s\ndatabase=%s\nsearch=%s\nqueue=%s\ncache=%s\nwebCache=%s\nedge=%s\n' \
		"$(yq -r '.application.edition // "open-source"' "$LIVE_CONFIG")" \
		"$(yq -r '.build.php // ""' "$LIVE_CONFIG")" \
		"$(yq -r '.build.composer.version // ""' "$LIVE_CONFIG")" \
		"$(yq -r '(.build.extensions // []) | sort | join(",")' "$LIVE_CONFIG")" \
		"$(yq -r '.target.aws.catalog.databaseEngine // .target.aws.catalog.engine // "rds-mysql"' "$LIVE_CONFIG")" \
		"$(yq -r '(.target.aws.catalog.eks.searchMode // .target.aws.catalog.searchMode // "disabled")' "$LIVE_CONFIG")" \
		"$(yq -r '(.target.aws.catalog.eks.queueMode // .target.aws.catalog.queueMode // "db")' "$LIVE_CONFIG")" \
		"$(yq -r '.target.aws.catalog.cacheMode // "valkey"' "$LIVE_CONFIG")" \
		"$(yq -r '.target.aws.catalog.varnishMode // "varnish"' "$LIVE_CONFIG")" \
		"$(yq -r '(.edge.externalProvider // .edge.nativeProvider // "none")' "$LIVE_CONFIG")"
	printf 'computeMode=%s\n' "$(configured_compute_mode)"
	printf '\ncell-catalog:\n'
	cat "$CELL_CATALOG"
} | shasum -a 256 | awk '{print $1}')"
export ACCEPTANCE_CHECKPOINT_FINGERPRINT
MAGELIFT_ACCEPTANCE_RELEASE="$(yq -r '.application.version // "unknown"' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_EDITION="$(yq -r '.application.edition // "open-source"' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_RUNTIME="$(yq -r '.target.runtime // "ecs-fargate"' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_PHP_VERSION="$(yq -r '.build.php // ""' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS="$(yq -r '(.build.extensions // ["intl", "pdo_mysql"]) | join(",")' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_COMPOSER_VERSION="$(yq -r '.build.composer.version // ""' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_DATABASE="$(yq -r '.target.aws.catalog.databaseEngine // .target.aws.catalog.engine // "rds-mysql"' "$LIVE_CONFIG")"
if [[ "$target_runtime" == "eks" ]]; then
	MAGELIFT_ACCEPTANCE_COMPUTE_MODE="$(configured_compute_mode)"
	MAGELIFT_ACCEPTANCE_SEARCH="$(yq -r '.target.aws.catalog.eks.searchMode // "disabled"' "$LIVE_CONFIG")"
	MAGELIFT_ACCEPTANCE_QUEUE="$(yq -r '.target.aws.catalog.eks.queueMode // "database"' "$LIVE_CONFIG")"
else
	MAGELIFT_ACCEPTANCE_COMPUTE_MODE="$(configured_compute_mode)"
	MAGELIFT_ACCEPTANCE_SEARCH="$(yq -r '.target.aws.catalog.searchMode // "disabled"' "$LIVE_CONFIG")"
	MAGELIFT_ACCEPTANCE_QUEUE="$(yq -r '.target.aws.catalog.queueMode // "db"' "$LIVE_CONFIG")"
fi
MAGELIFT_ACCEPTANCE_CACHE="$(yq -r '.target.aws.catalog.cacheMode // "valkey"' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_WEB_CACHE="$(yq -r '.target.aws.catalog.varnishMode // "varnish"' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_EDGE="$(yq -r '(.edge.externalProvider // .edge.nativeProvider // "none")' "$LIVE_CONFIG")"
MAGELIFT_ACCEPTANCE_KUBERNETES_MODE="$(acceptance_shared_kubernetes_mode "$MAGELIFT_ACCEPTANCE_RUNTIME")"
export MAGELIFT_ACCEPTANCE_RELEASE MAGELIFT_ACCEPTANCE_EDITION MAGELIFT_ACCEPTANCE_RUNTIME
export MAGELIFT_ACCEPTANCE_PRESET="$profile"
export MAGELIFT_ACCEPTANCE_DIGEST="$MAGELIFT_AWS_ACCEPTANCE_DIGEST"
export MAGELIFT_ACCEPTANCE_PHP_VERSION MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS MAGELIFT_ACCEPTANCE_COMPOSER_VERSION
export MAGELIFT_ACCEPTANCE_DATABASE MAGELIFT_ACCEPTANCE_COMPUTE_MODE MAGELIFT_ACCEPTANCE_SEARCH MAGELIFT_ACCEPTANCE_QUEUE
export MAGELIFT_ACCEPTANCE_CACHE MAGELIFT_ACCEPTANCE_WEB_CACHE MAGELIFT_ACCEPTANCE_EDGE
export MAGELIFT_ACCEPTANCE_STACK_ID="${project_tag}-${profile}-aws-${target_runtime}"
export MAGELIFT_ACCEPTANCE_KUBERNETES_MODE
aws_seed_path="$(yq -r ".environments.\"$profile\".seedDump // \"\"" "$LIVE_CONFIG")"
if [[ -n "$aws_seed_path" && "$aws_seed_path" != /* ]]; then
	aws_seed_path="$(cd "$(dirname "$MAGELIFT_CONFIG")" && pwd)/$aws_seed_path"
fi
aws_seed_fixture="$(acceptance_seed_fixture_id "$aws_seed_path")"
acceptance_export_reuse_boundary \
	"$aws_seed_fixture" \
	"aws-rds-automated:${project_tag}-${profile}" \
	"cloudwatch" \
	"$MAGELIFT_ACCEPTANCE_EDGE" \
	"$MAGELIFT_AWS_ACCEPTANCE_DIGEST" \
	"$aws_seed_fixture" \
	"${PULUMI_BACKEND_URL:?PULUMI_BACKEND_URL is required for reuse-boundary evidence}"
ensure_state_bucket
run config validate
run doctor
run login

if [[ "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-false}" == true || "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-0}" == 1 ]]; then
	if ! assert_resume_scope; then
		printf 'refusing AWS acceptance resume because the retained stack scope is not exact\n' >&2
		exit 2
	fi
elif ! assert_clean; then
	printf 'refusing AWS acceptance mutation because the exact project scope is not clean\n' >&2
	exit 2
fi

if should_skip_create_once; then
	printf 'acceptance resume: skipping create-once (stack assumed present; KEEP/RESUME/checkpoint)\n' >&2
	created=1
else
	printf 'acceptance create-once\n' >&2
	create_once_started=$(date +%s)
	run preview
	acceptance_maybe_sign_digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST"
	acceptance_promote_digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:-}" "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:-}"
	created=1
	run deploy --infra-only --yes
	seed_database_if_configured
	run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes
	run outputs
	run_runtime_health_with_retry
	create_once_duration="$(( $(date +%s) - create_once_started ))"
	record_create_once_baseline "$create_once_duration"
fi

if [[ "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-false}" == true || "${MAGELIFT_AWS_ACCEPTANCE_RESUME:-0}" == 1 ]]; then
	# Origin YAML is copied over the workdir copy on every shell. Replay PASS
	# cells so resume matches the living stack (engine, queue, search, NAT).
	if ! replay_passed_cell_patches; then
		printf 'refusing AWS acceptance resume because PASS cell patches could not be replayed onto the live config\n' >&2
		exit 2
	fi
fi

if [[ "${MAGELIFT_AWS_ACCEPTANCE_RESEED_ONLY:-}" == 1 || "${MAGELIFT_AWS_ACCEPTANCE_RESEED_ONLY:-}" == true ]]; then
	printf 'acceptance reseed-only (no cell loop)\n' >&2
	seed_database_if_configured
	exit $?
fi

# Resume skips create-once, including the dump import that Magento deploy
# expects. Seed before the first unfinished Magento cell; do not re-import
# after that cell is already PASS.
load_cells
acceptance_checkpoint_load
if should_skip_create_once && [[ -n "${CELLS[0]:-}" ]] && ! cell_done "${CELLS[0]}"; then
	if [[ "$target_runtime" == "eks" && -f "$LIVE_WORKDIR/eks-seed-import.json" ]]; then
		printf 'acceptance resume: skipping EKS seed; dump import already completed\n' >&2
	else
		printf 'acceptance resume: create-once Magento did not finish; seeding before cell deploy\n' >&2
		seed_database_if_configured
	fi
fi

live_cell_loop

printf 'aws acceptance ok profile=%s; destroy + assert_clean run on EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true\n' "$profile" >&2

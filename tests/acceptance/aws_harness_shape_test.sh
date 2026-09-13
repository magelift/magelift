#!/usr/bin/env bash
# Offline AWS harness check: a resumed dry-run skips completed cells and keeps
# the live cleanup contract visible in the script.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/aws-acceptance-local.sh"
CATALOG="$ROOT/scripts/acceptance/cells-aws-preview.txt"
SELF_MANAGED_RESILIENCE_CATALOG="$ROOT/scripts/acceptance/cells-aws-eks-self-managed-resilience.txt"
SELF_MANAGED_ZONE_RESILIENCE_CATALOG="$ROOT/scripts/acceptance/cells-aws-eks-self-managed-zone-resilience.txt"
SELF_MANAGED_OBSERVABILITY_CATALOG="$ROOT/scripts/acceptance/cells-aws-eks-self-managed-observability.txt"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ACCEPTANCE_CHECKPOINT="$TMP/checkpoint.json"
export ACCEPTANCE_EVIDENCE="$TMP/evidence.md"
export ACCEPTANCE_SHARED_EVIDENCE="$TMP/evidence.jsonl"
export MAGELIFT_ACCEPTANCE_CELL_CATALOG="$CATALOG"
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
export MAGELIFT_ACCEPTANCE_PROVIDER=aws
export MAGELIFT_ACCEPTANCE_ACCOUNT=test-aws

mapfile -t CELLS < <(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$CATALOG")
if [[ "${#CELLS[@]}" -lt 2 ]]; then
	printf 'AWS preview catalog needs at least two cells\n' >&2
	exit 1
fi

source "$ROOT/scripts/acceptance/lib-checkpoint.sh"
acceptance_checkpoint_ensure
record_cell "${CELLS[0]}" PASS

for required in 'MAGELIFT_AWS_ACCEPTANCE_KEEP' 'trap cleanup EXIT' 'magelift destroy --yes' 'deploy --infra-only' 'wait_for_clean' 'assert_clean' 'assert_resume_scope' 'non-empty Pulumi stack' 'same-project resources outside ownership marker' 'cleanup_acceptance_log_groups' 'cleanup_acceptance_cache_subnet_groups' 'cleanup_acceptance_prerequisite_secrets' 'MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS' 'MAGELIFT_AWS_ACCEPTANCE_DELETE_PREREQUISITE_SECRETS' 'export AWS_PROFILE=default' 'ACCEPTANCE_CHECKPOINT_FINGERPRINT' 'configured_project' 'must exactly match' 'refusing AWS acceptance mutation' 'MAGELIFT_AWS_ACCEPTANCE_BACKEND_URL' 'state_bucket="${MAGELIFT_AWS_ACCEPTANCE_STATE_BUCKET:-' 'configured_compute_mode' 'separate cold run' 'qualify_aws_pulumi_stack_ref' 'organization/magelift/' 'dump_pod="magelift-${profile}-seed-import"' 'run_aws_eks_node_loss_cell' 'run_aws_eks_zone_loss_cell' 'run_aws_eks_cloudwatch_observability_cell' 'resilience:node-loss' 'resilience:zone-loss' 'observability:cloudwatch' 'acceptance_checkpoint_bind_run_id'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'AWS harness missing %s\n' "$required" >&2
		exit 1
	fi
done
for required in 'lib-cosign.sh' 'acceptance_require_certificate_identity' 'acceptance_maybe_sign_digest' 'acceptance_promote_digest' 'acceptance_export_reuse_boundary' 'aws-rds-automated:'; do
	if ! grep -q "$required" "$SCRIPT"; then
		printf 'AWS Cosign harness missing %s\n' "$required" >&2
		exit 1
	fi
done
if [[ "$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$SELF_MANAGED_RESILIENCE_CATALOG" | head -n 1)" != 'computeMode:self-managed' ]] || ! grep -qxF 'resilience:node-loss' "$SELF_MANAGED_RESILIENCE_CATALOG"; then
	printf 'self-managed EKS resilience catalog must start with computeMode:self-managed and include resilience:node-loss\n' >&2
	exit 1
fi
if [[ "$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$SELF_MANAGED_ZONE_RESILIENCE_CATALOG" | head -n 1)" != 'computeMode:self-managed' ]] || ! grep -qxF 'resilience:zone-loss' "$SELF_MANAGED_ZONE_RESILIENCE_CATALOG"; then
	printf 'self-managed EKS zone-resilience catalog must start with computeMode:self-managed and include resilience:zone-loss\n' >&2
	exit 1
fi
if [[ "$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$SELF_MANAGED_OBSERVABILITY_CATALOG" | head -n 1)" != 'computeMode:self-managed' ]] || ! grep -qxF 'observability:cloudwatch' "$SELF_MANAGED_OBSERVABILITY_CATALOG"; then
	printf 'self-managed EKS observability catalog must start with computeMode:self-managed and include observability:cloudwatch\n' >&2
	exit 1
fi
if ! grep -q 'resilience:node-loss' "$SCRIPT" || ! grep -q 'resilience:zone-loss' "$SCRIPT" || ! grep -q 'observability:cloudwatch' "$SCRIPT" || ! grep -q 'ec2 terminate-instances' "$SCRIPT" || ! grep -q 'replacementInstance' "$SCRIPT" || ! grep -q 'zoneReadyGapObserved' "$SCRIPT" || ! grep -q 'ContainerInsights' "$SCRIPT" || ! grep -q 'kube-apiserver-audit' "$SCRIPT"; then
	printf 'AWS self-managed node/zone-loss/observability contract is incomplete\n' >&2
	exit 1
fi
if grep -q 'seed_image="\\${MAGELIFT_AWS_ACCEPTANCE_SEED_IMAGE' "$SCRIPT"; then
	printf 'AWS EKS seed image default must expand before kubectl run\n' >&2
	exit 1
fi
if grep -q 'if \[\[ -z "\${PULUMI_BACKEND_URL:-}" \]\]' "$SCRIPT"; then
	printf 'AWS harness must not trust an ambient PULUMI_BACKEND_URL\n' >&2
	exit 1
fi
for required in 'list-origin-access-controls' 'Origins.Items ||'; do
	if ! grep -q "$required" "$ROOT/scripts/acceptance/lib-assert-clean-aws.sh"; then
		printf 'AWS cleanup library missing %s\n' "$required" >&2
		exit 1
	fi
done
if ! grep -q 'list-objects-v2' "$SCRIPT"; then
	printf 'AWS state bucket readiness must verify object listing before Pulumi starts\n' >&2
	exit 1
fi
if ! grep -Fq '.[:1000]' "$SCRIPT"; then
	printf 'AWS state cleanup must batch versioned S3 deletes at the service limit\n' >&2
	exit 1
fi
if ! grep -Fq '"$cell" == queueMode:* || "$cell" == searchMode:*' "$SCRIPT"; then
	printf 'AWS warm matrix must use infra-only deploys for queue and search mode cells\n' >&2
	exit 1
fi
if ! grep -Fq 'architecture cells require a separate cold run' "$SCRIPT"; then
	printf 'AWS harness must not warm-transition between compute architectures\n' >&2
	exit 1
fi
if ! grep -Fq -- '--capacity-provider-strategy' "$SCRIPT"; then
	printf 'AWS seed import must place Managed Instances tasks with a capacity provider strategy\n' >&2
	exit 1
fi
if ! grep -Fq 'create-once Magento did not finish; seeding before cell deploy' "$SCRIPT"; then
	printf 'AWS resume must seed before Magento when create-once never finished\n' >&2
	exit 1
fi
if ! grep -Fq 'aws_apply_magento_storefront_base_url' "$SCRIPT"; then
	printf 'AWS EKS Magento health must set storefront base_url on the public NLB\n' >&2
	exit 1
fi
if ! grep -Fq 'skipping EKS seed; dump import already completed' "$SCRIPT"; then
	printf 'AWS EKS resume must not re-import a completed seed dump\n' >&2
	exit 1
fi
if ! grep -Fq '(.cells | type == "object" and (.cells | length) == 0)' "$SCRIPT"; then
	printf 'AWS resume must accept an empty checkpoint after create-once health failure\n' >&2
	exit 1
fi

declare -A expected_architectures=(
	["cells-aws-eks-architecture-auto-mode.txt"]='computeMode:auto-mode'
	["cells-aws-eks-architecture-managed-node-groups.txt"]='computeMode:managed-node-groups'
	["cells-aws-eks-architecture-self-managed.txt"]='computeMode:self-managed'
	["cells-aws-eks-architecture-fargate.txt"]='computeMode:fargate'
	["cells-aws-ecs-architecture-fargate.txt"]='computeMode:fargate'
	["cells-aws-ecs-architecture-fargate-spot.txt"]='computeMode:fargate-spot'
	["cells-aws-ecs-architecture-ec2-asg.txt"]='computeMode:ec2-asg'
	["cells-aws-ecs-architecture-managed-instances.txt"]='computeMode:managed-instances'
)
for catalog_name in "${!expected_architectures[@]}"; do
	catalog_path="$ROOT/scripts/acceptance/$catalog_name"
	if [[ ! -f "$catalog_path" ]]; then
		printf 'missing architecture catalog %s\n' "$catalog_name" >&2
		exit 1
	fi
	first_cell=$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$catalog_path" | head -n 1)
	if [[ "$first_cell" != "${expected_architectures[$catalog_name]}" ]]; then
		printf '%s first cell = %s, want %s\n' "$catalog_name" "$first_cell" "${expected_architectures[$catalog_name]}" >&2
		exit 1
	fi
done

LOG="$TMP/aws-dry-run.log"
set +e
bash "$SCRIPT" >"$LOG" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
	printf 'AWS dry-run exited %s\n' "$rc" >&2
	cat "$LOG" >&2
	exit 1
fi

if ! grep -q "acceptance skip cell=${CELLS[0]} (checkpoint)" "$LOG"; then
	printf 'AWS resume must skip the completed first cell\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q "acceptance cell-update cell=${CELLS[1]}" "$LOG"; then
	printf 'AWS resume must execute the next cell\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if grep -Eq '\+ magelift (preview|promote|deploy|destroy)|aws (ec2|rds|ecs|elasticache|elbv2|logs) .*(create|delete|run-instances)' "$LOG"; then
	printf 'AWS dry-run must not invoke a mutating command\n' >&2
	cat "$LOG" >&2
	exit 1
fi
if ! grep -q 'aws acceptance dry-run ok' "$LOG"; then
	printf 'AWS dry-run must report success\n' >&2
	cat "$LOG" >&2
	exit 1
fi

printf 'aws_harness_shape_test OK skip=%s ran=%s\n' "${CELLS[0]}" "${CELLS[1]}"

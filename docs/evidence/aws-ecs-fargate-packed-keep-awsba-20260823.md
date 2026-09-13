# AWS ECS Fargate packed KEEP — 2026-08-23 (`awsba`)

Packed Magento origin on ECS Fargate for `certification-aws` task 3.2. One
signed 2.4.9 digest, catalog
`scripts/acceptance/cells-aws-campaign-fargate-keep.txt`. All nine cells PASS,
then harness destroy and `assert_clean ok`. This file does not replace the
certified preview tuple
[20260813ai](aws-ecs-fargate-magento-live-20260813ai.md). Amazon MQ, Aurora,
AOSS (`searchMode:serverless`), and `ha:multi-az` stay experimental.

## Scope

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Project tag | `awsba` |
| Profile | preview |
| Runtime | `ecs-fargate` (`computeMode:fargate`) |
| Catalog spec | `certification-aws` |
| Catalog | `scripts/acceptance/cells-aws-campaign-fargate-keep.txt` |
| Artifact | ECR `magelift-acceptance-20260823ba@sha256:8588b13f…fdb2be4` (same bytes as `gcap28` / `gcap29`) |
| Cosign | Google SA `devops@…iam.gserviceaccount.com` / `https://accounts.google.com` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Run id | `run-20260823t154325z-63566` |
| Wrapper | `scripts/aws-acceptance-local.sh` with `MAGELIFT_AWS_ACCEPTANCE_KEEP` then `KEEP=false` resume |

## Catalog

| Cell | Result | Session |
| --- | --- | --- |
| `computeMode:fargate` | PASS 2243s | baseline |
| `queueMode:db` | PASS 172s | reused |
| `queueMode:ecs-rabbitmq` | PASS 219s | reused |
| `queueMode:amazon-mq` | PASS 945s | reused |
| `searchMode:disabled` | PASS 174s | reused |
| `searchMode:serverless` | PASS 173s | reused |
| `databaseEngine:rds-mysql` | PASS 1635s | reused |
| `databaseEngine:aurora-mysql` | PASS 2036s | reused |
| `ha:multi-az` | PASS 2277s | reused |

Warm cells used the same stack `awsba-preview-aws-ecs-fargate`. Checkpoint
cleanup was pending until destroy.

## Destroy

`KEEP=false` resume on 2026-08-23T20:11:06Z skipped create-once, skipped every
PASS cell, then EXIT `magelift destroy --yes`. Pulumi deleted 136 resources in
26m2s, deleted state bucket
`magelift-<aws-account>-eu-west-3-awsba-preview-state`, RDS error log group,
and force-deleted the three `awsba` prerequisite secrets. Harness
`assert_clean ok` (tag index still reported tombstones; live inventories
passed). Exit 0 at 2026-08-23T20:37:44Z.

## Non-claims

- Certified-row replacement for [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md)
- Managed Instances, EKS, provisioned OpenSearch, Magento-origin CloudFront
- X-Ray Magento traces (typed unsupported)
- Homebrew/Scoop

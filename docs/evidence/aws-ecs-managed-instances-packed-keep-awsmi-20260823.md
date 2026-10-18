# AWS ECS Managed Instances packed KEEP — 2026-08-23 (`awsmi`)

Cold Magento origin on ECS Managed Instances for `certification-aws` task 3.2.
One signed 2.4.9 digest, catalog
`scripts/acceptance/cells-aws-ecs-architecture-managed-instances.txt`. Both
cells PASS, then harness destroy and `assert_clean ok`. Experimental. This file
does not certify Managed Instances and does not replace the Fargate preview
tuple [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md).

## Scope

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Project tag | `awsmi` |
| Profile | preview |
| Runtime | `ecs-fargate` (`computeMode:managed-instances`) |
| Catalog spec | `certification-aws` |
| Catalog | `scripts/acceptance/cells-aws-ecs-architecture-managed-instances.txt` |
| Artifact | ECR `magelift-acceptance-20260823ba@sha256:8588b13f…fdb2be4` (same bytes as `gcap28` / `gcap29` / `awsba`) |
| Cosign | Google SA `devops@…iam.gserviceaccount.com` / `https://accounts.google.com` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Run id | `run-20260823t210441z-8825` |
| Wrapper | `scripts/aws-acceptance-local.sh` with isolated checkpoint `.magelift/awsmi/acceptance-checkpoint.json`, `KEEP` then `KEEP=false` resume |

## Catalog

| Cell | Result | Session |
| --- | --- | --- |
| `computeMode:managed-instances` | PASS 1592s | reused (create-once failed before Magento; resume seeded then deployed) |
| `queueMode:db` | PASS 209s | reused |

Stack `awsmi-preview-aws-ecs-fargate`. Runtime health passed after the Managed
Instances capacity provider reached `ACTIVE` and the seed dump imported through
that provider (not Fargate `run-task`).

## Destroy

`KEEP=false` resume on 2026-08-23T21:50:51Z skipped PASS cells, then EXIT
`magelift destroy --yes`. Pulumi deleted 119 resources in 19m46s, deleted state
bucket `magelift-<aws-account>-eu-west-3-awsmi-preview-state`, RDS error log
group, and force-deleted the three `awsmi` prerequisite secrets. Harness
`assert_clean ok` (tag index still reported tombstones; live inventories
passed). Exit 0 at 2026-08-23T22:11:08Z.

## Non-claims

- Certified Managed Instances, or replacement of [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md)
- EKS, provisioned OpenSearch, Magento-origin CloudFront
- Mixing Managed Instances with Fargate or EC2 Auto Scaling capacity providers
- Homebrew/Scoop

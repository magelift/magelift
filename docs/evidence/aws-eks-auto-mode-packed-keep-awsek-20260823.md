# AWS EKS Auto Mode packed KEEP — 2026-08-23 (`awsek`)

Cold Magento origin on EKS Auto Mode for `certification-aws` task 3.2.
One signed 2.4.9 digest, catalog
`scripts/acceptance/cells-aws-campaign-eks-keep.txt`. Four cells PASS,
then harness destroy and `assert_clean ok`. Experimental. This file does
not certify EKS and does not replace the Fargate preview tuple
[20260813ai](aws-ecs-fargate-magento-live-20260813ai.md) or the
self-managed EKS row
[20260817bf](aws-eks-self-managed-magento-live-20260817bf.md).

## Scope

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Project tag | `awsek` |
| Profile | preview |
| Runtime | `eks` (`computeMode:auto-mode`) |
| Catalog spec | `certification-aws` |
| Catalog | `scripts/acceptance/cells-aws-campaign-eks-keep.txt` |
| Artifact | ECR `magelift-acceptance-20260823ba@sha256:8588b13f…fdb2be4` (same bytes as `gcap28` / `gcap29` / `awsba` / `awsmi`) |
| Cosign | Google SA `devops@…iam.gserviceaccount.com` / `https://accounts.google.com` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Run id | `run-20260823t221235z-34081` |
| Wrapper | `scripts/aws-acceptance-local.sh` with isolated checkpoint `.magelift/awsek/acceptance-checkpoint.json`, `KEEP` then `KEEP=false` resume |

## Catalog

| Cell | Result | Session |
| --- | --- | --- |
| `computeMode:auto-mode` | PASS 313s | reused (create-once Magento HTTP timed out on an internal NLB; resume after internet-facing Service + live LoadBalancer probe) |
| `queueMode:database` | PASS 19s | reused |
| `queueMode:rabbitmq` | PASS 97s | reused |
| `searchMode:disabled` | PASS 17s | reused |

Stack `awsek-preview-aws-eks`. Runtime health required Magento HTTP 200 on the
public NLB after Magento `base_url` was set to that origin. Search stayed
disabled.

## Destroy

`KEEP=false` resume on 2026-08-23T23:14:50Z skipped PASS cells, then EXIT
`magelift destroy --yes`. Pulumi deleted 109 resources in 13m23s, deleted state
bucket `magelift-<aws-account>-eu-west-3-awsek-preview-state`, Container
Insights and EKS/RDS log groups, and force-deleted the three `awsek`
prerequisite secrets. Harness `assert_clean ok` (tag index still reported
tombstones; live inventories passed). Exit 0 at 2026-08-23T23:28:45Z.

## Non-claims

- Certified EKS, or replacement of [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md) or [20260817bf](aws-eks-self-managed-magento-live-20260817bf.md)
- Managed node groups, self-managed nodes, or EKS Fargate compute
- OpenSearch, Magento-origin CloudFront, HA
- Homebrew/Scoop

# AWS ECS Fargate Magento 2.4.9 live cell — 2026-08-13 (`20260813ai`)

This is a bounded Magento application cold baseline on AWS ECS Fargate. It
proves signed-digest promote, seed import into RDS, ECS runtime health, a
warm `queueMode:db` infra-only health check, destroy of the 101-resource
stack, disposable state-bucket deletion, prerequisite-secret force-delete,
and independent empty stack inventories. It does not claim ALB HTTP 200,
custom-domain TLS, Magento origin through CloudFront or Fastly, Fargate
Spot/EC2/Managed Instances interruption, HA, DR, collectors, or WAF.

Earlier attempt `20260813ah` failed at `magelift promote` (`signed release
verification failed`) with `created=0`. That run ID is not a PASS.

## Scope

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Run ID | `20260813ai` |
| Project tag | `awsah` |
| Profile | preview |
| Runtime | `ecs-fargate` (`computeMode:fargate`) |
| Catalog spec | `certification-aws` |
| Cell | runtime `ecs-fargate`, compute `fargate`, Magento 2.4.9, `rds-mysql` 8.4.10, search `disabled`, queue `db`, digest below |
| Network | single-AZ `fck-nat`, CIDR `10.83.0.0/16` |
| Edge | `edge.mode: none` (media CloudFront only) |
| Catalog | `scripts/acceptance/cells-aws-ecs-architecture-fargate.txt` |
| Database | RDS MySQL `8.4.10`, database `magento` |
| Cache | ElastiCache Valkey `9.0` `cache.t4g.micro` |
| Queue | `queueMode:db` |
| Search | disabled |
| Artifact | ECR `magelift-acceptance-rc1-20260813@sha256:e82dcf767cf57492b1c5619eac708d9af8b2e66f7b144ff49e6b2513753db495` |
| Cosign | `devops@<gcp-project>.iam.gserviceaccount.com` / `https://accounts.google.com` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (gitignored; values not logged) |
| Wrapper | `scripts/aws-acceptance-local.sh` |
| Wall clock | 3364723 ms (~56 min), exit 0 |

Destroyed identities (not live): VPC `vpc-0b19091fa6feba3e7`, ALB
`awsah-preview-ingress-alb-1981458961.eu-west-3.elb.amazonaws.com`, media
CloudFront `droyr6v2qdokt.cloudfront.net`, RDS
`awsah-preview-database-instance6e11f90.cvcuaioecipe.eu-west-3.rds.amazonaws.com`,
ECS cluster `awsah-preview-runtime-cluster-984e6ca`, stack
`awsah-preview-aws-ecs-fargate`.

## Observed result

1. Promote verified the Cosign signature on the ECR digest. Create-once
   planned 101 resources. Seed import completed (`database=magento`).
2. Baseline `computeMode:fargate` PASSed in 1982s. Runtime health was
   ECS-only: service 1/1 running, primary rollout completed, task healthy.
   Catalog has no `cutover:dns`. Custom-domain HTTPS was not exercised.
3. Warm `queueMode:db` used `--infra-only`, retried while rollout was
   incomplete, then reached the same ECS runtime healthy state (`result=PASS`).
4. EXIT destroy deleted 101 resources in 19m42s, then deleted state bucket
   `magelift-<aws-account>-eu-west-3-awsah-preview-state`, RDS error log group,
   and force-deleted the three `awsah` prerequisite secrets. Harness
   `assert_clean ok` (tag index still reported 38 mappings; live inventories
   passed).

## Independent inventory after destroy

Empty: VPC, RDS, ECS clusters, ALB named `awsah`, ElastiCache, S3 named
`awsah`, Secrets Manager named `awsah`, CloudFront distributions.

Immediately after destroy: unused ISSUED ACM in `eu-west-3`/`us-east-1` and
two Cloudflare ACM-validation CNAMEs (bootstrap from `20260813ah`, `InUse=[]`).
Follow-up deleted those CNAMEs (marker `magelift/acceptance/acm-dns/20260813ah`)
and both certificates (`ResourceNotFoundException` on describe).

Still recorded, not stack resources:

- Customer KMS `237f4553-5cb1-4c15-ace5-824dc44e2301` Enabled (do not delete)
- EC2 `i-<redacted>` State=`terminated` (fck-nat tombstone)

## Non-claims

Spot/EC2/Managed Instances live interruption, EKS, ALB HTTP through the
hostname, Magento Admin/storefront HTTPS on `ml-aws-20260813ah.example.test`,
Magento origin on CloudFront/Fastly, HA, DR, fixtures, and collector wiring
remain open.

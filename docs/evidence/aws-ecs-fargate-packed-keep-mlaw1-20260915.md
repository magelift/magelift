# AWS ECS Fargate packed session — 2026-09-15 (`mlaw1`)

Packed Magento origin on ECS Fargate for `certification-aws` plan 1.5. One
signed 2.4.9 digest, catalog `tests/fixtures/cells-aws-mlaw1.txt` (the
campaign catalog with `searchMode:provisioned` per ADR 0012 instead of
`serverless`). All nine cells PASS, then harness destroy and `assert_clean
ok`. This file does not replace the certified preview tuple
[20260813ai](aws-ecs-fargate-magento-live-20260813ai.md). Amazon MQ, Aurora,
provisioned OpenSearch, and `ha:multi-az` stay experimental.

## Scope

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Project tag | `mlaw1` |
| Profile | preview |
| Runtime | `ecs-fargate` (`computeMode:fargate`) |
| Catalog spec | `certification-aws` |
| Catalog | `tests/fixtures/cells-aws-mlaw1.txt` |
| Artifact | ECR `magelift-acceptance-20260823ba@sha256:8588b13f…fdb2be4` (same bytes as `gcap28` / `gcap29` / `awsba`) |
| Cosign | Google SA `devops@…iam.gserviceaccount.com` / `https://accounts.google.com` |
| Seed | `/tmp/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Binary | `4a63fc7` (with the session fixes below) |
| Wrapper | `scripts/aws-acceptance-local.sh`, no KEEP; destroy on EXIT |

## Catalog

| Cell | Result | Session |
| --- | --- | --- |
| `computeMode:fargate` | PASS 2310s | baseline |
| `queueMode:db` | PASS | reused |
| `queueMode:ecs-rabbitmq` | PASS | reused |
| `queueMode:amazon-mq` | PASS | reused |
| `searchMode:disabled` | PASS | reused |
| `searchMode:provisioned` | PASS | reused |
| `databaseEngine:rds-mysql` | PASS | reused |
| `databaseEngine:aurora-mysql` | PASS | reused |
| `ha:multi-az` | PASS | reused |

Warm cells used the same stack `mlaw1-preview-aws-ecs-fargate`. Warm-cell
durations were not preserved (the harness wipes its scratch dir on EXIT);
only the baseline duration is claimed. First live AWS run of
`searchMode:provisioned` (August ran `serverless`/AOSS).

## Session fixes

The green run needed four fixes found on earlier `mlaw1` attempts:

- `c64b43b`: AWS bootstrap skips KMS keys with denied tag access.
- `095e0d7`: drop unconfigurable `VpcId` from OpenSearch `VpcOptions`.
- `4a63fc7`: prefix `https://` on the native Magento search hostname when
  the endpoint is HTTPS (else `setup:upgrade` dials `http://host:443` and
  the domain answers 400).
- Account-level, one-time: KMS key policy statement for CloudWatch Logs,
  legacy `es.amazonaws.com` service-linked role for VPC OpenSearch,
  `catalog.rabbitMq.instanceType: mq.m7g.large` in the origin YAML
  (preview blanks it), hex cache secret (ElastiCache forbids `/`).

## Search data-plane notes (order-9)

Least privilege for `searchMode:provisioned`: FGAC off, unsigned HTTPS
in-VPC, security groups as the network gate; no public exposure, TLS
enforced, KMS encryption at rest. Magento validated the connection
during `setup:upgrade` on the `databaseEngine:rds-mysql` cell. Explicit
reindex/query/recycle logs were not collected on AWS (the cell uses
`--infra-only`); see order-9 report for the experimental close.

## Destroy

EXIT `magelift destroy --yes` deleted 127 resources in 35m23s, deleted state
bucket `magelift-<aws-account>-eu-west-3-mlaw1-preview-state`, and
force-deleted the three `mlaw1` prerequisite secrets. Harness `assert_clean
ok` (tag index still reported mappings; live inventories passed). Exit 0.

## Spend

Session cap `$25`. Cost Explorer at 2026-09-15T11:00Z showed `$0.03` posted
for 2026-09-15 (EC2-Other, ECS, MQ); RDS/Aurora/OpenSearch/ElastiCache hours
post with delay, so the settled total is higher and unseen at seal time.
Tag-filtered cost was `$0` (allocation tags not activated).

## Non-claims

- Certified-row replacement for [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md)
- AOSS (`searchMode:serverless`), Managed Instances, EKS
- X-Ray Magento traces (typed unsupported)
- Homebrew/Scoop

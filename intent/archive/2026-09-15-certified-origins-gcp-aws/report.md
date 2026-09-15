# Report: certified-origins-gcp-aws — PASS

## What shipped

- GCP packed session: preview `mldp3` (subprocess provider), standard
  `mldp4`, HA `mldp5` 16/16, order-9 search `mldp6` (reindex + query +
  recycle), order-10 loop `mldp7` + `pr-999` lifecycle. All evidence
  committed with sealed JSONL; all stacks destroyed with `assert_clean`.
- AWS packed session: `mlaw1` run 15 went 9/9 (queue db/ecs-rabbitmq/
  amazon-mq, search disabled/provisioned, rds-mysql/aurora-mysql,
  ha:multi-az), `HARNESS_EXIT=0`, `assert_clean ok`. First live AWS
  provisioned OpenSearch. Evidence
  `docs/evidence/aws-ecs-fargate-packed-keep-mlaw1-20260915.md` indexed.
- Code fixes with tests: AWS bootstrap KMS skip (`c64b43b`), OpenSearch
  `VpcId` (`095e0d7`), search `https://` scheme (`4a63fc7`), plus the
  `CONFIG__` search bindings from the `mldp6` proof.
- No capability-matrix status changes (GKE Standard/HA/search/queue
  extras stay experimental; Autopilot/Fargate stay certified).
  `make docs` strict green.

## Deviations

- `mldp3` standard lost to transient GCP Valkey capacity; retried as
  `mldp4` (same config).
- AWS needed 15 attempts: gate/env reconstruction, KMS CloudWatch
  policy, `mediaDomain` cert coverage, `rabbitMq.instanceType`, legacy
  ES service-linked role, hex cache secret, and the three code fixes.
  Two manual teardowns when EXIT destroy preview failed (runs 9, 11).
- `mlaw1` catalog uses `searchMode:provisioned`, not `serverless`
  (per ADR 0012 + plan 1.5); `tests/fixtures/cells-aws-mlaw1.txt`
  records the derivation.

## Spend

- AWS session cap $25: Cost Explorer showed $0.03 posted at seal time
  (bulk unsettled). Cap intact.
- GCP spend per the packed-session evidence files.

## Verdict: pass

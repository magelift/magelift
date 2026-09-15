# Evidence

This page is the current proof pack for MageLift claims. The [capability matrix](../capability-matrix.md) is the summary; a cell is certified only when that matrix and a file here agree. Older attempt logs live in git history, not on this site.

Evidence files map to per-provider catalog cell IDs (`certification-aws`,
`certification-gcp`, `certification-ovh`, `certification-scaleway`). KEEP rows
are not certified until destroy plus orphan assert succeed.

Sealed JSONL for `gencertdocs` is under `runs/`. Maintainers seal candidates with `magelift certification seal` before publishing a new row here. Replace a file when a newer run is the current proof; do not append a ledger.

## Certified Magento

| Claim | Current proof | Not claimed |
| --- | --- | --- |
| AWS ECS Fargate Magento 2.4.9 preview, signed digest, seed import, runtime health, destroy | [20260813ai](aws-ecs-fargate-magento-live-20260813ai.md) | ALB HTTP 200, custom-domain TLS, HA, DR |
| GCP GKE Autopilot Magento 2.4.9 preview catalog, Magento HTTP 200, destroy | [gcap28](gcp-gke-autopilot-magento-live-gcap28-20260820.md) | GKE Standard, HA, or release-wide Adobe cache intersection |

`gcap29` was a retained Autopilot KEEP on the same 2.4.9 digest. Destroy
emptied the prefix; it still does not replace
[gcap28](gcp-gke-autopilot-magento-live-gcap28-20260820.md). See
[gcap29](gcp-gke-autopilot-magento-live-gcap29-20260823.md). There is no signed
2.4.8-p5 image in that account (`not-run`).

## Experimental (bounded)

| Claim | Current proof | Not claimed |
| --- | --- | --- |
| AWS ECS Fargate packed KEEP (amazon-mq, AOSS, Aurora, HA), destroy | [awsba](aws-ecs-fargate-packed-keep-awsba-20260823.md) | Certified-row replacement for 20260813ai; Managed Instances; EKS |
| AWS ECS Fargate packed (amazon-mq, provisioned OpenSearch, Aurora, HA), destroy | [mlaw1](aws-ecs-fargate-packed-keep-mlaw1-20260915.md) | Certified-row replacement for 20260813ai; AOSS/serverless; Managed Instances; EKS |
| AWS ECS Managed Instances Magento KEEP (`queueMode:db`), destroy | [awsmi](aws-ecs-managed-instances-packed-keep-awsmi-20260823.md) | Certified Managed Instances; mix with Fargate/ASG providers; EKS |
| GCP GKE Autopilot preview-env loop (base 13/13, `pr-999` lifecycle, guards, teardown) | [mldp7](gcp-gke-autopilot-preview-loop-mldp7-20260915.md) | CI-generated workflow live run; multi-region previews |
| AWS EKS Auto Mode Magento KEEP (`queueMode:rabbitmq`, search disabled), destroy | [awsek](aws-eks-auto-mode-packed-keep-awsek-20260823.md) | Certified EKS; managed-node-groups / self-managed / EKS Fargate; OpenSearch |
| AWS EKS self-managed Magento 2.4.9, ELB HTTP, teardown | [20260817bf](aws-eks-self-managed-magento-live-20260817bf.md) | Node loss, zone loss, edge, EKS as a certified target |
| GCP GKE Standard Magento 2.4.9 with Memorystore Valkey 9.0 | [20260815](gcp-gke-standard-magento-valkey90-live-20260815.md) | Public HTTP/TLS, HA, certified target |
| GCP GKE Standard HA Magento known-content (pod/node/zone loss) | [gcha36](gcp-gke-ha-standard-magento-live-gcha36-20260820.md) | Physical zone outage, regional DR, certified target |
| Scaleway Kapsule infrastructure-only | [2026-09-15](scaleway-kapsule-infrastructure-live-2026-09-15.md) | Magento runtime (`not-run`), HA |
| OVH MKS infrastructure-only | [2026-08-12](ovh-mks-infrastructure-live-2026-08-12.md) | Magento runtime (`not-run`), HA |
| AWS CloudFront Magento-safe WAF | [2026-08-13](aws-cloudfront-magento-safe-waf-live-2026-08-13.md) | EKS edge |
| AWS CloudFront alias HTTPS | [2026-08-13](aws-cloudfront-alias-https-live-2026-08-13.md) | Magento origin |
| GCP URL-map failover HTTPS | [20260817bl](gcp-edge-failover-https-live-20260817bl.md) | Magento origin, Armor, failback |
| Fastly adapter TLS/purge | [2026-08-13](fastly-adapter-live-2026-08-13.md) | Magento origin, Magento edge certification |
| New Relic EU operations | [2026-08-13](newrelic-operations-live-2026-08-13.md) | Provider collector composition |
| AWS VPC+RDS brownfield adopt | [2026-08-02](aws-brownfield-adopt-2026-08-02.md) | Cloud SQL attach |
| GCP Cloud SQL known-row restore | [2026-08-15](gcp-cloudsql-application-fixture-live-2026-08-15.md) | Regional DR |
| AWS Aurora known-row restore | [2026-08-17](aws-aurora-application-fixture-live-2026-08-17.md) | HA/DR |

## Withheld

| Why | Proof |
| --- | --- |
| Cloud Armor GA import drops Magento `requestBodiesToExclude` | [20260813af](gcp-edge-armor-attempt-20260813af.md) |
| AWS X-Ray Magento traces | typed unsupported: no `ObservabilityAdapter` registers X-Ray; YAML `nativeProvider: xray` is typed unavailable. An EKS IAM snippet is not a Magento cell. |
| Schema-mismatch rollback refuses when Magento epochs differ | [gcap28 leftover](gcp-gke-autopilot-schema-mismatch-gcap28-20260820.md) |

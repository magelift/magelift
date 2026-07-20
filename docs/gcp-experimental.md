# Experimental GCP target (GKE Autopilot)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
AWS ECS Fargate remains the only certified v1 path.

## Target

```yaml
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: your-gcp-project-id
    region: europe-west1
    networkCidr: 10.20.0.0/16
    zones: [europe-west1-b, europe-west1-c]   # HA needs 3 zones
    imageDigest: ghcr.io/org/magento@sha256:...
    encryptionKeySecret: magento-crypt-key
```

## Magento capability map (production-shaped)

| Magento need | Preview | Standard / HA |
| --- | --- | --- |
| Network | VPC + NAT, 2 zones | Same; HA uses 3 zones |
| MySQL | Cloud SQL zonal | Cloud SQL **REGIONAL** HA + backups |
| Valkey | Memorystore 0 replicas | 1 / 2 replicas |
| Search | OpenSearch on GKE (1) | OpenSearch on GKE (1 / 3) |
| Queue | Magento DB queue | RabbitMQ on GKE |
| Media | GCS versioned bucket | Same |
| Edge | optional Armor | Cloud Armor policy |
| Compute | GKE Autopilot web/cron | + queue consumers; replicas 2 / 3 |
| Migrate | GKE Job via Magento Ops | Same |

Magento env contracts (`platform.CoreEnvBindings`, migration shell) stay in core.
GCP only adapts products.

## Offline verification

- Pulumi mocks: `go test ./internal/cloud/gcp/...` (preview / standard / HA graphs)
- Floci is **AWS-only** today — there is no floci-gcp. Do not invent GCP emulator coverage;
  use mocks + short-lived real GCP acceptance.

## Real cloud

See [gcp-acceptance.md](gcp-acceptance.md). Use a **pullable** image digest for `up`
(placeholder digests fail ImagePull). Destroy + assert_clean always.

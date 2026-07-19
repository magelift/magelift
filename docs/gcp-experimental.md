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
    region: europe-west1          # optional; falls back to defaults.region
    networkCidr: 10.20.0.0/16    # optional
    zones: [europe-west1-b, europe-west1-c]
    imageDigest: ghcr.io/org/magento@sha256:...
    encryptionKeySecret: magento-crypt-key   # Secret Manager secret id
```

## What this stack provisions

| Magento need | GCP product |
| --- | --- |
| Network | VPC, private/public subnets, Cloud Router + NAT |
| MySQL | Cloud SQL MySQL 8 (private IP) |
| Valkey | Memorystore for Valkey |
| Compute | GKE Autopilot (web, cron, deploy Job, optional queue) |
| Ingress | Kubernetes Service `LoadBalancer` (no Cloud CDN yet) |

Deferred: search, RabbitMQ, media/CDN, WIF/OIDC bootstrap, GCS DIY state, Floci-gcp,
candidate deploy orchestration.

## Architecture seam

Portable Magento ports live in `internal/platform` (`StackModule`, output keys,
env bindings). GCP code under `internal/cloud/gcp` is an adapter only — it does not
share Pulumi resource types with AWS.

## Verification

- Unit/mock: `go test ./internal/cloud/gcp/... ./internal/platform/...`
- Real preview against a billed project: use [Local GCP acceptance](gcp-acceptance.md)
  (`MAGELIFT_GCP_ACCEPTANCE=1`, destroy on exit). Do not leave Autopilot / Cloud SQL /
  Memorystore running.

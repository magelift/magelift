# Experimental OVHcloud target (Managed Kubernetes)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
AWS ECS Fargate remains the only certified v1 path. Validated with Pulumi
`WithMocks` graph tests — no OVH account required for CI.

## Target

```yaml
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: your-ovh-public-cloud-project-id
    region: GRA9
    networkCidr: 10.30.0.0/16
    zones: [GRA9]
    imageDigest: ghcr.io/org/magento@sha256:...
    encryptionKeySecret: magento-crypt-key
```

## What this stack provisions

| Magento need | OVH product |
| --- | --- |
| Network | Private network + subnet V2 |
| MySQL | Managed Databases for MySQL |
| Valkey | Managed Databases for Valkey |
| Compute | Managed Kubernetes (MKS) + node pool |
| Ingress | Kubernetes Service `LoadBalancer` |

Deferred: search, RabbitMQ, media/CDN, bootstrap/state/secrets day-2, Magento
candidate deploy orchestration.

## Verification

- Unit/mock: `go test ./internal/cloud/ovh/...`
- Real account acceptance is not wired yet.

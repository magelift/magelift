# Experimental Scaleway target (Kapsule)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
AWS ECS Fargate remains the only certified v1 path. Validated with Pulumi
`WithMocks` graph tests — no Scaleway account required for CI.

## Target

```yaml
target:
  provider: scaleway
  runtime: kapsule
  scaleway:
    projectId: 11111111-1111-1111-1111-111111111111
    region: fr-par
    zone: fr-par-1
    networkCidr: 10.40.0.0/16
    cacheMode: redis          # escape hatch: Scaleway has no managed Valkey yet
    imageDigest: ghcr.io/org/magento@sha256:...
    encryptionKeySecret: magento-crypt-key
```

## What this stack provisions

| Magento need | Scaleway product |
| --- | --- |
| Network | VPC + Private Network |
| MySQL | Managed Database for MySQL |
| Cache | Managed Redis (`cacheMode: redis` escape hatch) |
| Compute | Kubernetes Kapsule + pool |
| Ingress | Kubernetes Service `LoadBalancer` |

**Cache caveat:** Adobe Commerce 2.4.9+ prefers Valkey. Scaleway offers Managed Redis
only; MageLift documents `cacheMode: redis` explicitly rather than pretending Valkey
exists. Wire-compatible for older Magento lines; reassess when Scaleway ships Valkey.

**Provider caveat:** Pulumi package is community `pulumiverse/pulumi-scaleway` (pinned
in `go.mod`), not an official Scaleway-owned provider.

Deferred: search, RabbitMQ, Edge Services/CDN, bootstrap/state/secrets day-2, Magento
candidate deploy orchestration.

## Verification

- Unit/mock: `go test ./internal/cloud/scaleway/...`
- Real account acceptance is not wired yet.

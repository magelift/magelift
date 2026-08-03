# Experimental Scaleway target (Kapsule)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
Certified paths remain AWS ECS Fargate and GCP GKE Autopilot. Validated with Pulumi
`WithMocks` graph tests, so CI needs no Scaleway account.

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

Deferred: search, RabbitMQ, Edge Services/CDN, Bootstrap/Secrets/Cost day-2,
DNS/managed dump (Phase 7). Magento deploy Steps exist offline via shared
`kube.Steps`. Not live-acceptance certified.

**Day-2:** Observe (logs/exec/health), Magento deploy Steps, and DIY State /
`AcquireLock` work offline (unit/Floci). Bootstrap, Secrets, and Cost remain
`ErrNotSupported` (tier-named). Experimental shared kube surface; not certified.
See [capability matrix](capability-matrix.md).


## Verification

- Unit/mock: `go test ./internal/cloud/scaleway/...`
- Real account acceptance is not wired yet.

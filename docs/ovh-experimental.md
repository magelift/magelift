# Experimental OVHcloud target (Managed Kubernetes)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
Certified paths remain AWS ECS Fargate and GCP GKE Autopilot. Validated with Pulumi
`WithMocks` graph tests, so CI needs no OVH account.

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

Deferred: search, RabbitMQ, media/CDN, Bootstrap/Secrets/Cost day-2, DNS/managed
dump (Phase 7). Magento deploy Steps exist offline via shared `kube.Steps`. Not
live-acceptance certified.

**Day-2:** Observe (logs/exec/health), Magento deploy Steps, and DIY State /
`AcquireLock` work offline (unit/Floci). Bootstrap, Secrets, and Cost remain
`ErrNotSupported` (tier-named). Experimental shared kube surface; not certified.
See [capability matrix](capability-matrix.md).


## Verification

- Unit/mock: `go test ./internal/cloud/ovh/...`
- Real account acceptance is not wired yet.

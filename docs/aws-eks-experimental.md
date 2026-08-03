# Experimental AWS target (EKS Autopilot / Auto Mode)

Status: **experimental** (ADR 0007 / ADR 0008). Not Magento-acceptance certified.
AWS ECS Fargate remains the only certified v1 path. See the product matrix in
[architecture.md](architecture.md#aws-magento-product-matrix).

## Target

```yaml
target:
  provider: aws
  runtime: eks-autopilot
  aws:
    # Same account inputs as ECS (KMS, secrets, VPC CIDR, AZs, image digest).
    catalog:
      eks:
        cpuRequest: 500m
        memoryRequest: 1Gi
        desiredWebReplicas: 1
      # Do not set catalog.fargate CPU/memory on this runtime.
      databaseEngine: rds-mysql   # preview-friendly
      searchMode: disabled        # deferred on EKS
```

DIY stack name: `project-env-aws-eks-autopilot`.

## What this stack provisions

| Magento need | AWS product |
| --- | --- |
| Network | VPC, private/public/data subnets, NAT (`natMode`) |
| MySQL | Aurora MySQL or RDS MySQL (preview) |
| Valkey | ElastiCache Valkey |
| Compute | EKS Auto Mode (web + cron Deployments, migrate Job, optional queue) |
| Ingress | Kubernetes Service `LoadBalancer` (no CloudFront/WAF yet) |

Deferred (same honesty as GCP experimental): OpenSearch, Amazon MQ, media/CDN,
Synthetics, Magento candidate deploy orchestration, kube logs/exec.

## Day-2 ports

Bootstrap, DIY state locks, and Secrets Manager reuse the certified AWS packages.
`HasOps` Magento deploy steps and `HasRuntimeObserve` return `ErrNotSupported`
until phase 3.

## Architecture seam

Portable Magento ports stay in `internal/platform`. Runtime lives in
`internal/cloud/aws/eks`; the `StackModule` is `internal/cloud/aws/eksops`.
There is no shared Kubernetes package with GCP.

## Verification

- Unit/mock: `go test ./internal/cloud/aws/eks/... ./internal/cloud/aws/eksops/...`
- Preview against a real account only when you intend to pay for EKS Auto Mode +
  RDS/ElastiCache; destroy on exit. Prefer Floci + mocks for day-to-day work.

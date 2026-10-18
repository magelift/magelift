# Experimental AWS target (EKS Autopilot / Auto Mode)

Status: **experimental** ([ADR 0002](adr/0002-certified-vs-experimental.md)). The infrastructure and shared
Kubernetes workload graph are wired. Packed KEEP `awsek` ran Magento 2.4.9 on
Auto Mode with RabbitMQ, then destroy. This target is not Magento-acceptance
certified. Certified paths remain AWS ECS Fargate and GCP GKE Autopilot. See the product matrix in
[architecture.md](architecture.md#aws-magento-product-matrix).

## Target

```yaml
target:
  provider: aws
  runtime: eks
  aws:
    # Same account inputs as ECS (KMS, secrets, VPC CIDR, AZs, image digest).
    catalog:
      eks:
        kubernetesVersion: "1.36"
        cpuRequest: 500m
        memoryRequest: 1Gi
        desiredWebReplicas: 1
        searchMode: disabled    # use opensearch for standard/HA search cells
        queueMode: database     # use rabbitmq for standard/HA broker cells
      # Do not set catalog.fargate CPU/memory on this runtime.
      databaseEngine: rds-mysql   # preview-friendly
```

DIY stack name: `project-env-aws-eks`.

## What this stack provisions

| Magento need | AWS product |
| --- | --- |
| Network | VPC, private/public/data subnets, NAT (`natMode`) |
| MySQL | Aurora MySQL or RDS MySQL (preview) |
| Valkey | ElastiCache Valkey |
| Compute | EKS Auto Mode (web + cron Deployments, migrate Job, optional queue) |
| Ingress | Kubernetes Service `LoadBalancer` (no CloudFront/WAF yet) |

The EKS runtime can deploy the shared in-cluster OpenSearch and RabbitMQ
workloads, inject their Magento endpoints, and expose the shared Kubernetes
candidate/runtime day-2 ports. The paid warm session covered database and
RabbitMQ queue transitions, OpenSearch reindex, search removal, runtime health,
and exact teardown. High availability, Varnish, Amazon MQ, media/CDN, external
HTTPS, and synthetics evidence are still deferred. See
[the warm matrix evidence](evidence/aws-eks-self-managed-magento-live-20260817bf.md).

## Day-2 ports

Bootstrap, DIY state locks, and Secrets Manager reuse the certified AWS packages.
`HasOps` Magento deploy steps and `HasRuntimeObserve` use the shared Kubernetes
adapter. They remain experimental until broader EKS account runs prove the
required release, managed-service, HA, edge, migration, health, logs, and
cleanup paths.

## Architecture seam

Portable Magento ports stay in `internal/platform`. Runtime lives in
`internal/cloud/aws/eks`; the `StackModule` is `internal/cloud/aws/eksops`.
The provider-specific EKS topology reuses the provider-neutral Kubernetes
workload components for OpenSearch and RabbitMQ with AWS-specific Pulumi type
tokens, so existing GCP state identities are not changed.

## Verification

- Unit/mock: `go test ./internal/cloud/aws/eks/... ./internal/cloud/aws/eksops/...`
- Preview against a real account only when you intend to pay for EKS Auto Mode +
  RDS/ElastiCache; destroy on exit. Prefer Floci + mocks for day-to-day work.
- Paid warm matrix: use `scripts/aws-acceptance-local.sh` with the EKS profile,
  a stable `MAGELIFT_AWS_ACCEPTANCE_DIR`, and `KEEP=true` until all compatible
  cells finish. Disable `KEEP` for the final teardown and cleanup assertion.

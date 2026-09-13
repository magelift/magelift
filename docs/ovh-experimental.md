# Experimental OVHcloud target (Managed Kubernetes)

Catalog ownership: OpenSpec `certification-ovh`. Certified subset is empty.
`ovh` / `mks` stays experimental. Bootstrap and Secrets remain
`ErrNotSupported`. Magento runtime is `not-run` until a Magento-compatible
digest is exercised; current proof is [infrastructure-only](evidence/ovh-mks-infrastructure-live-2026-08-12.md).
Live bar is at most one MKS preview when credits allow, then destroy. No KEEP.

Status: **experimental** ([ADR 0002](adr/0002-certified-vs-experimental.md)). Not Magento-acceptance certified.
Certified paths remain AWS ECS Fargate and GCP GKE Autopilot. Validated with Pulumi
`WithMocks` graph tests, so CI needs no OVH account.

## Target

```yaml
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: your-ovh-public-cloud-project-id
    apiEndpoint: ovh-eu
    region: EU-WEST-PAR
    networkCidr: 10.30.0.0/16
    zones: [eu-west-par-a, eu-west-par-b, eu-west-par-c]
    imageDigest: ghcr.io/org/magento@sha256:...
    encryptionKeySecret: magento-crypt-key
```

`zones` are MKS availability-zone identifiers inside the selected OVH region;
they are not additional database regions. A Free MKS cluster uses one zone. A
Standard multi-zone cluster creates one worker pool per configured zone and
requires at least one worker per zone; MageLift distributes `nodeCount`
deterministically across those pools. Managed Database nodes remain in the
selected managed-database region, where OVH provides the 1-AZ or 3-AZ service
shape; select a documented 3-AZ region and a multi-node plan when zonal database
resilience is required. See OVHcloud's [MKS plan guide](https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/mks-plans),
[MKS node-pool API guide](https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/managing-nodes),
and [managed-database deployment-mode guide](https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison)
for the provider's current regional availability.

The current managed-service defaults are MySQL 8.4 and Valkey 8.1 on the
current Gen 3 `b3-8` node flavor. The official Valkey capability page lists
7.2, 8.0, and 8.1; the authenticated EU-WEST-PAR availability catalog also
currently returns 9.0 and 9.1. The preview preset uses the current
availability-admitted Discovery one-node shape; standard and high-availability
use the current Production two-node shape. Advanced YAML may select the
documented or catalog-admitted discovery/essential, business/production, or
enterprise/advanced plan family, exact flavor, version, node count, backup
settings, and regions.
The optional `apiEndpoint` accepts `ovh-eu`, `ovh-ca`, or `ovh-us` and defaults
to `ovh-eu`. The provider availability matrix remains authoritative for the
selected region, network type, and account.

Before any Pulumi backend or child resource is created, MageLift performs
read-only OVH admission in this order:

1. Read the selected Public Cloud region and available zones.
2. Reject an unavailable region or an MKS plan that is not admitted in that
   region.
3. Resolve omitted zones to one provider zone for preview/standard or all
   provider zones for high availability; validate explicit zones against the
   provider response.
4. Read `/cloud/project/{serviceName}/database/availability` and match the
   selected MySQL and Valkey engine, version, plan, flavor, private-network
   boundary, region, and node count.
5. Revalidate the resulting plan and only then allow Pulumi to register paid
   resources.

If a capability endpoint is unavailable or a selected combination is not
admitted, the plan fails closed. The destroy path intentionally bypasses this
read-only admission so a temporary provider capability outage cannot block
cleanup. See OVHcloud's [database getting-started and availability
guide](https://docs.ovhcloud.com/en/guides/public-cloud/databases/getting-started)
and [MKS regional availability guide](https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors).

## What this stack provisions

| Magento need | OVH product |
| --- | --- |
| Network | Private network + subnet V2 |
| MySQL | Managed Databases for MySQL |
| Valkey | Managed Databases for Valkey |
| Compute | Managed Kubernetes (MKS) + node pool |
| Ingress | Kubernetes Service `LoadBalancer` |

Deferred: search, RabbitMQ, media/CDN, Bootstrap/Secrets, live OVH pricing, and
DNS/managed dump (Phase 7). Magento deploy Steps exist offline via shared
`kube.Steps`. Not live-acceptance certified.

**Day-2:** Observe (logs/exec/health), Magento deploy Steps, and DIY State /
`AcquireLock` work offline (unit/Floci). Bootstrap and Secrets remain
`ErrNotSupported` (tier-named). `magelift cost` reports account-free MKS,
managed MySQL, Valkey, load-balancer, and queue capacity; `magelift cost --live`
fails explicitly until OVH pricing is implemented. Experimental shared
kube surface; not certified. See [capability matrix](capability-matrix.md).


## Verification

- Unit/mock: `go test ./internal/cloud/ovh/...`
- Offline harness: `MAGELIFT_ACCEPTANCE_DRY_RUN=1 ./scripts/ovh-acceptance-local.sh`
- Live infrastructure smoke: `MAGELIFT_K8S_ACCEPTANCE=1 ./scripts/ovh-acceptance-local.sh`

The live command requires a pullable immutable image, an exact unique resource
prefix, and an OVH project ID. It accepts one configured service shape at a
time, uses disposable local Pulumi state, runs `deploy --infra-only`, destroys
the Pulumi stack on exit, and polls the OVH MKS, managed database, and load
balancer APIs for resources with that prefix. By default it proves
infrastructure lifecycle only and records that runtime health was not
exercised. Set `MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH=true` only when the
digest is a Magento-compatible image; a generic infrastructure smoke image is
not sufficient for the runtime gate. Provider login/bootstrap and DIY object
state are not implemented yet, so this is not Magento deploy or release-signing
certification. When Magento runtime health is enabled, or when
`MAGELIFT_CERTIFICATE_IDENTITY` (or `MAGELIFT_OVH_CERTIFICATE_IDENTITY`) is set,
the harness signs if a Cosign identity-token source is configured and promotes
the digest before `deploy --infra-only`. The full shared Kubernetes deploy path is
covered offline.

Live runs have a six-hour disposable TTL by default. Set
`MAGELIFT_ACCEPTANCE_TTL_SECONDS` to a shorter value when the selected shape is
known to converge faster (the guard accepts at most 24 hours). TTL expiry forces
teardown even when debug retention was requested; direct provider inventories
remain the cleanup authority.

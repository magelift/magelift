# Experimental Scaleway target (Kapsule)

Catalog ownership: OpenSpec `certification-scaleway`. Certified subset is
empty. `scaleway` / `kapsule` stays experimental. Bootstrap remains
`ErrNotSupported`. Cache family is Redis (`cacheMode: redis`); do not relabel
it Valkey. Magento runtime is `not-run` until a Magento-compatible digest is
exercised inside the $50 own-money cap; current proof is
[infrastructure-only](evidence/scaleway-kapsule-infrastructure-live-2026-08-13.md).
Live bar is at most one Kapsule preview, then destroy. No KEEP. Vendors attach
to a GCP origin.

Status: **experimental** ([ADR 0002](adr/0002-certified-vs-experimental.md)). Not Magento-acceptance certified.
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
    zone: fr-par-1              # primary managed-cache zone
    zones: [fr-par-1, fr-par-2] # one Kapsule pool per listed zone
    networkCidr: 10.40.0.0/16
    cacheMode: redis          # escape hatch: Scaleway has no managed Valkey yet
    redisVersion: 8.6.3       # query the provider catalog before changing this
    kapsuleVersion: 1.36.1    # query the provider catalog before changing this
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

Deferred: search, RabbitMQ, Edge Services/CDN, Bootstrap and live Cost
pricing, DNS/managed dump (Phase 7). Magento deploy Steps exist offline via shared
`kube.Steps`. Not live-acceptance certified.

**Day-2:** Observe (logs/exec/health), Magento deploy Steps, DIY State /
`AcquireLock`, provider-backed application Secrets, and account-free Cost work
offline (unit/Floci). Bootstrap remains `ErrNotSupported`; application Secrets
use the planned project/region and ownership tags, while live CRUD/IAM evidence
is still pending. `magelift cost --live` fails explicitly until Scaleway live
pricing is implemented. Experimental shared kube surface; not certified.
See [capability matrix](capability-matrix.md).

### Availability-zone shape

`zone` is the primary zone used by the managed Redis adapter. `zones` controls the
Kapsule worker placement. When more than one zone is configured, MageLift creates
one node pool per zone and distributes `nodeCount` across those pools; `nodeCount`
must be at least the number of configured zones. Web and queue Deployments receive
strict Kubernetes zone-spread constraints when their replica count can cover all
configured zones. This keeps the YAML setting connected to the actual worker and
workload topology instead of treating it as metadata.

The preview preset may use one zone. Standard requires at least two configured
zones, and high-availability requires at least three; the planner rejects a
smaller list before Pulumi creates anything.

Scaleway's current guidance recommends multiple pools for multi-AZ Kapsule and
documents important control-plane, gateway, and zone-local persistent-volume
limitations. Read the [official multi-AZ Kapsule guidance](https://www.scaleway.com/en/docs/kubernetes/reference-content/multi-az-clusters/)
before selecting a production topology. Live multi-AZ failure, backup/restore,
and Magento runtime certification remain release-gated.


## Verification

- Unit/mock: `go test ./internal/cloud/scaleway/...`
- Offline harness: `MAGELIFT_ACCEPTANCE_DRY_RUN=1 ./scripts/scaleway-acceptance-local.sh`
- Live infrastructure smoke: `MAGELIFT_K8S_ACCEPTANCE=1 ./scripts/scaleway-acceptance-local.sh`

The live command requires a pullable immutable image, an exact unique resource
prefix, and a Scaleway project ID. It accepts one configured service shape at a
time, uses disposable local Pulumi state, runs `deploy --infra-only`, destroys
the Pulumi stack on exit, and polls Kapsule, managed database, Redis, private
network, and load balancer resources for that prefix. By default it proves
infrastructure lifecycle only and records that runtime health was not
exercised. Set `MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH=true` only when the
digest is a Magento-compatible image; the generic NGINX smoke image is
deliberately insufficient for that gate. Provider login/bootstrap and DIY
object state are not implemented yet, so this is not Magento deploy or
release-signing certification. When Magento runtime health is enabled, or when
`MAGELIFT_CERTIFICATE_IDENTITY` (or `MAGELIFT_SCALEWAY_CERTIFICATE_IDENTITY`) is
set, the harness signs if a Cosign identity-token source is configured and
promotes the digest before `deploy --infra-only`. The full shared Kubernetes
deploy path is covered offline. See
[scaleway-kapsule-infrastructure-live-2026-08-13.md](evidence/scaleway-kapsule-infrastructure-live-2026-08-13.md).

Live runs have a six-hour disposable TTL by default. Set
`MAGELIFT_ACCEPTANCE_TTL_SECONDS` to a shorter value when the selected shape is
known to converge faster (the guard accepts at most 24 hours). TTL expiry forces
teardown even when debug retention was requested; direct provider inventories
remain the cleanup authority.

# GCP target (GKE Autopilot and Standard)

Status: **certified** for `gcp` / `gke-autopilot` on evidenced Magento cells;
`gcp` / `gke-standard` is experimental
(see [gcp-acceptance.md](gcp-acceptance.md), [capability-matrix.md](capability-matrix.md),
and the [evidence pack](evidence/README.md)).
AWS ECS Fargate and GCP GKE Autopilot are the two certified first-party Magento
origins ([ADR 0002](adr/0002-certified-vs-experimental.md)).

**Still experimental / not certified:** GKE Standard as a release-wide target,
hosted GitHub Actions WIF, the complete Valkey 9
runtime/cache boundary, and community providers. The adapter now selects
Memorystore `VALKEY_9_0` (GA) for Adobe Commerce 2.4.9 by default and accepts
`VALKEY_9_1` only as an explicit Preview profile. A bounded real-account
`VALKEY_9_1` Preview provisioning cell passed on 2026-08-11 and was cleaned;
it proves the current provider version mapping, not the complete runtime,
backup, HA, or DR claim. Current 2.4.9 Standard application evidence uses
Memorystore `VALKEY_9_0`; see
[20260815](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md).
The Autopilot preview path has a current Magento pass
([gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md));
the remaining service and release matrix is still open.

## Day-2 status

| Surface | Status |
| --- | --- |
| Infra preview/deploy | **certified** (preview create-once) |
| Magento candidate migrate | **certified** (`deploy:candidate` cell) |
| Bootstrap | state bucket + WIF (pool/provider/CI SA; live STS/impersonation evidenced) |
| Secrets / Composer SM | `gcp-secret-manager://` via AccessSecretVersion (**certified**) |
| logs / exec / health | **certified for Autopilot evidenced cells**; Standard shapes have bounded live exercise and stay experimental |

See [capability matrix](capability-matrix.md).


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
| Compute | GKE Autopilot web/cron | GKE Standard for node-level settings; + queue consumers; replicas 2 / 3 |
| Migrate | GKE Job via Magento Ops | Same |

Magento env contracts (`platform.CoreEnvBindings`, migration shell) stay in core.
GCP only adapts products.

The three-node OpenSearch HA shape needs the Linux
`vm.max_map_count=262144` sysctl. The GKE Standard node pool applies it before
the workload starts. GKE Autopilot intentionally restricts unsafe sysctls and
privileged initialization, so HA selects Standard; see Google's
[Autopilot security measures](https://cloud.google.com/kubernetes-engine/docs/concepts/autopilot-security)
and [Standard node system configuration](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/node-system-config).

## GitHub WIF

Bootstrap `Ensure` provisions a workload identity pool, GitHub OIDC provider
(`https://token.actions.githubusercontent.com`), attribute condition
`assertion.repository == 'magelift/magelift'`, and a CI service account with
`roles/iam.workloadIdentityUser`. Details expose `wif.provider` / `wif.serviceAccount`
(never `"deferred"`). Do not commit a service-account key.

This repository does not run that token exchange. The hosted workflow is
`magelift.yml` in
[acceptance-magento-249](https://github.com/magelift/acceptance-magento-249).
A local acceptance cell can impersonate the CI service account with `gcloud`.
That checks IAM on the account. It does not present a token signed by
`token.actions.githubusercontent.com`.

## Offline verification

- Pulumi mocks: `go test ./providers/gcp/...` (preview / standard / HA graphs)
- floci-gcp (`make floci-gcp-test`): digest-pinned `floci/floci-gcp:0.11.0` on port 4588. Closes GCS, Secret Manager, Pub/Sub publish, Cloud Logging write/list, and Cloud Monitoring time-series write/list against the official Go clients without a GCP project. Does **not** certify Autopilot, Memorystore Valkey 9.0, Cloud Armor, Google-managed TLS, or Magento Cloud SQL PITR.
- WIF unit proof: `go test ./providers/gcp/bootstrap/ -count=1 -run 'WIF|Identity'`
- Composer SM: `go test ./providers/gcp/secrets/ ./internal/cli/ -count=1 -run 'Secret|Composer|GCP'`

## Real cloud

See [gcp-acceptance.md](gcp-acceptance.md). Use a **pullable** image digest for `up`
(placeholder digests fail ImagePull). Destroy + assert_clean always.

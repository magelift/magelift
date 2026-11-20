# Google Cloud runtimes

The GCP plugin supports GKE Autopilot and experimental GKE Standard. The August 2026 runs exercised Magento 2.4.9, a migration job, Secret Manager, logs, exec and health checks on specific environments. The Standard run used Memorystore Valkey 9.0. A separate run provisioned a 9.1 preview instance. The records list the versions, operations and teardown results for each run. [Autopilot run](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md) · [Standard run](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md) · [Capability guide](capability-matrix.md)

## Stack operations

The plugin manages state in Cloud Storage and can provision a Workload Identity Federation pool, provider and CI service account. It reads `gcp-secret-manager://` references through Secret Manager. Tests cover planning, migration jobs and Kubernetes workload operations. Hosted GitHub identity exchange and recovery depend on the identity and services configured in the shop’s account. [Bootstrap](bootstrap.md) · [Provider support](capability-matrix.md)

## Target

Use this array for the `target` entry in `magelift.php`.

```php
$target = [
    'provider' => 'gcp',
    'runtime' => 'gke-autopilot',
    'gcp' => [
        'project' => 'your-gcp-project-id',
        'region' => 'europe-west1',
        'networkCidr' => '10.20.0.0/16',
        'zones' => ['europe-west1-b', 'europe-west1-c'],
        'imageDigest' => 'ghcr.io/org/magento@sha256:...',
        'encryptionKeySecret' => 'magento-crypt-key',
    ],
];
```

## Service settings by preset

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

## GitHub identity

Bootstrap can create a Workload Identity Federation pool and separate
GitHub build and deploy service accounts. Review the permission plan and
bind it to the shop repository. The build identity needs registry access.
The deploy identity needs the roles for the resources MageLift uses.
[Configure GitHub for Google Cloud](bootstrap.md) · [Staging walkthrough](onboarding.md)

## Offline verification

- Pulumi mocks: `go test ./providers/gcp/...` (preview / standard / HA graphs)
- `make floci-gcp-test` starts the digest-pinned GCP emulator on port 4588 and exercises Cloud Storage, Secret Manager, Pub/Sub, Logging and Monitoring with the Go clients.
- WIF unit proof: `go test ./providers/gcp/bootstrap/ -count=1 -run 'WIF|Identity'`
- Composer SM: `go test ./providers/gcp/secrets/ ./internal/cli/ -count=1 -run 'Secret|Composer|GCP'`

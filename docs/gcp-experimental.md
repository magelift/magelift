# GCP target (GKE Autopilot)

Status: **certified** for `gcp` / `gke-autopilot` on the maintainer acceptance path
(2026-08-02 create-once — evidence `.magelift/gcp-matrix/matrix-results.md`;
see [gcp-acceptance.md](gcp-acceptance.md) and [capability-matrix.md](capability-matrix.md)).
AWS ECS Fargate and GCP GKE Autopilot are the two certified first-party targets
(ADR 0007 multi-cloud gate).

**Still experimental / not certified:** non-preview presets (`standard` /
`high-availability` apply spend), hosted GitHub Actions WIF (Act-only until
minutes return), and community providers.

## Day-2 honesty

| Surface | Status |
| --- | --- |
| Infra preview/deploy | **certified** (preview create-once) |
| Magento candidate migrate | **certified** (`deploy:candidate` cell) |
| Bootstrap | state bucket + WIF (pool/provider/CI SA; live STS/impersonation evidenced) |
| Secrets / Composer SM | `gcp-secret-manager://` via AccessSecretVersion (**certified**) |
| logs / exec / health | **certified** on Autopilot (kubectl + BindOutputs) |

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
| Compute | GKE Autopilot web/cron | + queue consumers; replicas 2 / 3 |
| Migrate | GKE Job via Magento Ops | Same |

Magento env contracts (`platform.CoreEnvBindings`, migration shell) stay in core.
GCP only adapts products.

## GitHub WIF (Act-only until minutes return)

Bootstrap `Ensure` provisions a workload identity pool, GitHub OIDC provider
(`https://token.actions.githubusercontent.com`), attribute condition
`assertion.repository == 'acourtiol/magelift'`, and a CI service account with
`roles/iam.workloadIdentityUser`. Details expose `wif.provider` / `wif.serviceAccount`
(never `"deferred"`).

Hosted Actions minutes are exhausted — matrix evidence stays **Act-only** until
minutes return. Do not enable this workflow as a required hosted check.

### Run via Act

After bootstrap, pass the provider resource name and SA email (WIF names only —
never `credentials_json` or SA JSON keys):

```bash
act -W .github/workflows/gcp-wif-act-smoke.yml workflow_dispatch \
  -s GCP_WORKLOAD_IDENTITY_PROVIDER='projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL/providers/github' \
  -s GCP_SERVICE_ACCOUNT='ml-PROJECT-ENV-ci@GCP_PROJECT.iam.gserviceaccount.com'
```

Workflow: `.github/workflows/gcp-wif-act-smoke.yml` in the repository
(`google-github-actions/auth`, `permissions.id-token: write`).

### Fallback: documented gcloud STS exchange

Without Act, prove federation with ADC + IAM Credentials / STS (no SA key file):

```bash
# 1) Obtain a GitHub OIDC-shaped subject is CI-only; locally use gcloud print-identity-token
#    against a workload that already federates, or exchange after Act once.
# 2) Generate an access token for the CI SA via WIF (example shape):
gcloud iam service-accounts get-iam-policy \
  "ml-shop-preview-ci@${GCP_PROJECT}.iam.gserviceaccount.com"
# Confirm principalSet member for attribute.repository/acourtiol/magelift exists.
# 3) Token mint (when a valid federated credential is available):
gcloud auth print-access-token --impersonate-service-account \
  "ml-shop-preview-ci@${GCP_PROJECT}.iam.gserviceaccount.com"
```

Record the matrix row as Act-only (or `gcloud` STS dry-run) until hosted minutes return.

## Offline verification

- Pulumi mocks: `go test ./internal/cloud/gcp/...` (preview / standard / HA graphs)
- Floci is **AWS-only** today — there is no floci-gcp. Do not invent GCP emulator coverage;
  use mocks + short-lived real GCP acceptance.
- WIF unit proof: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/bootstrap/ -count=1 -run 'WIF|Identity'`
- Composer SM: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/secrets/ ./internal/cli/ -count=1 -run 'Secret|Composer|GCP'`

## Real cloud

See [gcp-acceptance.md](gcp-acceptance.md). Use a **pullable** image digest for `up`
(placeholder digests fail ImagePull). Destroy + assert_clean always.

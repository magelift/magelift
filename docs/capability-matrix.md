# Capability matrix

Presets (`preview`, `standard`, `high-availability`) keep day-one YAML small.
Power users set explicit catalog fields. Only cells marked **certified** are
production-supported on the acceptance path. Experimental cells may change or
return `ErrNotSupported` on day-2 commands. Unsupported cells fail before mutate
unless `compatibility.allowUnsupported` records the risk.

Headless in MageLift means Magento `application.mode: headless|integrated`
(API, CORS, media, edge). Storefront frameworks (Next.js, PWA, etc.) stay outside
the CLI; wire them with Magento outputs and your usual frontend deploy tool.

## Providers and runtimes

| Provider | Runtime | Tier | Magento deploy Ops | Day-2 (logs/exec/secrets/state) |
| --- | --- | --- | --- | --- |
| `aws` | `ecs-fargate` | **certified** | full | full |
| `aws` | `eks-autopilot` | experimental | shared kube.Steps offline (unit) | Observe+Steps+State offline (unit/Floci); Bootstrap/Secrets AWS-wired; Cost unsupported |
| `gcp` | `gke-autopilot` | experimental | partial | partial (bootstrap = state bucket; WIF deferred) |
| `ovh` | `mks` | experimental | shared kube.Steps offline (unit) | Observe+Steps+State offline (unit/Floci); Bootstrap/Secrets/Cost `ErrNotSupported` |
| `scaleway` | `kapsule` | experimental | shared kube.Steps offline (unit) | Observe+Steps+State offline (unit/Floci); Bootstrap/Secrets/Cost `ErrNotSupported` |

On targets without Magento deploy Ops, `magelift deploy` refuses unless you pass
`--infra-only` (infrastructure graph update only). With the flag, Magento migrate,
cutover, and health are skipped and announced on stderr.

Multi-cloud is not claimed until two first-party targets are **certified**
([ADR 0007](adr/0007-multi-provider-community-targets.md)).

## AWS ECS Fargate catalog cells

| Capability | Config | Certified | Experimental / notes |
| --- | --- | --- | --- |
| NAT | `target.aws.natMode` | `nat-gateway`, `fck-nat` (cost/preview) | — |
| Database | `catalog.databaseEngine` | `aurora-mysql`, `rds-mysql` | — |
| Search | `catalog.searchMode` | `disabled` (preview / free-tier), `serverless` (preview), `provisioned` | Wiring certified offline + prior Chantelle Terraform OpenSearch ops ([source](sources/chantelle-opensearch.md)); MageLift live SigV4 data-plane deferred to paid acceptance |
| Cache | managed Valkey | certified shapes per preset | — |
| Queue | `catalog.queueMode` | `db` (preview default), `amazon-mq`, `ecs-rabbitmq` | `ecs-artemis` experimental; EKS queue deferred |
| Web runtime | `application.webRuntime` | `nginx-fpm`, `frankenphp-classic` | FrankenPHP worker reserved |
| Edge | CloudFront + WAF | certified on Fargate | deferred on EKS |
| Compute launch | Fargate (default) | **certified** | ECS Managed Instances (MI) — post-beta cost path; Chantelle uses MI + RI for RabbitMQ/web density |

Fargate remains the agency-friendly default (no capacity planning). **ECS Managed Instances**
are the cost-friendly path when you can commit to RI/Savings Plans and operate node
lifecycle — same Magento task graphs, different capacity provider. Do not claim MI until
a certified acceptance cell exists; until then document it as roadmap only.

### Queue modes (AWS)

| Mode | What it provisions | Default when |
| --- | --- | --- |
| `db` | Magento database-backed messaging (no broker) | `preview` preset if unset |
| `amazon-mq` | Amazon MQ for RabbitMQ (CLUSTER_MULTI_AZ) | `standard` / `high-availability` if unset |
| `ecs-rabbitmq` | RabbitMQ container on ECS Fargate (single task today) | explicit |
| `ecs-artemis` | ActiveMQ Artemis container on ECS | explicit (experimental) |

Amazon MQ is often the expensive cell (~reference: ~$150/mo for small managed HA). Prefer
`ecs-rabbitmq` when cost matters. Prior Terraform Magento-on-AWS work at Chantelle
runs **self-hosted RabbitMQ quorum on ECS Managed Instances** (1 node staging /
3 nodes prod, EFS + AMQPS + serial rolling) — MageLift’s Fargate cell is the cheap
single-node cousin, not that HA ladder yet.

`magelift cost` reflects `databaseEngine`, `searchMode`, and `queueMode` via the
AWS `CostEstimator` adapter. Other providers need their own adapters before cost
is claimed there.

### Free-tier / disposable acceptance (what to run)

Keep one preview stack up and iterate cells; destroy once at the end.

| Cell | Free-tier? | Evidence tier | Notes |
| --- | --- | --- | --- |
| `natMode: fck-nat` | yes | Pulumi mocks | Prefer over NAT Gateway |
| `databaseEngine: rds-mysql` | yes | real-account acceptance | Prior free-tier create measured 2026-07-21 |
| `databaseEngine: aurora-mysql` | **unverifiable** | — | See [Unverifiable on maintainer accounts](#unverifiable-on-maintainer-accounts) |
| `searchMode: disabled` | yes | real-account acceptance | Certified free-tier cell; OpenSearch spend avoided |
| `queueMode: db` | yes | real-account acceptance | Default preview; harness create-once + cell PASS 2026-07-29 |
| `queueMode: ecs-rabbitmq` | yes | real-account acceptance | Same stack cell-update 2026-07-29 (Cloud Map SRV + queue SG HTTPS egress) |
| `queueMode: ecs-artemis` | yes (experimental) | real-account acceptance | Same stack cell-update 2026-07-29; experimental broker image |
| `queueMode: amazon-mq` | **no** | — | Skip apply on disposable accounts; see unverifiable |
| OpenSearch serverless/provisioned | **avoid apply** | Pulumi mocks | Preview-only on free-tier; live SigV4 = unverifiable here |
| `webRuntime: frankenphp-classic` | needs matching image digest | Pulumi mocks | Not a free YAML toggle alone |

Measured first create (2026-07-21, eu-north-1 free-tier): **~10m31s** Pulumi apply for
`preview` + `rds-mysql` + `fck-nat` + `searchMode=disabled` + `queueMode=db`.
Harness re-measure (2026-07-29, same account/shape): **~10m13s** create-once, then
cell updates `db` 255s / `ecs-rabbitmq` 254s / `ecs-artemis` 204s (destroy clean).

### Unverifiable on maintainer accounts

These cells must not be marked certified or real-account-accepted on free-tier /
disposable maintainer accounts. Reasons are specific (TRUST-04):

| Cell | Why unverifiable |
| --- | --- |
| Aurora `CreateDBCluster` | Free-tier API block: disposable accounts often cannot call `CreateDBCluster`; do not claim Aurora apply proof from free-tier acceptance |
| `amazon-mq` × `preview` | Preview preset is 2-AZ while Amazon MQ `CLUSTER_MULTI_AZ` requires 3 AZs — plan is incompatible by design; skip apply on disposable accounts |
| OpenSearch SigV4 data-plane | Live SigV4 search data-plane is deferred paid / not free-tier certifiable; Pulumi graph mocks do not prove SigV4 query auth |

AOSS / OpenSearch Serverless OCU cost ceilings remain empirical/unpublished maintainer
notes (Phase 1 honesty) — do not treat OCU quotes as certified AWS policy.

## Evidence tiers

| Tier | Proves |
| --- | --- |
| Pulumi mocks | Composition / resource graph |
| Floci | Selected AWS API contracts without an account |
| real-account acceptance (`scripts/aws-acceptance-local.sh`) | Real account path with destroy + `assert_clean` |

**Honesty rule (TRUST-03):** a catalog cell's evidence tier must not exceed recorded
evidence. Mocks alone do not certify a matrix cell for production use. Do not claim
real-account acceptance without harness / matrix-results evidence.

## Day-2 port coverage

Checked-in map of AWS ECS day-2 ports → offline vs paid evidence (ACCEPT-06 foundation;
Floci gap closure is a later plan). Prefer under-claim.

| Port | Evidence | Notes |
| --- | --- | --- |
| Bootstrap (`VerifyAccount` / `Ensure`) | paid-only | Live OIDC / account bootstrap not certified by Floci |
| State (`Status` / `Lock` / `Unlock` / `Backup` / `Restore`) | Floci | `TestBootstrapStateAndLockAgainstFloci` |
| Secrets (`List` / `Set` / `Remove`) | Floci | `TestSecretsManagerAgainstFloci` |
| RuntimeObserve.TailLogs | Floci | `TestCloudWatchLogsAgainstFloci` |
| RuntimeObserve.CheckRuntime | Floci / paid-only | `TestECSRuntimeHealthAgainstFloci`; ALB+ECS live health remains paid |
| RuntimeObserve.PrepareExec | unit-fake / paid-only | `TestPrepareExecRejectsDeployWorkload`; live ECS ExecuteCommand is paid-only |
| Ops.AcquireLock | Floci / unit-fake | Covered by `TestBootstrapStateAndLockAgainstFloci` (S3 DIY lock) |
| Ops.NewDeploySteps | unit-fake / paid-only | `TestNewDeployStepsRejectsWrongBackend`; live Magento cutover paid |
| Media / storage (S3 paths exercised by Floci) | Floci | `tests/floci/storage_test.go` (`TestVersionedMediaRestoreAgainstFloci`); `env media-sync` listing-diff: unit (`internal/mediasync`) + `tests/floci/media_sync_test.go` (`TestMediaSyncListingDiffAgainstFloci`). Live paid-account media cutover remains unpaid-proof (Phase 7 HUMAN_GATE with DNS). |
| SelectTask (PrepareExec helper) | unit-fake | `TestSelectTaskSortsRunningTaskARNs` in `internal/cloud/aws/operations` |

Shared Kubernetes day-2 (`eks-autopilot` / `mks` / `kapsule`, experimental): `kube.Observe` +
`kube.Steps` + S3-compatible DIY State/AcquireLock are proven offline (unit + Floci AES256
where applicable). Do not claim live GKE/OVH/SCW Magento acceptance. DNS and managed dump
cutover stay Phase 7.

## GCP GKE Autopilot cells (experimental)

Cells are **preset-derived** (no AWS-style YAML catalog toggles for queue/search):

| Preset | Queue | Search | SQL | Memorystore | Apply on credits? |
| --- | --- | --- | --- | --- | --- |
| `preview` | `database` | OpenSearch on GKE (1) | ZONAL | `SHARED_CORE_NANO` | **yes** (credit-efficient) |
| `standard` | `rabbitmq` on GKE | OpenSearch (1) | REGIONAL | `STANDARD_SMALL` +1 replica | **preview-only** unless budgeted |
| `high-availability` | `rabbitmq`×2 | OpenSearch×3 | REGIONAL | 2 replicas | **preview-only** |

Measured preview create (2026-07-21, `digital-lab-341608` / europe-west1, prefix `mlgcpmx`):
**~19m21s** Pulumi Duration for infra-only. Evidence: `.magelift/gcp-matrix/matrix-results.md`.
Destroy often needs PSA soak + `force_clean` after Cloud SQL (see gcp-acceptance.md).

## Community / out-of-tree modules

Out-of-tree modules register through a custom binary
(see `examples/custom-cli` in the repo, [ADR 0007](adr/0007-multi-provider-community-targets.md)).
No unsigned auto-download plugin loader in v1.

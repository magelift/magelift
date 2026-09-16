# Capability matrix

Presets (`preview`, `standard`, `high-availability`) keep day-one YAML small.
Power users set explicit catalog fields. Only cells marked **certified** are
production-supported on the acceptance path.

Adobe-unsupported combinations fail before mutate unless
`compatibility.allowUnsupported` records that Adobe hatch. The hatch does not
recertify a cell and does not apply to MageLift-experimental or
provider-unavailable targets. Experimental cells warn and continue; the warning
names MageLift. They may still return `ErrNotSupported` on some day-2 commands.
Provider-unavailable cells fail closed.

Headless in MageLift means Magento `application.mode: headless|integrated`
(API, CORS, media, edge). Storefront frameworks (Next.js, PWA, etc.) stay outside
the CLI; wire them with Magento outputs and your usual frontend deploy tool.

OpenSpec `certification-matrix` holds the shared evidence rules. Implemented
cells belong in the per-provider catalogs below. Tracking those cells only
inside a packed multi-provider campaign is not enough.

| Catalog spec | Certified subset today | Experimental, withheld, or named gap until tuple evidence |
| --- | --- | --- |
| `certification-aws` | ECS Fargate preview Magento (`nginx-fpm`) with the recorded catalog services | `managed-instances`, `eks`, `aurora-mysql`, `amazon-mq`, Magento search data-plane (`serverless` / `provisioned`), HA, FrankenPHP, Apache, X-Ray |
| `certification-gcp` | GKE Autopilot evidenced runtime cells | `gke-standard`, HA, Cloud Armor Magento `requestBodiesToExclude`, Cloud SQL attach, Pub/Sub Magento modules |
| `certification-ovh` | empty | every `mks` cell; Bootstrap/Secrets stay `ErrNotSupported` |
| `certification-scaleway` | empty | every `kapsule` cell; Bootstrap stays `ErrNotSupported`; cache family is Redis |

## Providers and runtimes

| Provider | Runtime | Tier | Magento deploy Ops | Day-2 (logs/exec/secrets/state) |
| --- | --- | --- | --- | --- |
| `aws` | `ecs-fargate` | **certified for the evidenced preview tuple** | Magento 2.4.9, `nginx-fpm`, and its recorded catalog services only | full for that exact tuple |
| `aws` | `eks` | experimental | 2.4.9 Auto Mode Magento + RabbitMQ KEEP `awsek`; self-managed Magento [20260817bf](evidence/aws-eks-self-managed-magento-live-20260817bf.md); HA, edge, OpenSearch, and broader release matrix pending | Runtime health/state exercised live; account-free cost classification now covers the EKS control plane, Auto Mode, managed nodes, self-managed nodes, and Fargate; live EKS Price List queries, broader Observe/Steps, and day-2 certification remain pending |
| `gcp` | `gke-autopilot` | **certified for evidenced runtime cells** | 2.4.6-p15 is topology/runtime evidence only because GCP Cloud SQL MySQL is Adobe-unsupported for that patch; 2.4.9 preview has live MySQL 8.4 evidence but remains experimental for the full Adobe cache intersection | full (WIF + Secret Manager Composer + logs/exec/secrets/state/health) for the evidenced runtime cells |
| `gcp` | `gke-standard` | experimental | 2.4.6-p15 HA is topology/runtime evidence only; 2.4.9 Standard and HA have bounded application evidence with MySQL 8.4 and Valkey 9.0, while broader architecture and resilience evidence remain open | full for the evidenced runtime shapes; broader matrix pending |
| `ovh` | `mks` | experimental | shared kube.Steps offline (unit) | Observe+Steps+State offline (unit/Floci); Bootstrap/Secrets `ErrNotSupported`; account-free Cost capacity is supported, live OVH pricing is not wired |
| `scaleway` | `kapsule` | experimental | shared kube.Steps offline (unit) | Observe+Steps+State offline (unit/Floci); Bootstrap `ErrNotSupported`; provider-backed application Secrets use the planned project/region and ownership tags; account-free Cost capacity is supported, live Scaleway pricing is not wired |

On targets without Magento deploy Ops, `magelift deploy` refuses unless you pass
`--infra-only` (infrastructure graph update only). With the flag, Magento migrate,
cutover, and health are skipped and announced on stderr.

Multi-cloud is claimed for the two certified first-party targets
(`aws`/`ecs-fargate` and `gcp`/`gke-autopilot`) per
([ADR 0002](adr/0002-certified-vs-experimental.md)). Community providers
remain experimental.

## AWS implemented catalog (`certification-aws`)

Adobe / MageLift / certified are separate columns. An omitted YAML combination
must fail closed or be typed unavailable. Private Magento-on-AWS shops may
illustrate one wiring; they do not certify a cell.

| Dimension | YAML | Adobe | MageLift | Status |
| --- | --- | --- | --- | --- |
| Runtime | `ecs-fargate` | n/a (infra) | implemented | **certified** for the evidenced preview tuple |
| Runtime | `eks` | n/a | implemented | experimental |
| Fargate compute | `fargate` (default) | n/a | implemented | **certified** on the preview tuple |
| Fargate compute | `fargate-spot` | n/a | implemented | experimental |
| Fargate compute | `ec2-asg` | n/a | implemented | experimental |
| Fargate compute | `managed-instances` | n/a | implemented | experimental (bounded Magento 2.4.9 KEEP `awsmi`) |
| EKS compute | `auto-mode` | n/a | implemented | experimental (bounded Magento 2.4.9 KEEP `awsek`) |
| EKS compute | `managed-node-groups` | n/a | implemented | experimental |
| EKS compute | `self-managed` | n/a | implemented | experimental (bounded Magento 2.4.9 evidence) |
| EKS compute | `fargate` | n/a | implemented | experimental |
| Database | `rds-mysql` | Adobe-listed 8.0 / 8.4 | implemented | **certified** on Fargate preview |
| Database | `aurora-mysql` | Adobe-gated Aurora 3.11 / 3.12 | implemented | experimental; unaimed Aurora 8.4 fails closed; a private shop on 8.4 is an example, not the pin |
| Database | `rds-mariadb` | Adobe-listed majors | implemented | experimental |
| Search (Fargate) | `disabled` | Magento can run without a dedicated search service | implemented | **certified** preview / free-tier |
| Search (Fargate) | `serverless` | Magento cannot SigV4 | implemented (AOSS + sidecar) | experimental |
| Search (Fargate) | `provisioned` | Magento HTTPS 443 in-VPC | implemented (no SigV4 sidecar) | experimental |
| Search (EKS) | `opensearch` / `disabled` | same Magento search contract | implemented | experimental |
| Queue (Fargate) | `db` | Magento database messaging | implemented | **certified** preview default |
| Queue (Fargate) | `ecs-rabbitmq` | RabbitMQ | implemented | **certified** as unset standard/HA default |
| Queue (Fargate) | `amazon-mq` | managed RabbitMQ | implemented | experimental-warn |
| Queue (Fargate) | `ecs-artemis` | no Adobe default | implemented | experimental |
| Queue (EKS) | `database` / `rabbitmq` | Magento messaging | implemented | experimental (bounded Magento 2.4.9 KEEP `awsek`) |
| NAT | `nat-gateway` / `fck-nat` | n/a | implemented | certified shapes per the NAT row below |
| Web runtime | `nginx-fpm` | Adobe lists nginx on this Magento line | implemented | **certified** on the Fargate preview tuple |
| Web runtime | `frankenphp-classic` / `php-apache` | no Adobe row | hatch plugins | experimental |
| Web runtime | `frankenphp-worker` | n/a | unregistered | unavailable |
| Edge | CloudFront + WAF | Magento-safe WAF | implemented | certified on Fargate; deferred on EKS |
| Observability | CloudWatch | n/a | implemented | first-party; not an X-Ray cell |
| Observability | X-Ray | n/a | plugin required | typed unavailable without an `ObservabilityAdapter` |

RDS and Aurora both inject the writer endpoint and Secrets Manager
username/password JSON. `searchMode: provisioned` is HTTPS 443 in-VPC with no
signing sidecar; `searchMode: serverless` keeps the SigV4 sidecar.

## AWS ECS Fargate catalog cells

| Capability | Config | Certified | Experimental / notes |
| --- | --- | --- | --- |
| NAT | `target.aws.natMode` + `target.aws.natTopology` + `target.aws.natReplacementMode` + optional `target.aws.natInstanceType` | `nat-gateway`, or `fck-nat` with explicit `single-az`/`multi-az` topology, `none`/`auto-scaling` replacement semantics, and ARM64 Graviton sizing (cost/preview and HA boundaries remain evidence-gated) | - |
| Database | `catalog.databaseEngine` | `rds-mysql` | `aurora-mysql` (unverifiable on free-tier; not certified apply) |
| Search | `catalog.searchMode` | `disabled` (preview / free-tier) | `serverless` and provisioned OpenSearch remain experimental. Wiring is covered offline, but MageLift has no certified live Magento search data-plane evidence. |
| Cache | managed Valkey | certified shapes per preset | - |
| Queue | `catalog.queueMode` | `db` (preview default), `ecs-rabbitmq` (unset standard/HA default) | Explicit `amazon-mq` is experimental-warn; `ecs-artemis` has live preview evidence but remains experimental; EKS database/RabbitMQ KEEP `awsek` exists, while Amazon MQ and full EKS certification remain deferred |
| Web runtime | `application.webRuntime` | `nginx-fpm` on the evidenced AWS preview tuple | Adobe lists nginx on this Magento line. `frankenphp-classic` and `php-apache` are Adobe-unsupported plugins that require `compatibility.allowUnsupported`; neither has an Adobe row or certification. `frankenphp-worker` is unregistered. |
| Edge | CloudFront + WAF | certified on Fargate | deferred on EKS |
| Compute launch | Fargate (default) | **certified** | ECS Managed Instances (MI), post-beta cost path; prior Magento-on-AWS ops used MI + RI for RabbitMQ/web density |

Fargate remains the agency-friendly default (no capacity planning). **ECS Managed Instances**
are the cost-friendly path when you can commit to RI/Savings Plans and operate node
lifecycle. Same Magento task graphs, different capacity provider. Do not claim MI until
a certified acceptance cell exists; until then document it as roadmap only.

### Queue modes (AWS)

| Mode | What it provisions | Default when |
| --- | --- | --- |
| `db` | Magento database-backed messaging (no broker) | `preview` preset if unset |
| `ecs-rabbitmq` | RabbitMQ container on ECS Fargate (single task today) | `standard` / `high-availability` if unset |
| `amazon-mq` | Amazon MQ for RabbitMQ (CLUSTER_MULTI_AZ) | explicit (experimental-warn) |
| `ecs-artemis` | ActiveMQ Artemis container on ECS | explicit (experimental) |

Amazon MQ is often the expensive cell (~reference: ~$150/mo for small managed HA). Prefer
`ecs-rabbitmq` when cost matters. Prior Magento-on-AWS reference ops used **self-hosted
RabbitMQ quorum on ECS Managed Instances** (1 node staging / 3 nodes prod, EFS + AMQPS +
serial rolling). MageLift's Fargate cell is the cheap single-node cousin, not that HA
ladder yet.

`magelift cost` reflects `databaseEngine`, `searchMode`, and `queueMode` via the
AWS `CostEstimator` adapter. Account-free mode also classifies EKS control-plane,
compute-mode, and in-cluster workload capacity; current live AWS Price List
queries remain implemented only for ECS capacity. Other providers need their own
adapters before cost is claimed there.

### Free-tier / disposable acceptance (what to run)

Keep one preview stack up and iterate cells; destroy once at the end.

| Cell | Free-tier? | Evidence tier | Notes |
| --- | --- | --- | --- |
| `natMode: fck-nat` | yes | Pulumi mocks; live egress/repair evidence pending | Prefer for cost-sensitive single-AZ or explicitly modeled multi-AZ paths; `auto-scaling` uses stable ENIs; do not infer HA from resource count |
| `databaseEngine: rds-mysql` | yes | real-account acceptance | Prior free-tier create measured 2026-07-21 |
| `databaseEngine: aurora-mysql` | **unverifiable** | - | See [Unverifiable on maintainer accounts](#unverifiable-on-maintainer-accounts) |
| `searchMode: disabled` | yes | real-account acceptance | Certified free-tier cell; OpenSearch spend avoided |
| `queueMode: db` | yes | real-account acceptance | Default preview; harness create-once + cell PASS 2026-07-29 |
| `queueMode: ecs-rabbitmq` | yes | real-account acceptance | Same stack cell-update 2026-07-29 (Cloud Map SRV + queue SG HTTPS egress) |
| `queueMode: ecs-artemis` | yes (experimental) | real-account acceptance | Same stack cell-update 2026-08-06; still experimental because this is one preview shape and not a HA certification |
| `queueMode: amazon-mq` | **no** | - | Skip apply on disposable accounts; see unverifiable. Explicit `amazon-mq` remains experimental-warn; unset standard/HA YAML now selects `ecs-rabbitmq`. |
| OpenSearch serverless/provisioned | **avoid apply** | Pulumi mocks | Preview-only on free-tier; live SigV4 = unverifiable here |

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
| `amazon-mq` × `preview` | Preview preset is 2-AZ while Amazon MQ `CLUSTER_MULTI_AZ` requires 3 AZs. Plan is incompatible by design; skip apply on disposable accounts |
| OpenSearch SigV4 data-plane | Live SigV4 search data-plane is deferred paid / not free-tier certifiable; Pulumi graph mocks do not prove SigV4 query auth |

AOSS / OpenSearch Serverless OCU cost ceilings remain empirical/unpublished maintainer
notes (Phase 1). Do not treat OCU quotes as certified AWS policy.

## Evidence tiers

| Tier | Proves |
| --- | --- |
| Pulumi mocks | Composition / resource graph |
| Floci | Selected AWS API contracts without an account |
| real-account acceptance (`scripts/aws-acceptance-local.sh`) | Real account path with destroy + `assert_clean` |

A catalog cell's evidence tier must not exceed recorded evidence (TRUST-03).
Mocks alone do not certify a matrix cell for production use. Do not claim
real-account acceptance without harness / matrix-results evidence.

## OVH implemented catalog (`certification-ovh`)

Certified subset: empty. `ovh` / `mks` stays experimental. Magento is not
certified by unit tests, Floci, or account-free cost. The live bar is one MKS
preview when credits allow, then destroy; KEEP is forbidden. Magento runtime
stays `not-run` until a Magento-compatible digest is exercised. Current proof
is infrastructure-only.

Gateway, floating IP, and CNI failures are separate cells. Do not infer custom
gateway routing from `privateNetworkRoutingAsDefault`.

| Dimension | YAML | Adobe | MageLift | Status |
| --- | --- | --- | --- | --- |
| Runtime | `mks` | n/a | implemented | experimental |
| MKS plan | `mksPlan`: `free` / `standard` | n/a | implemented | experimental |
| Nodes | `nodeFlavor`, `nodeCount`, CPU/memory requests, `desiredWebReplicas`, `queueConsumerCount` | n/a | implemented | experimental |
| MySQL | `databasePlan` (`discovery`/`essential`/`business`/`production`/`enterprise`/`advanced`), `databaseVersion` `8.0`/`8.4`, node count, backup | Adobe MySQL row per patch | implemented | experimental |
| Valkey | `valkeyPlan`, `valkeyVersion` `7.2`/`8.0`/`8.1`/`9.0`/`9.1`, node count, backup | Adobe Valkey row per patch | implemented | experimental |
| Network | `attachFloatingIps`, `privateNetworkRoutingAsDefault` (DHCP gateway; custom gateway not inferred) | n/a | implemented | experimental; split from CNI |
| Logs | opaque `nativeReference` to Logs Data Platform | n/a | implemented (audit stream only) | experimental; not Magento log delivery |
| Day-2 | Bootstrap, Secrets | n/a | `ErrNotSupported` | unavailable |

## Scaleway implemented catalog (`certification-scaleway`)

Certified subset: empty. `scaleway` / `kapsule` stays experimental. Magento is
not certified by account-free cost or kube.Steps. Cache family is Redis
(`cacheMode: redis`); do not relabel it Valkey. The live bar is one Kapsule
preview inside the $50 own-money cap shared with Cloudflare, SendGrid, Fastly,
and New Relic; destroy always; no KEEP. Vendors attach to a GCP origin.
Magento runtime stays `not-run` if that cap is spent elsewhere. Current proof
is infrastructure-only.

| Dimension | YAML | Adobe | MageLift | Status |
| --- | --- | --- | --- | --- |
| Runtime | `kapsule` | n/a | implemented | experimental |
| Kapsule | `kapsuleVersion`, `nodeType`, `nodeCount` | n/a | implemented | experimental |
| Replicas | `desiredWebReplicas`, `queueConsumerCount` | n/a | implemented | experimental |
| RDB | `databaseNodeType`, `databaseHighAvailability`, backup, `databaseEncryptionAtRest` | Adobe MySQL row per patch | implemented | experimental |
| Cache | `cacheMode: redis`, `redisVersion`, `redisClusterSize` (1 standalone, 2 HA, 3–6 cluster) | Adobe prefers Valkey; Scaleway has Redis | implemented | experimental; mismatch stays visible |
| Cockpit | first-party log/metric/trace sources | n/a | implemented | experimental; sources are not Magento delivery evidence |
| Day-2 | Bootstrap | n/a | `ErrNotSupported` | unavailable |
| Day-2 | application Secrets | n/a | planned project/region tags | experimental; live CRUD pending |

## Day-2 port coverage

Checked-in map of AWS ECS day-2 ports → offline vs paid evidence (ACCEPT-06 foundation;
Floci gap closure is a later plan). Prefer under-claim.

| Port | Evidence | Notes |
| --- | --- | --- |
| Bootstrap (`VerifyAccount` / `Ensure`) | Floci IAM OIDC API; live GitHub token exchange | `TestIAMOpenIDConnectProviderAgainstFloci` covers Create/Get/Tag. Account `Ensure` and GitHub Actions OIDC remain paid-only |
| State (`Status` / `Lock` / `Unlock` / `Backup` / `Restore`) | Floci | `TestBootstrapStateAndLockAgainstFloci` |
| Secrets (`List` / `Set` / `Remove`) | Floci | `TestSecretsManagerAgainstFloci` |
| RuntimeObserve.TailLogs | Floci | `TestCloudWatchLogsAgainstFloci` |
| RuntimeObserve.CheckRuntime | Floci / paid-only | `TestECSRuntimeHealthAgainstFloci`; ALB+ECS live health remains paid |
| RuntimeObserve.PrepareExec | unit-fake / paid-only | `TestPrepareExecRejectsDeployWorkload`; live ECS ExecuteCommand is paid-only |
| Ops.AcquireLock | Floci / unit-fake | Covered by `TestBootstrapStateAndLockAgainstFloci` (S3 DIY lock) |
| Ops.NewDeploySteps | unit-fake / paid-only | `TestNewDeployStepsRejectsWrongBackend`; live Magento cutover paid |
| Media / storage (S3 paths exercised by Floci) | Floci | `tests/floci/storage_test.go` (`TestVersionedMediaRestoreAgainstFloci`); `env media-sync` listing-diff: unit (`internal/mediasync`) + `tests/floci/media_sync_test.go` (`TestMediaSyncListingDiffAgainstFloci`). Live paid-account media cutover remains unpaid-proof (paid follow-up gate with DNS). |
| SelectTask (PrepareExec helper) | unit-fake | `TestSelectTaskSortsRunningTaskARNs` in `internal/cloud/aws/operations` |

Shared Kubernetes day-2 (`eks` / `mks` / `kapsule`, experimental): `kube.Observe` +
`kube.Steps` + S3-compatible DIY State/AcquireLock are proven offline (unit + Floci AES256
where applicable). Do not claim live EKS/OVH/SCW Magento acceptance. DNS and managed dump
cutover stay Phase 7.

## AWS EKS Auto Mode cells

EKS is a managed-Kubernetes target with the shared Magento workload graph wired.
Packed KEEP `awsek` ran Magento 2.4.9 on Auto Mode with a database queue, then
in-cluster RabbitMQ, with search disabled; Magento HTTP 200; destroy plus
`assert_clean`. See [awsek](evidence/aws-eks-auto-mode-packed-keep-awsek-20260823.md).
The target remains experimental until HA, secured Valkey, external HTTPS/edge,
broader release coverage, and full day-2 certification pass. Preview defaults to
database queues and no search service; standard selects one-node OpenSearch and
RabbitMQ, while high availability selects three OpenSearch replicas, two
RabbitMQ replicas, and two consumers. Self-managed Magento evidence remains
[20260817bf](evidence/aws-eks-self-managed-magento-live-20260817bf.md).
`kubernetesVersion` defaults to the current verified AWS minor and is explicit in
the plan so a matrix run cannot silently move to another cluster API version.

## GCP implemented catalog (`certification-gcp`)

Presets (`preview` / `standard` / `high-availability`) fill omitted
`target.gcp` fields. The agency API is the explicit YAML dimensions, not the
preset name. Autopilot certified status does not transfer to Standard, HA,
Armor Magento exclusion, Cloud SQL attach, or Pub/Sub Magento modules.

| Dimension | YAML | Adobe | MageLift | Status |
| --- | --- | --- | --- | --- |
| Runtime | `gke-autopilot` | n/a | implemented | **certified** for evidenced runtime cells |
| Runtime | `gke-standard` | n/a | implemented | experimental |
| Cloud SQL availability | `ZONAL` / `REGIONAL` | Cloud SQL MySQL intersection is Adobe-gated per patch | implemented | Autopilot preview zonal is the certified runtime path; regional/HA stay experimental |
| Cloud SQL backup / PITR | `cloudSqlBackupEnabled`, binary log, retention, start time, location | n/a | implemented | experimental except as recorded on the Autopilot tuple |
| Memorystore | engine `VALKEY_8_0` / `VALKEY_9_0` / `VALKEY_9_1`; mode `CLUSTER` / `CLUSTER_DISABLED`; zone `MULTI_ZONE` / `SINGLE_ZONE` | Adobe Valkey row per Magento patch | implemented | evidenced 9.0 on current 2.4.9 cells; 9.1 is a bounded provision cell |
| Search | `openSearchMode`: `opensearch` / `disabled` | Magento search | implemented (GKE workload) | Autopilot three-node OpenSearch needs `vm.max_map_count` that Autopilot cannot set; HA OpenSearch stays on Standard |
| Queue | `queueMode`: `database` / `rabbitmq` | Magento messaging | implemented | Autopilot preview uses database; rabbitmq is selectable without switching presets |
| Standard nodes | type/count/min/max, disk, image, `standardNodeSpot` | n/a | implemented | experimental |
| Autopilot requests | `autopilotCpuRequest` / `autopilotMemoryRequest` | n/a | implemented | certified path uses preview requests |
| Armor | `enableCloudArmor` | Magento body exclusion | implemented topology | Magento `requestBodiesToExclude` withheld |
| GKE version | `kubernetesVersion`, `releaseChannel` (`RAPID`/`REGULAR`/`STABLE`) | n/a | implemented | experimental except as recorded on the Autopilot tuple |
| CIDRs | `clusterIpv4Cidr`, `servicesIpv4Cidr` | n/a | implemented | experimental |
| Replicas | `desiredWebReplicas`, `queueConsumerCount` | n/a | implemented | Autopilot preview uses the evidenced replica counts |
| Named gaps | Cloud SQL attach, Pub/Sub Magento modules | n/a | not implemented | fail closed / named; do not inherit Autopilot certified |

## GCP GKE Autopilot and Standard cells

The certified GCP tier (GCP-06) covers Autopilot evidenced runtime cells.
Current proof is the [evidence pack](evidence/README.md): Autopilot Magento
2.4.9 preview [gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md)
(sealed JSONL under `evidence/runs/`). 2.4.6-p15 Standard cells used matching
PHP and Composer contracts but Cloud SQL MySQL is not an Adobe-compatible
database intersection for that patch. See [gcp-acceptance.md](gcp-acceptance.md).

| Preset | Queue | Search | SQL | Memorystore | Apply on credits? |
| --- | --- | --- | --- | --- | --- |
| `preview` | `database` | OpenSearch on GKE (1) | ZONAL | `SHARED_CORE_NANO` | **yes** (credit-efficient; 2.4.9 pass) |
| `standard` | `rabbitmq` on GKE | OpenSearch (1) | REGIONAL | `STANDARD_SMALL` +1 replica | **runtime evidence only for 2.4.6-p15; experimental 2.4.9** |
| `high-availability` | `rabbitmq`×2 | OpenSearch×3 | REGIONAL | 2 replicas | **bounded 2.4.9 application evidence; broader HA/resilience experimental** |

GKE Standard is experimental. 2.4.9 Standard Magento with Memorystore Valkey
9.0 is [20260815](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md)
(not public HTTP/TLS, HA, or a certified target). HA known-content on Standard
is [gcha36](evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md):
pod, node, and backing-VM zone-loss simulation with Magento seed-probe; not
physical zone outage, regional DR, or a certified target. HA defaults to
Standard because three-node OpenSearch needs `vm.max_map_count` that Autopilot
does not provide.

A separate GKE Standard collector path exists as bounded New Relic
logs/metrics/traces evidence. It is collector evidence only and does not
change Magento or GKE architecture certification rows. See
[evidence](evidence/README.md).

The current GCP implementation selects Memorystore `VALKEY_9_0` (GA) for
Adobe's 2.4.9 latest-patch row and supports explicit `VALKEY_9_1` (Preview).
A bounded real-account `VALKEY_9_1` Preview provisioning cell passed on
2026-08-11 and was cleaned, so the version mapping is no longer an untested
provider-create path. That cell does not close the full runtime/cache,
backup, HA, or DR claim. The current 2.4.9 preview, Standard, and HA
application records use Valkey 9.0. The current HA records include one bounded
web-pod replacement cell, but remain application evidence, not physical
zone outage, DR, or full production certification. Current Magento proofs are
in the [evidence pack](evidence/README.md).

Measured preview create (2026-07-21, disposable GCP project / europe-west1):
**~19m21s** Pulumi Duration for infra-only. Certified cell matrix (2026-08-02):
create-once + cells then destroy/`force_clean`/`assert_clean`.
Destroy often needs PSA soak + `force_clean` after Cloud SQL (see gcp-acceptance.md).

## Cross-cutting edge and observability

These capabilities are independent of the cloud runtime. A provider extension
receives typed edge and telemetry intents and owns its credentials, API resources,
outputs, and cleanup.

| Capability | Current status | Evidence / boundary |
| --- | --- | --- |
| Fastly (`internal/external/fastly`) in front of any origin provider | experimental | `magelift edge plan/apply/destroy` and origin-deploy integration are implemented. The adapter uses the current Domain Management CLI, authenticated Fastly profiles, idempotent domain reconciliation, purge policy, and exact ownership state. Live adapter cleanup and routed-domain smoke pass, but complete production edge certification remains open ([evidence](evidence/fastly-adapter-live-2026-08-13.md)) |
| Native provider edge | evidence-gated | AWS CloudFront/WAF is bounded to its Magento-origin evidence. GCP Cloud Armor Magento `requestBodiesToExclude` remains withheld. |
| CloudWatch | first-party AWS adapter | CloudWatch logs are separate from X-Ray. Exact signal coverage and live delivery remain evidence dimensions; an EKS IAM policy does not establish Magento X-Ray evidence. |
| X-Ray | typed unavailable without an `ObservabilityAdapter` plugin | Magento-origin X-Ray is typed unsupported. An EKS IAM snippet is not a Magento X-Ray runtime cell. |
| Google Cloud Operations | first-party GCP adapter | GKE system/workload logging and monitoring plus dashboard/log-presence alert resources; trace instrumentation and live delivery evidence remain explicit open dimensions |
| Cloudflare | DNS cutover only | Cloudflare DNS evidence is not CDN or WAF certification. |
| SES / SendGrid | configuration validation | SMTP settings and secret references do not prove Magento delivery. SendGrid delivery needs Magento-origin evidence; SES stays configuration-only until delivery is evidenced. |
| Scaleway Cockpit | first-party Scaleway adapter | Cockpit log/metric/trace sources with retention; unsupported audit/alert semantics remain visible rather than inferred |
| OVHcloud Logs Data Platform | first-party OVH adapter | MKS audit-log subscription only when an existing stream is supplied through opaque `nativeReference`; workload log/metric/trace delivery remains explicit and open |
| Datadog / New Relic (`internal/external/newrelic`) / OTLP | extension boundary | Typed intents, secret-safe OTLP export, provider-neutral New Relic ECS Contrib and Kubernetes NRDOT mappings for EKS/GKE/Kapsule/MKS, an injectable provider-owned NerdGraph/NRQL marker verifier, a bounded authenticated New Relic CLI marker/NRQL probe, live GKE Autopilot and GKE Standard Contrib collector paths with separate ingest/query credentials and logs/metrics/traces delivery, and one AWS ECS Contrib sidecar exercise with eventual three-signal delivery exist. The AWS verifier budget was exceeded before the trace became queryable, so broader collector lifecycle, credential rotation, native composition, retention/alerting, and provider-wide cleanup evidence remain open |

## Community / out-of-tree modules

Out-of-tree modules register through a custom binary
(see `examples/custom-cli` in the repo, [ADR 0008](adr/0008-provider-load-path.md)).
Unsigned auto-download is refused. Signed, digest-pinned first-party and trusted
community subprocess artifacts are the published-CLI path.

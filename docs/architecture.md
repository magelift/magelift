# Architecture charter

## Product promise

A Magento developer adds `magelift.yaml` to an existing repository and uses one native
CLI to deploy and operate a production-grade environment in their own cloud account.
The supported path is Magelift commands only: `doctor`, `bootstrap`, `deploy`,
`health`, `destroy`. Operators should not need Pulumi, Kubernetes, Cosign, or
GitHub federation commands on a laptop preview.

Presets keep YAML small for agencies and small e-merchants coming from Adobe Commerce
Cloud or Upsun. Power users open catalog escape hatches
([capability matrix](capability-matrix.md)). Headless means Magento
`application.mode: headless|integrated`; storefront frameworks stay external.

MageLift provisions resources in the user's cloud account. It does not proxy cloud
billing or add a resource markup. The user chooses the provider, runtime, managed
services, edge layer, and telemetry destination, subject to the selected target's
compatibility and certification status.

## Boundaries

- The RC1 matrix marks AWS ECS Fargate and GCP GKE Autopilot as certified for
  evidenced cells. GCP GKE Standard is experimental, with current 2.4.6-p15 HA
  and 2.4.9 MySQL 8.4 passes but no release-wide certification yet. AWS EKS
  (`aws` / `eks`), OVH (`ovh` / `mks`), and Scaleway
  (`scaleway` / `kapsule`) remain experimental ([ADR 0002](adr/0002-certified-vs-experimental.md)). Evidence, not
  a descriptor, is the authority for a certification claim.
- Stable interfaces may exist for config, lifecycle, and capabilities;
  experimental targets must be labeled in docs and CLI output.
- The CLI orchestrates; Pulumi owns durable infrastructure.
- The core owns configuration envelopes, command UX, plugin discovery, trust,
  locking, and orchestration policy. Providers own cloud implementation, stack
  execution, and operating capabilities, reached over versioned typed
  operations with explicit negotiation; incompatibility fails closed
  ([ADR 0013](adr/0013-provider-plugin-contract.md)).
- Provider subprocesses run with the user's cloud privileges; the process
  boundary is a deployment and compatibility boundary, not a security sandbox.
- Artifacts are immutable, signed, built once, and promoted by digest.
  `magelift build --push`, `sign`, and `promote` use the operator's current
  cloud or CI login. Cosign keyless identity remains Sigstore OIDC under the
  hood (Actions, Google SA), not Artifact Registry or ECR native signatures.
  The YAML-only path does not require Cosign flags.
- Secrets are references resolved at runtime, never plaintext config.
- Unsupported Magento/service combinations fail before mutate unless an auditable
  override accepts the risk.
- No shared Pulumi components that switch on provider. Each cloud owns topology
  under `internal/cloud/<provider>/` until Order 5 moves providers to nested
  `providers/<name>/` modules per ADR 0013.

## System shape

The control plane is the `magelift` CLI: load typed YAML, resolve an environment,
validate compatibility, then drive build, Pulumi, and ops. On certified AWS the
request path is Route 53, CloudFront, WAF, ALB, and private ECS Fargate. Managed
AWS services hold state; S3 is the media path. GCP maps Magento onto GKE Autopilot
for the smaller path and GKE Standard when a workload needs node-level kernel
settings, alongside Cloud SQL and Memorystore. See [gcp-experimental.md](gcp-experimental.md)
for provider-specific implementation notes.
Magento migrate candidates run as GKE Jobs through the shared `deployflow` port.

The PHP package exposes a lifecycle DAG: validate, build, package, deploy,
post-deploy. The build runner runs the first three without runtime credentials.
Deploy and post-deploy run after connectivity and runtime config injection.
Extensions use stable logical IDs, not raw provider schemas. Normal projects stay
YAML-only.

Artifact manifest creation has a pre-digest prepare step and a post-build finalize
step. The finalized manifest stays outside the image and binds the prepared metadata
to BuildKit's OCI digest. An image may contain pre-digest build metadata, but MageLift
does not rebuild or mutate it to embed the final manifest. See [ADR 0005](adr/0005-external-artifact-manifest.md).

## Portability boundary

The application model, build phases, artifact manifest, and capability requirements
are portable contracts. Network layout, compute resources, managed services, recovery
controls, and cost models belong to a target implementation.

### Cross-cutting edge and telemetry

Edge delivery and observability are separate from the cloud runtime. `edge` and
`observability` in the configuration are typed intents, not vendor credential
containers. Fastly can sit in front of an AWS, GCP, OVHcloud, or Scaleway origin;
CloudFront, Cloud Armor, or another provider edge can remain the native path. The
core refuses an unknown edge provider instead of silently substituting the cloud
provider's CDN.

The public extension boundary receives `sdk.EdgeIntent` and
`sdk.ObservabilityIntent`, plus the optional typed `sdk.ProjectionTarget` inside
`sdk.ResilienceIntent` for cache/search rebuild workloads. An extension owns
provider credentials, VCL or policy artifacts, telemetry exporters, and resource lifecycle;
it does not re-parse YAML or receive raw SDK argument maps. First-party adapters
provision or validate the native graphs for AWS CloudWatch, GCP GKE plus Cloud
Monitoring, Scaleway Cockpit, and the OVHcloud MKS audit stream. Their coverage
is signal-specific and remains explicit in the plan; a destination resource is
not evidence that every requested signal is collected. The registered Fastly
adapter has live API lifecycle and cleanup evidence but remains experimental
until the full routed production edge gate closes. New Relic, Datadog, and OTLP
remain extension-owned; the provider-owned New Relic adapter supports direct
OTLP for generic workloads and maps the provider-neutral NRDOT Kubernetes Helm
boundary to EKS, GKE, Kapsule, and MKS, while the ECS path uses the documented
OpenTelemetry Collector Contrib boundary. The bounded 2026-08-09 New Relic CLI
marker/NRQL probe proves queryable data-plane delivery, and the provider-owned
Go adapter has an offline-tested NerdGraph/NRQL marker verifier. Collector
lifecycle and signal-specific operational evidence remain open.

The web runtime sits on the portable side of this boundary. `nginx-fpm` is the Adobe-aligned default. `frankenphp-classic` and `php-apache` are Adobe-unsupported plugins that require `compatibility.allowUnsupported`; Adobe has no row for either and neither is certified. `frankenphp-worker` is unregistered. The default ECS task uses an nginx HTTP container and a PHP-FPM container built from the same immutable image. Integrated tasks add the pinned Varnish 8.0.2
sidecar on port 6081 and route the load balancer through it to nginx on port 8080.
The Varnish container uses a writable root like the other Magento containers;
its VSM lives at `/tmp/varnish` on the image filesystem (`-n /tmp/varnish`),
not on a Fargate empty volume (those mount root-owned and break Varnish).
Headless tasks route directly to
nginx on port 8080. The selected application mode is passed to the application
container as a runtime contract, so integrated and headless deployments can change
request wiring without changing the public provider boundary.

Certified v1 targets are AWS ECS Fargate and GCP GKE Autopilot. Other runtimes and
clouds implement `sdk` Target contracts and register a `platform.StackModule`.
Each provider keeps capabilities explicit and must pass the shared Magento
acceptance suite before certification. Portable YAML is not a lowest-common-denominator
cloud catalog.

### AWS Magento product matrix

"Full AWS Magento" means the Magento acceptance path on ECS Fargate with explicit
escape hatches, not every AWS SKU.

| Choice | Certified (ECS Fargate) | Experimental / deferred |
| --- | --- | --- |
| Runtime | `ecs-fargate` | `eks` (EKS Auto Mode-shaped; live certification pending) |
| `natMode` | `nat-gateway` (default), `fck-nat` | - |
| `natTopology` | `single-az` or `multi-az`; omitted defaults to single-AZ for preview and multi-AZ for standard/HA | - |
| `natReplacementMode` | `none` or `auto-scaling`; omitted defaults to automatic replacement for multi-AZ fck-nat | - |
| `natInstanceType` | ARM64 Graviton fck-nat size; omitted defaults to cost-optimized `t4g.nano` | x86 instance types are rejected for the first-party ARM64 AMI |
| `databaseEngine` | `rds-mysql` | `aurora-mysql` (experimental-warn; unverifiable on maintainer free-tier). Adobe gate is Aurora 3.11/3.12. Unaimed Aurora 8.4 fails closed. A private shop on Aurora 8.4 is an example, not the certified pin. |
| Managed database durability | RDS backup and maintenance windows, deletion protection, automated-backup deletion policy | Aurora inherits the same YAML knobs but is not a certified apply cell |
| Valkey snapshot durability | ElastiCache snapshot retention and daily snapshot window | `0` explicitly disables automatic snapshots; Valkey remains reconstructible state |
| `searchMode` | `disabled` (preview / free-tier) | `serverless` (AOSS + Magento SigV4 sidecar) and `provisioned` (unsigned HTTPS:443 in-VPC). Both remain experimental until matrix + evidence say otherwise. A private Magento-on-AWS shop may illustrate the provisioned path. It does not certify MageLift. |
| Queue (`catalog.queueMode`) | `db` (preview default), `ecs-rabbitmq` (standard/HA default when unset) | Explicit `amazon-mq` remains experimental-warn. EKS `database` or `rabbitmq` pending live certification. `ecs-artemis` remains experimental. |
| Edge | CloudFront + WAF | deferred on EKS |
| Day-2 ops | deploy/logs/exec via AWS adapters | shared Kubernetes deploy/logs/exec path (live certification pending) |

See [aws-eks-experimental.md](aws-eks-experimental.md) for the EKS path and
[gcp-experimental.md](gcp-experimental.md) for GCP implementation notes.

Evidence tiers differ: Pulumi mocks prove composition, Floci AWS and floci-gcp
prove selected API contracts without an account, and packed live sessions
([certification sessions](certification-sessions.md)) prove Magento on GCP then
AWS. Emulators do not certify Autopilot, Armor, managed TLS, or Magento Cloud
SQL PITR. Mocks alone do not certify a matrix cell for production use.

Coming from Adobe Commerce Cloud or Platform.sh? Start with
[migrating-from-paas.md](migrating-from-paas.md).

See [ADR 0003](adr/0003-portable-contracts-vs-topology.md) and
[ADR 0004](adr/0004-ports-and-adapters.md). Contributor checklist:
[adding-a-provider.md](adding-a-provider.md).

## Repository layout

Provider-neutral code: `internal/platform` (stack modules, Magento output keys,
env bindings), `internal/automation`, `internal/deploy`, `internal/infra` (SDK
extension index), `internal/topology`, and `sdk`. Providers live under
`internal/cloud/<provider>/`.

AWS packages today include `bootstrap`, `secrets`, `target`, `network`,
`security`, `ingress`, `runtime`, `edge`, `database`, `cache`, `search`, `queue`,
`storage`, `observability`, `operations`, `deployment`, `stack`, and `state`.
GCP code mirrors the stack composition boundary under `internal/cloud/gcp/`.

**CLI deploy path:** `platform.ModuleRegistry` selects a `StackModule` by
provider + runtime. Magento lock and candidate steps are optional via
`platform.HasOps`. Day-2 surfaces (bootstrap, state, secrets, logs, exec, runtime
health) resolve the same way through `HasBootstrap` / `HasState` / `HasSecrets` /
`HasRuntimeObserve` ([ADR 0004](adr/0004-ports-and-adapters.md)). DIY stack names
include provider and runtime so targets do not collide.
`internal/infra.Registry` is for Target/Capability/Hook discovery tests; it does
not replace module registration.

The SDK registry indexes targets, capabilities, typed transforms, and lifecycle
hooks by stable IDs. Transforms stay type-checked at the boundary. Hook discovery
is deterministic by phase and ID. The default CLI ships the first-party AWS, GCP,
OVH, and Scaleway modules. Third-party modules use a custom binary that calls
`cli.NewWithExtensions` with an explicit `sdk.Module` ([ADR 0008](adr/0008-provider-load-path.md)).

The AWS stack package composes a validated plan: catalog values from the caller,
Pulumi outputs between capabilities, separate providers for regional vs
CloudFront/WAF resources. Each capability validates typed inputs before register.
The planner checks Magento release against Adobe's AWS service versions.
`allowUnsupported` is the only recorded exception path. An existing-network
reference can supply VPC and subnets; MageLift then skips routing, NAT, and VPC
endpoints for that network.

Standard and high-availability networks create private interface endpoints for ECR,
CloudWatch Logs, Secrets Manager, SSM, and ECS Exec, alongside the S3 gateway endpoint.
Preview keeps only the S3 gateway endpoint to preserve its low-cost profile. Endpoint
security groups allow HTTPS from the VPC CIDR and endpoint private DNS remains enabled.

Production observability also provisions a CloudWatch Synthetics canary with the
current Puppeteer 11 runtime. The canary checks the public HTTPS health endpoint every
five minutes, stores encrypted run artifacts in a versioned private S3 bucket, and
publishes a breaching alarm when two of three five-minute windows are unsuccessful.
Its execution role is limited to that bucket prefix, the configured KMS key, its log
groups, and the CloudWatch Synthetics metric namespace. Preview and non-production
environments do not incur canary resources.

The Go build system follows the same rule. Its packages are grouped below
`internal/build/kit`, `internal/build/pipeline`, `internal/build/plan`, and
`internal/build/runner`. The repository-level `build/` directory remains the
Magento Composer package and is separate from these Go internals.

Infrastructure commands keep seams narrow: config yields a `platform.PlannedStack`
from the selected module; Automation API owns the Pulumi workspace; the CLI owns
environment selection, approvals, provider Ops (lock / Magento steps when present),
and post-update health. Stack backends are swappable in tests so commands can run
without cloud credentials.

ECS web, cron, and queue tasks use the application task role. The one-off migration
candidate uses a separate ECS deployment role, which can be tightened independently
as the Magento lifecycle gains more granular capability declarations. Bootstrap also
creates a recovery-only state role, a repository-scoped GitHub Actions CI role, and
a read-only build role for MageLift-scoped secrets. The state role cannot mutate
infrastructure; generated CI uses the CI role for Pulumi, ECS, and managed-service
changes, while the build job uses the build role.

Magento containers (php-fpm, web/nginx, Varnish, cron, consumers) use a writable
root (`ReadonlyRootFilesystem: false`): Magento needs to write `env.php`,
generated files, and the nginx pid, and Fargate empty volumes mount as
root-owned, so overlaying `/tmp` or `/app/var` would break the non-root
runtime. Only the search-proxy sidecar runs with a read-only root. The image
contains only a non-secret `env.php` scaffold. ECS Secrets Manager selectors inject the managed
database JSON fields and the stable Magento encryption key, while Adobe's
`MAGENTO_DC_*` environment configuration supplies capability endpoints and
credentials at task start.

### Runtime storage (definitive writable set, 2026-09-16)

Baked into the immutable image (read at runtime, never written): application
code, DI output (`setup:di:compile`), and static content for the configured
locales plus themes (`build.staticContent` is required). Written at runtime:
`app/etc/env.php` (generated from Secrets Manager selectors plus
`MAGENTO_DC_*` values), disposable `var/` (cache, page cache, logs, tmp),
and `/tmp` (nginx pid, Varnish VSM at `/tmp/varnish`, probe scratch).
Durable state lives outside the container filesystem: media in buckets,
sessions in Valkey, secrets in Secrets Manager, state in versioned
backups. Writable root is the deliberate alpha answer for this set
(`internal/cloud/aws/runtime/containers.go`); no read-only flip is planned
until a writable-storage design proves itself against these requirements.

### Alpha security review (minimum, 2026-09-16)

Reviewed for the alpha recipe; each item names its evidence or stays explicitly
open. A full security review and penetration test remain outstanding and are
not claimed here.

- Secret references: reviewed. YAML carries references only; validation
  rejects plaintext for Composer credentials, Magento variables, email
  credentials, and edge tokens (`internal/config/config.go`).
- Log redaction: reviewed within its design boundary. Reference-only config
  keeps secret values out of YAML, logs, and evidence; `env dump` warns its
  output is unsanitized unless `--sanitize` (mailbox-address hashing only,
  not certified anonymization) and database passwords travel via environment
  variables unprinted (`internal/cli/env.go`). No global log-scanning
  redactor is claimed.
- Least-privilege permissions: reviewed. Bootstrap mints a state role that
  cannot mutate infrastructure, a repository-scoped CI role for Pulumi and
  service changes, and a read-only build role for MageLift-scoped secrets
  (`internal/cloud/aws/bootstrap`; see the IAM paragraph above).
- Network exposure: reviewed for workload isolation. Workload security groups
  accept ingress only within the VPC CIDR; workloads sit in private subnets
  with NAT egress (`internal/cloud/aws/network/network.go`). The public
  surface is the load balancer by storefront design; a rule-by-rule edge
  audit stays with the full review.
- Artifact trust: reviewed. Images build once and promote by digest; `magelift
  upgrade` verifies the keyless Sigstore bundle for `checksums.txt` then the
  archive SHA-256 before atomic replace (`docs/operations.md`, upgrade
  command). AWS acceptance requires a signed immutable runtime digest with a
  matching Cosign identity (`docs/aws-acceptance.md`).
- Recovery material: reviewed. State backups are versioned snapshots under
  the deployment lock with 90-day retention; incomplete multipart uploads are
  removed after seven days; the bootstrap lock object reports its owner
  (`docs/operations.md`, state commands).

## Local development

Local development is an execution context, not a cloud target or a low-cost preview
environment. It does not require AWS credentials, Pulumi, remote state, or an
environment account. `magelift local init` creates a small Docker Compose project,
and `magelift local up`, `status`, `logs`, `exec`, `down`, and `reset` manage it. The
default services are MySQL and Valkey. Starting the app profile also starts the
pinned OpenSearch and RabbitMQ services, so a local Magento install exercises the
same capability names as a deployed environment. nginx with PHP-FPM is the default local web runtime. `frankenphp-classic` and `php-apache` are Adobe-unsupported plugins that require `compatibility.allowUnsupported`; they are not certified. The catalog can select Artemis over RabbitMQ and Varnish in front of nginx when those image and health contracts are available. `frankenphp-worker` is unregistered.

The local workflow provides fast source synchronization, deterministic reset commands,
service logs, and direct Magento command execution. `magelift local seed` provides a
non-interactive first install for repositories with `bin/magento` and Composer
dependencies already present. It uses local-only database, cache, search, queue,
and administrator defaults, persists the administrator password in the ignored
mode-0600 `.magelift/local.env`, and stops when `app/etc/env.php` already exists.
The app binds loopback HTTP 8080 and HTTPS 8443. Those certificates are local
only. Use them for cookie and integration tests, not as production TLS evidence.
Local development uses the compatibility catalog and capability names used by
deployed environments so that validation catches unsupported combinations early.
It does not promise identical managed-service behavior; tests that depend on AWS
semantics still run against an ephemeral AWS environment.

Preview queueing uses Magento's database queue. Certified AWS queue cells are
`db` and `ecs-rabbitmq`. Standard and high-availability YAML that omits
`catalog.queueMode` selects `ecs-rabbitmq`. Explicit `amazon-mq` remains
experimental-warn, not certified. Amazon MQ clusters span three availability
zones, so that experimental shape needs three private queue subnets even when
the web tier uses fewer zones.
On EKS and GKE, changing in-cluster RabbitMQ from one replica to quorum (or the
reverse) is refused in place; create a new environment instead.

Docker is the supported host boundary. Debian is the certified container base. A Nix
development shell may pin host tools for contributors, but it remains optional and
does not replace the container contract. Alpine and NixOS images can be evaluated
later if they pass the same Magento and extension tests.

## Safety invariants

Production changes require a preview, signed digest, deployment lock, approval,
successful pre-traffic deploy phase, ECS stabilization, and smoke checks. The
provider-neutral deployment orchestrator keeps that order and always releases the
lock. Rollback is a forward deployment of an older signed digest and does not reverse
database changes.
Protected and retained resources must be explicit during destruction.

## Evolution

Public contracts are independently versioned. Current and previous major configuration
schemas receive deterministic migrations. Released infrastructure component names use
aliases or migration logic before renaming. Material decisions are recorded as ADRs.

Provider growth follows [ADR 0003](adr/0003-portable-contracts-vs-topology.md) and
[ADR 0004](adr/0004-ports-and-adapters.md): Magento-shaped ports live in
`internal/platform`; cloud adapters live under `internal/cloud/<provider>`.
Certified Magento targets are AWS ECS Fargate and GCP GKE Autopilot. GKE Standard,
AWS EKS, OVH (`ovh` / `mks`), and Scaleway (`scaleway` / `kapsule`) stay
experimental. See
[gcp-experimental.md](gcp-experimental.md), [ovh-experimental.md](ovh-experimental.md),
and [scaleway-experimental.md](scaleway-experimental.md). Later clouds
register another adapter without expanding portable YAML into a lowest-common-denominator
cloud schema.

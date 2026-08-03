# Architecture charter

## Product promise

A Magento developer adds `magelift.yaml` to an existing repository and uses one native
CLI to deploy and operate a production-grade environment in their own cloud account,
without editing Pulumi or Go for the supported path.

Presets keep YAML small for agencies and small e-merchants coming from Adobe Commerce
Cloud or Upsun. Power users open catalog escape hatches
([capability matrix](capability-matrix.md)). Headless means Magento
`application.mode: headless|integrated`; storefront frameworks stay external.

## Boundaries

- V1 certifies AWS ECS Fargate. GCP (`gcp` / `gke-autopilot`), AWS EKS
  (`aws` / `eks-autopilot`), OVH (`ovh` / `mks`), and Scaleway
  (`scaleway` / `kapsule`) are experimental (ADR 0007 / 0008). GCP Magento Ops
  and day-2 ports exist as cloud adapters. Multi-cloud is not claimed until two
  targets are certified.
- Stable interfaces may exist for config, lifecycle, and capabilities;
  experimental targets must be labeled in docs and CLI output.
- The CLI orchestrates; Pulumi owns durable infrastructure.
- Artifacts are immutable, signed, built once, and promoted by digest.
- Secrets are references resolved at runtime, never plaintext config.
- Unsupported Magento/service combinations fail before mutate unless an auditable
  override accepts the risk.
- No shared Pulumi components that switch on provider. Each cloud owns topology
  under `internal/cloud/<provider>/`.

## System shape

The control plane is the `magelift` CLI: load typed YAML, resolve an environment,
validate compatibility, then drive build, Pulumi, and ops. On certified AWS the
request path is Route 53, CloudFront, WAF, ALB, and private ECS Fargate. Managed
AWS services hold state; S3 is the media path. Experimental GCP maps Magento onto
GKE Autopilot, Cloud SQL, and Memorystore. See [gcp-experimental.md](gcp-experimental.md).
Magento migrate candidates run as GKE Jobs through the shared `deployflow` port.

The PHP package exposes a lifecycle DAG: validate, build, package, deploy,
post-deploy. The build runner runs the first three without runtime credentials.
Deploy and post-deploy run after connectivity and runtime config injection.
Extensions use stable logical IDs, not raw provider schemas. Normal projects stay
YAML-only.

Artifact manifest creation has a pre-digest prepare step and a post-build finalize
step. The finalized manifest stays outside the image and binds the prepared metadata
to BuildKit's OCI digest. An image may contain pre-digest build metadata, but MageLift
does not rebuild or mutate it to embed the final manifest. See [ADR 0003](adr/0003-external-final-artifact-manifest.md).

## Portability boundary

The application model, build phases, artifact manifest, and capability requirements
are portable contracts. Network layout, compute resources, managed services, recovery
controls, and cost models belong to a target implementation.

The web runtime sits on the portable side of this boundary. Nginx with PHP-FPM is the
default certified implementation; its ECS task uses an nginx HTTP container and a
PHP-FPM container built from the same immutable image. Integrated tasks add the pinned
Varnish 8.0.2 sidecar on port 6081 and route the load balancer through it to nginx on
port 8080. The Varnish root filesystem is read-only; its VSM and transient cache use
an executable task-scoped tmpfs at `/var/lib/varnish`. Headless tasks route directly to nginx on port 8080. FrankenPHP classic is
a selectable runtime contract with its own image adapter. Both use the same health,
port, task, and artifact checks. The selected application mode is passed to the
application container as a runtime contract, so integrated and headless deployments
can evolve their request and capability wiring without changing the public provider
boundary.
FrankenPHP worker mode is reserved and does not appear in configuration because
Magento compatibility has not been proven.

Certified v1 targets are AWS ECS Fargate and GCP GKE Autopilot. Other runtimes and
clouds implement `sdk/v1` Target contracts and register a `platform.StackModule`.
Each provider keeps capabilities explicit and must pass the shared Magento
acceptance suite before certification. Portable YAML is not a lowest-common-denominator
cloud catalog.

### AWS Magento product matrix

"Full AWS Magento" means the Magento acceptance path on ECS Fargate with explicit
escape hatches, not every AWS SKU.

| Choice | Certified (ECS Fargate) | Experimental / deferred |
| --- | --- | --- |
| Runtime | `ecs-fargate` | `eks-autopilot` (EKS Auto Mode-shaped; infra-only) |
| `natMode` | `nat-gateway` (default), `fck-nat` (cost/preview) | - |
| `databaseEngine` | `aurora-mysql`, `rds-mysql` | - |
| `searchMode` | `serverless`, `provisioned`, `disabled` | OpenSearch deferred on EKS |
| Queue (`catalog.queueMode`) | `db`, `amazon-mq`, `ecs-rabbitmq` | `ecs-artemis` (experimental); deferred on EKS |
| Edge | CloudFront + WAF | deferred on EKS |
| Day-2 ops | deploy/logs/exec via AWS adapters | `ErrNotSupported` on EKS until phase 3 |

See [aws-eks-experimental.md](aws-eks-experimental.md) for the EKS path and
[gcp-experimental.md](gcp-experimental.md) for GCP.

Evidence tiers differ: Pulumi mocks prove composition, Floci proves selected AWS
API contracts without an account, and `scripts/aws-acceptance-local.sh` proves a
real account path with destroy + `assert_clean`. Mocks alone do not certify a
matrix cell for production use.

Coming from Adobe Commerce Cloud or Platform.sh? Start with
[migrating-from-paas.md](migrating-from-paas.md).

See [ADR 0002](adr/0002-provider-runtime-extension-boundary.md) and
[ADR 0008](adr/0008-ports-and-adapters-multi-provider.md). Contributor checklist:
[adding-a-provider.md](adding-a-provider.md).

## Repository layout

Provider-neutral code: `internal/platform` (stack modules, Magento output keys,
env bindings), `internal/automation`, `internal/deploy`, `internal/infra` (SDK
extension index), `internal/topology`, and `sdk/v1`. Providers live under
`internal/cloud/<provider>/`.

AWS packages today include `bootstrap`, `secrets`, `target`, `network`,
`security`, `ingress`, `runtime`, `edge`, `database`, `cache`, `search`, `queue`,
`storage`, `observability`, `operations`, `deployment`, `stack`, and `state`.
GCP experimental code mirrors the stack composition boundary under
`internal/cloud/gcp/`.

**CLI deploy path:** `platform.ModuleRegistry` selects a `StackModule` by
provider + runtime. Magento lock and candidate steps are optional via
`platform.HasOps`. Day-2 surfaces (bootstrap, state, secrets, logs, exec, runtime
health) resolve the same way through `HasBootstrap` / `HasState` / `HasSecrets` /
`HasRuntimeObserve` ([ADR 0009](adr/0009-day2-magento-ports.md)). DIY stack names
include provider and runtime so targets do not collide.
`internal/infra.Registry` is for Target/Capability/Hook discovery tests; it does
not replace module registration.

The SDK registry indexes targets, capabilities, typed transforms, and lifecycle
hooks by stable IDs. Transforms stay type-checked at the boundary. Hook discovery
is deterministic by phase and ID. The default CLI ships AWS plus experimental GCP;
third-party modules use a custom binary that calls `RegisterModule` (ADR 0007).

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

Runtime configuration keeps the root filesystem read-only. The image contains only a
non-secret `env.php` scaffold. ECS Secrets Manager selectors inject the managed
database JSON fields and the stable Magento encryption key, while Adobe's
`MAGENTO_DC_*` environment configuration supplies capability endpoints and
credentials at task start.

## Local development

Local development is an execution context, not a cloud target or a low-cost preview
environment. It does not require AWS credentials, Pulumi, remote state, or an
environment account. `magelift dev init` creates a small Docker Compose project,
and `magelift dev up`, `status`, `logs`, `exec`, `down`, and `reset` manage it. The
default services are MySQL and Valkey. Starting the app profile also starts the
pinned OpenSearch and RabbitMQ services, so a local Magento install exercises the
same capability names as a deployed environment. The app uses the same FrankenPHP
classic image contract used by the runtime.

The local workflow provides fast source synchronization, deterministic reset commands,
service logs, and direct Magento command execution. `magelift dev seed` provides a
non-interactive first install for repositories with `bin/magento` and Composer
dependencies already present. It uses local-only database, cache, search, queue,
and administrator defaults, persists the administrator password in the ignored
mode-0600 `.magelift/local.env`, and stops when `app/etc/env.php` already exists.
The FrankenPHP app also exposes `https://localhost:8443` through Caddy's internal
development CA. The CA state is ephemeral and local to the container, so this
listener is for secure-cookie and integration testing rather than production TLS.
Local development uses the compatibility catalog and
capability names used by deployed environments so that validation catches
unsupported combinations early. It does not promise identical managed-service
behavior; tests that depend on AWS semantics still run against an ephemeral AWS
environment.

Preview queueing uses Magento's database queue. Standard and high-availability
environments use Amazon MQ for RabbitMQ cluster deployments with quorum queues. A
RabbitMQ cluster spans three availability zones, so the target must provision three
private queue subnets even when the web tier uses fewer zones.

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

Provider growth follows [ADR 0007](adr/0007-multi-provider-community-targets.md) and
[ADR 0008](adr/0008-ports-and-adapters-multi-provider.md): AWS is the certified v1
target; Magento-shaped ports live in `internal/platform`; cloud adapters live under
`internal/cloud/<provider>`. GCP (`gcp` / `gke-autopilot`), OVH (`ovh` / `mks`), and
Scaleway (`scaleway` / `kapsule`) are experimental first-party candidates. See
[gcp-experimental.md](gcp-experimental.md), [ovh-experimental.md](ovh-experimental.md),
and [scaleway-experimental.md](scaleway-experimental.md). Later clouds
register another adapter without expanding portable YAML into a lowest-common-denominator
cloud schema.

# Architecture charter

## Product promise

A Magento developer adds `magelift.yaml` to an existing repository and uses one native
CLI to deploy and operate a production-grade environment in their own AWS account,
without editing Pulumi or Go for the supported path.

## Boundaries

- V1 is AWS-only and certifies ECS Fargate.
- Configuration, application lifecycle, capabilities, and target APIs may be stable
  interfaces, but must not imply unimplemented multi-cloud support.
- The CLI orchestrates; Pulumi owns durable infrastructure.
- Artifacts are immutable, signed, built once, and promoted by digest.
- Secrets are references resolved at runtime and never plaintext configuration.
- Unsupported Magento/service combinations fail before infrastructure mutation unless
  a committed, auditable override explicitly accepts the risk.

## System shape

The public control plane is the native `magelift` CLI. It loads strict typed YAML,
resolves an environment, validates compatibility, and drives build, Pulumi, AWS, and
operational workflows. The initial production request path is Route 53, CloudFront,
WAF, ALB, and private ECS Fargate tasks. Managed AWS services supply stateful
capabilities; S3 remote storage is the media golden path.

The PHP build package exposes a typed lifecycle DAG: validate, build, package, deploy,
and post-deploy. The build runner executes the first three nodes without runtime
credentials. The deployment workflow consumes the deploy and post-deploy nodes after
it has established connectivity and injected runtime configuration. Extension
interfaces use stable logical IDs rather than raw provider schemas. Normal projects
remain YAML-only.

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

AWS ECS Fargate is the only certified v1 target. Future AWS EKS, Kubernetes, or other
cloud targets must implement versioned `Target` and `CapabilityProvider` interfaces.
Each provider keeps its capabilities explicit and must pass the shared
application-level acceptance suite. MageLift does not flatten provider features into a
lowest-common-denominator YAML schema and does not claim multi-cloud support today.

See [ADR 0002](adr/0002-provider-runtime-extension-boundary.md) for the extension
boundary and certification rule.

## Repository layout

Provider-neutral Go code lives in `internal/infra`, `internal/topology`,
`internal/automation`, `internal/deploy`, and `sdk/v1`. Provider implementations live below
`internal/cloud/<provider>` so adding another cloud does not scatter provider code
through shared packages. The AWS implementation currently has `bootstrap`,
`secrets`, `target`, `network`, `security`, `ingress`, `runtime`, `edge`, `database`,
`cache`, `search`, `queue`, `storage`, `observability`, `operations`, `deployment`,
`stack`, and `state` packages.
The extension registry SDK validates and indexes targets, capabilities, typed
transforms, and lifecycle hooks by stable IDs. Transform registration keeps the options type
checked at the extension boundary; lookup cannot apply a transform to a component
outside its descriptor. Hook discovery is deterministic by phase and ID. The v1 CLI
ships only the built-in AWS target; loading compiled third-party extensions remains an
advanced integration boundary rather than an implicit project feature.
The stack package is a thin AWS composition boundary. It receives a validated plan,
selects only catalog values supplied by the caller, and wires Pulumi outputs between
capability components. Regional resources and CloudFront/WAF resources use separate
provider instances. Each capability validates its typed inputs before registering
resources. The planner also checks the selected Magento release against the AWS
service versions listed in the Adobe compatibility tables. `allowUnsupported` is the
only way to record an exception, and the resulting artifact metadata marks it.
An explicit existing-network reference can provide a VPC and its public, private, and
data subnets; in that mode MageLift does not create routing, NAT, or VPC endpoint
resources for the imported network.

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

The current infrastructure commands keep the seams narrow: configuration produces
an AWS `stack.Spec`, the Automation API owns the Pulumi workspace, and the CLI owns
environment selection, approval checks, the deployment lock, the candidate migration
task, and post-update health evidence. A stack backend can be replaced in tests, so
the command path does not need AWS credentials to exercise validation and failure
handling.

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

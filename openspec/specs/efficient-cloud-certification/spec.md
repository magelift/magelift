## Purpose

Defines a coverage-driven certification runner that proves cold architectural
boundaries and reuses compatible cloud resources without multiplying paid
provisioning, teardown, artifact builds, or restore work unnecessarily.

## Requirements

### Requirement: Coverage is boundary-driven rather than Cartesian

The certification catalog MUST enumerate required cold boundaries and allowed
warm transitions for provider, region, compute topology, managed or
self-hosted stateful services, service majors, resilience profile,
observability path, edge path, artifact, schema, and migration. It MUST report
why a declared combination is represented by a shared baseline, a warm
transition, a contract test, an unsupported cell, or an unavailable cell.

#### Scenario: Three queues share one compatible stack

- **WHEN** database messaging, RabbitMQ, and Artemis preserve provider,
  topology, database, artifact, schema, and resilience assumptions
- **THEN** the catalog provisions one baseline and records the broker changes as
  warm transitions with explicit scope instead of provisioning three stacks

### Requirement: Immutable artifacts are built once per compatibility boundary

The runner MUST build, scan, sign or verify, and publish one immutable artifact
per required runtime contract and reuse its digest across compatible provider
cells. A mutable tag, changed PHP or Composer contract, or migration-affecting
artifact MUST force a new cold baseline.

#### Scenario: AWS and GCP consume the same application contract

- **WHEN** two providers support the same immutable runtime contract but use
  different infrastructure adapters
- **THEN** the runner reuses the verified artifact digest while keeping provider
  infrastructure and service evidence separate

### Requirement: Concurrency is quota- and cleanup-aware

The scheduler MAY run independent provider groups concurrently only after
checking account quotas, regional capacity, local CPU and memory budgets,
credential isolation, state-lock isolation, and unique ownership scopes. Runs
sharing a provider account, project network, service-connection policy,
database fixture, or cleanup scope MUST be serialized.

#### Scenario: AWS and GCP runs are independent

- **WHEN** AWS and GCP groups have isolated credentials, state, prefixes,
  artifacts, and provider quotas
- **THEN** the scheduler may overlap their slow infrastructure waits while
  preventing overlapping mutations inside either provider group

### Requirement: Warm sessions have a complete fingerprint

Warm reuse MUST fingerprint provider, region set, compute topology, release,
edition, artifact digest, runtime contract, database engine and major,
schema/migration, service majors, resilience profile and backup fixture,
observability signals and destination, edge path, and managed/self-hosted
boundaries. Any changed field MUST require a cold baseline.

#### Scenario: Observability destination changes

- **WHEN** a profile changes from native-only telemetry to New Relic export
- **THEN** the runner starts a new boundary or an explicitly approved
  observability transition and does not reuse evidence that lacks New Relic
  signal and cleanup proof

### Requirement: Baselines own shared fixtures and migration

Each cold baseline MUST own one scrubbed data fixture, one migration owner,
one backup seed, one observability setup, one edge setup, and one final
dependency-aware teardown. Warm cells MUST reuse those resources only when the
fingerprint permits and MUST NOT run a second migration or silently replace
the restore fixture.

#### Scenario: A restore drill follows warm service cells

- **WHEN** queue and search transitions have passed on a shared baseline
- **THEN** the restore drill uses the baseline's declared backup fixture and
  records whether it is a warm validation or a new cold recovery boundary

### Requirement: Checkpoint resume is safe after interruption

The runner MUST persist checkpoint identity, fingerprint, completed stages,
resource ownership, backup identifiers, edge identifiers, observability
identifiers, and cleanup state atomically enough to resume. An incompatible or
stale checkpoint MUST be archived and MUST NOT be reused.

#### Scenario: The process stops during provider deletion

- **WHEN** an interruption occurs after application destroy but before
  asynchronous provider resources disappear
- **THEN** resume continues bounded cleanup and inventory assertion for the
  original ownership scope without creating a second stack

### Requirement: Slow provider operations are amortized and bounded

The runner MUST use create-once flows, warm updates, prevalidated manifests,
restore-fixture reuse, asynchronous deletion polling, provider-specific retry
budgets, and per-stage timeouts. It MUST report cost as measured, estimated, or
unknown and MUST fail or pause when the configured time or spend budget is
exceeded.

#### Scenario: GKE deletion outlives the test timeout

- **WHEN** a provider accepts deletion but the resource remains in a transient
  state
- **THEN** the runner waits within the provider-specific cleanup budget,
  records the delayed resource, and prevents the next run from reusing its
  network or ownership scope prematurely

### Requirement: Test layers close before paid certification

The workflow MUST run catalog validation, unit tests, provider contract tests,
mock plans, local restore tests, and dry-run harness checks before live apply.
Live certification MUST run only cells that are compatible, available,
credentialed, and required by the declared claim.

#### Scenario: A provider contract fails offline

- **WHEN** a managed-service API contract or restore adapter test fails before
  live credentials are used
- **THEN** the scheduler marks the affected cells blocked or incomplete and
  does not spend credits on a known-invalid architecture

### Requirement: Local certification dependencies are preflighted

Every local certification entrypoint MUST check its required executables and
the supported tool dialects before provider authentication that can mutate
resources or before any provider mutation. JSON/YAML tooling checks MUST be
shared, MUST distinguish Mike Farah `yq` v4 from other `yq` implementations,
and MUST fail with an actionable installation hint. The entrypoint MUST NOT
silently install software on the user's machine.

#### Scenario: A workstation has no YAML parser

- **WHEN** a user starts a live or dry-run harness without a compatible `yq`
- **THEN** the harness reports the missing or incompatible dependency and an
  installation path before creating or mutating a provider resource

#### Scenario: The wrong yq dialect is first on PATH

- **WHEN** a jq-style or Mike Farah `yq` v3 executable is selected
- **THEN** the harness rejects it before reading or patching the acceptance
  configuration and tells the user to install Mike Farah `yq` v4

### Requirement: Evidence is sufficient for closure

Every live result MUST include architecture identity, provider and region,
service versions, artifact digest, backup and restore identifiers,
observability signal results, edge result, measured RPO/RTO, duration, cost
status, ownership markers, teardown mode, direct inventory, and final status.
Warm evidence MUST state which claims it does not independently prove.

#### Scenario: A warm transition passes without a cold restore

- **WHEN** a queue transition passes but no backup restore or DR exercise ran
- **THEN** the evidence records the transition as warm service evidence and
  leaves the resilience and DR requirements open

### Requirement: Cleanup is a certification stage

Every live run MUST attempt normal and interrupted teardown, poll asynchronous
deletion, assert direct owning-service inventories, report delayed tombstones,
and preserve unmarked resources. A run with unresolved owned resources MUST NOT
be reported as clean or certifying.

#### Scenario: A provider tag remains after the resource is gone

- **WHEN** the tag index reports a resource but the owning service reports no
  live object
- **THEN** evidence records the tag as a tombstone and relies on the owning
  service inventory for cleanup truth without deleting unrelated resources

### Requirement: Packed warm sessions cover many cells per origin

Live certification MUST reuse one KEEP Magento stack per origin session. Local CLI contracts that session 1 will exercise (named health layers, dump-from-live, rollback-vs-schema fail-closed, SendGrid secret-reference validation on the environment config) MUST exist with unit or fake-client proof before KEEP is set. Session 1 MUST be GCP Autopilot Magento 2.4.9 preview and MUST run remaining Magento-wired observability, New Relic, Fastly, Cloudflare DNS, SendGrid delivery or typed unsupported, dump-retrieve, layered health, rollback-vs-schema, remaining HA or backup cells that still need Magento, and the existing failed-deployment injector by setting `MAGELIFT_GCP_FAILED_DEPLOY_DIGEST`, before a single destroy and `assert_clean`. Session 1 MUST NOT re-implement `InjectFailedDeployment`. Session 2 MUST be AWS ECS Fargate Magento and MUST NOT re-prove Fastly, New Relic, SendGrid, or Cloudflare. Session 3 MAY run OVHcloud and Scaleway infrastructure-only cells concurrently with session 2 when credentials, state, and cleanup scopes are isolated. Magento on OVHcloud and Scaleway MUST remain experimental. GCS, Secret Manager, Pub/Sub, Logging, and Monitoring API regressions MUST run against floci-gcp rather than a Magento stack. Digest-pinned floci-gcp GCS and Floci AWS IAM OpenID Connect Provider smokes are already CI contracts. Remaining Secret Manager, Pub/Sub, Logging, and Monitoring smokes MUST NOT delay KEEP when the emulator rejects the official client; record typed unsupported instead.

#### Scenario: Integrations attach to the GCP origin

- **WHEN** New Relic, Fastly, Cloudflare, or SendGrid need live Magento evidence
- **THEN** the runner attaches them to the session-1 GCP origin and does not provision a second Magento stack

#### Scenario: AWS session skips already-proven integrations

- **WHEN** session 2 runs AWS ECS Fargate Magento after session 1 has Fastly New Relic Cloudflare and SendGrid evidence
- **THEN** those integrations are not re-executed on AWS and AWS evidence records Magento plus AWS-native remaining cells only

#### Scenario: KEEP does not invent CLI contracts

- **WHEN** dump-from-live or rollback-vs-schema has no unit-tested command
- **THEN** session 1 does not set KEEP until that command exists, rather than extending a live Magento stack while the CLI is written

#### Scenario: Integrations attach to the GCP origin

- **WHEN** New Relic, Fastly, Cloudflare, or SendGrid need live Magento evidence
- **THEN** the runner attaches them to the session-1 GCP origin and does not provision a second Magento stack

#### Scenario: AWS session skips already-proven integrations

- **WHEN** session 2 runs AWS ECS Fargate Magento after session 1 has Fastly New Relic Cloudflare and SendGrid evidence
- **THEN** those integrations are not re-executed on AWS and AWS evidence records Magento plus AWS-native remaining cells only

### Requirement: Emulators never certify provider-only behavior

Digest-pinned Floci AWS and floci-gcp suites MUST close cheaper API contracts in CI in parallel. Emulation MUST NOT mark a cell certified for GKE Autopilot, Memorystore Valkey 9.0, Cloud Armor data plane, Google-managed TLS, Magento Cloud SQL PITR, live WAF, GitHub Actions OIDC token exchange, or regional DR. Digest-pinned Floci AWS 1.7.0 MAY close IAM Create/Get/Tag OpenID Connect Provider API contracts.

#### Scenario: floci-gcp object restore is not Autopilot evidence

- **WHEN** a GCS or Secret Manager contract passes against floci-gcp
- **THEN** evidence records an emulator API contract and leaves Autopilot Magento, Memorystore, Armor, and managed TLS claims open

### Requirement: Certification follows a test pyramid and real providers when needed

Certification MUST close cheaper layers before paid cloud: unit and static analysis, package tests, integration tests, Docker or local emulation (including digest-pinned Floci for AWS API contracts and floci-gcp for GCS, Secret Manager, Pub/Sub, Logging, Monitoring, and IAM or STS contracts where they apply, and `act` for GitHub Actions workflows), then real provider certification. Emulation MUST NOT certify behavior that only a real provider can prove (GitHub Actions OIDC token exchange, managed TLS issuance, live WAF data plane, regional DR, GKE Autopilot, Memorystore Valkey 9.0, Magento Cloud SQL PITR). Live Magento certification order MUST be GCP first. Cloudflare, Fastly, New Relic, and SendGrid tests MUST attach to the GCP origin session when the vendor is the claim. AWS, Scaleway, OVHcloud, Cloudflare, Fastly, New Relic, and SendGrid live cells MUST assume thin paid credits: minimum viable sizes, short-lived Magento, reuse of slow GCP KEEP resources where safe, explicit cleanup, and ownership tags. Scaleway Magento SKUs MUST NOT be purchased on a plan that cannot afford them; record Magento blocked or experimental instead.

#### Scenario: Floci is not TLS evidence

- **WHEN** a CloudFront or Cloud Armor cell has only Floci or mock graph evidence
- **THEN** the cell MUST NOT be marked certified for public HTTPS or WAF data-plane behavior

#### Scenario: OVH cell stays small

- **WHEN** an OVHcloud live certification cell is planned
- **THEN** it uses the documented essential or minimum SKU, a unique ownership marker, and destroy-on-exit, and does not provision production-size HA by default

#### Scenario: Paid integration attaches to GCP

- **WHEN** Fastly, New Relic, or SendGrid live evidence is required
- **THEN** the runner attaches to a packed GCP Magento session or leaves the vendor cell unproven, and does not open a second Magento origin only for that vendor

### Requirement: AWS coverage is the implemented catalog plus packed KEEP

AWS certification coverage MUST enumerate required cold boundaries (Fargate, Managed Instances, EKS) and allowed warm catalog transitions on one KEEP digest. Exhaustive support means every implemented cell is listed and classified; it MUST NOT mean one live Magento shop per combination.

#### Scenario: Search modes are classified without three shops

- **WHEN** `searchMode` values `disabled`, `serverless`, and `provisioned` are all implemented
- **THEN** the catalog lists all three, live KEEP may pack compatible transitions, and only tuples with Magento evidence MAY be certified

### Requirement: GCP coverage is independent of AWS packing

GCP certification coverage MUST enumerate Autopilot versus Standard cold boundaries and allowed warm service transitions on one GCP KEEP digest. It MUST NOT share a fingerprint, prefix, or teardown with AWS. Exhaustive support means every implemented `target.gcp` cell is listed; it MUST NOT mean one live shop per combination.

#### Scenario: Autopilot and Standard are separate cold baselines

- **WHEN** a session has an Autopilot Magento KEEP baseline and YAML switches to `gke-standard`
- **THEN** the runner starts a cold Standard baseline instead of a warm Autopilot cell-update

### Requirement: OVH stays one serialized preview

OVH certification coverage MUST be one MKS Magento preview, serialized against other OVH mutations, with no KEEP and no overlap with AWS or GCP KEEP fingerprints. Exhaustive support means every implemented `target.ovh` cell is listed; it MUST NOT mean one live shop per combination.

#### Scenario: OVH does not wait on AWS Aurora

- **WHEN** AWS KEEP is mutating Aurora and OVH credits allow one MKS preview
- **THEN** the OVH preview MAY run in its own prefix and state, and AWS stall MUST NOT hide OVH catalog status

### Requirement: Scaleway stays one serialized preview inside the money cap

Scaleway certification coverage MUST be one Kapsule Magento preview, serialized against other Scaleway mutations, with no KEEP and no overlap with AWS or GCP KEEP fingerprints. Exhaustive support means every implemented `target.scaleway` cell is listed; it MUST NOT mean one live shop per combination. Live Scaleway plus vendor cells MUST share the $50 own-money cap.

#### Scenario: Scaleway does not wait on AWS Aurora

- **WHEN** AWS KEEP is mutating Aurora and the $50 cap still allows one Kapsule preview
- **THEN** the Scaleway preview MAY run in its own prefix and state, and AWS stall MUST NOT hide Scaleway catalog status

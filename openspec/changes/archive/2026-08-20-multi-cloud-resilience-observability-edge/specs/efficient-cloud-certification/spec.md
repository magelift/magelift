## Purpose

Defines a coverage-driven certification runner that proves cold architectural
boundaries and reuses compatible cloud resources without multiplying paid
provisioning, teardown, artifact builds, or restore work unnecessarily.

## ADDED Requirements

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

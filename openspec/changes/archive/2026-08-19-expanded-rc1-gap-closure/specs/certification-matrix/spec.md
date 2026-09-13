## Purpose

Defines how MageLift represents and closes the Adobe Commerce architecture
matrix without turning a passing topology smoke test into a broad support claim.

## ADDED Requirements

### Requirement: Matrix cells have explicit dimensions

Every release-gate cell MUST identify the Adobe release and patch, edition,
provider, region, runtime topology, database engine and major version, search,
cache, queue, web-cache, edge, observability, artifact digest, and effective
runtime contract. An omitted dimension MUST resolve to a named preset value,
not an implicit provider default.

#### Scenario: A user selects a database-backed queue

- **WHEN** the project requests Magento 2.4.9 on GKE with MySQL 8.4, OpenSearch,
  Valkey, database messaging, and no Varnish
- **THEN** the plan and evidence record each selected dimension and the resolved
  runtime contract

### Requirement: Provider capability is the intersection of three contracts

A cell MUST be marked supported only when the Adobe requirement, the provider
service/version availability, and MageLift's implementation all agree. A
provider choice that cannot satisfy the dated Adobe requirement MUST be marked
unsupported or blocked before mutation. A provider capability without live
MageLift evidence MUST remain experimental or not-run.

#### Scenario: A provider offers an older cache version

- **WHEN** a provider exposes Valkey 8 while the selected Adobe patch requires
  Valkey 9
- **THEN** the cell remains experimental or blocked and the release gate names
  the version mismatch

### Requirement: Certification status follows required evidence stages

Only a cell with the required live build, deploy, runtime health, service
health, application operation, and exact cleanup evidence MAY be certified.
Mocks, imports, topology-only evidence, or a warm transition MUST NOT close a
cold application certification gate. Every unrun, unavailable, blocked,
unsupported, and experimental cell MUST remain visible in generated matrix
output.

#### Scenario: A warm queue transition passes

- **WHEN** a database-backed baseline is changed to a self-hosted RabbitMQ
  service and the health checks pass
- **THEN** the transition is recorded as warm experimental or certified only
  within its declared scope, and it does not imply a fresh cold baseline

### Requirement: Cold boundaries are deterministic

The runner MUST create a new baseline when the provider, Kubernetes topology,
database engine or major version, migration-affecting artifact, schema, or
managed versus self-hosted service boundary changes. Compatible service-only
changes MAY reuse a warm session with the shared fingerprint.

#### Scenario: The search service changes from managed to self-hosted

- **WHEN** a session keeps the application and database contract but replaces a
  managed search service with a self-hosted workload
- **THEN** the runner starts a cold baseline unless the provider contract
  explicitly proves the boundary is service-only and schema-compatible

### Requirement: Matrix evidence includes cost and cleanup truth

Each applied cell MUST record planned cost status, baseline or reuse status,
per-cell duration, ownership markers, final teardown mode, delayed tombstones,
protected objects, and the direct inventory result. A missing cost estimate or
delayed provider deletion MUST be reported as unknown or pending, never as zero.

#### Scenario: A provider keeps a deleted encryption key in a tombstone window

- **WHEN** all live workloads are gone but the provider schedules the key for
  deletion
- **THEN** the cell cleanup passes for live resources and records the key as a
  delayed tombstone with its deletion date

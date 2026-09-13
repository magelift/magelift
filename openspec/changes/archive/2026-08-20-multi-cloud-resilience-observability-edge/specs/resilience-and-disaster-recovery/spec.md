## Purpose

Defines how MageLift protects durable data and restores service across normal
failure, zone loss, region loss, corruption, credential loss, and deployment
failure for every declared provider architecture.

## ADDED Requirements

### Requirement: Resilience targets are explicit inputs

Every production or certification profile MUST declare availability target,
maximum RPO, maximum RTO, retention period, recovery scope, and whether
recovery is restore-in-place, alternate-zone, alternate-region, or
alternate-provider. A profile without targets MUST remain unclassified and
MUST NOT be called production-ready.

#### Scenario: A production profile omits RPO

- **WHEN** an operator plans an HA production environment without a maximum
  acceptable data-loss window
- **THEN** validation fails before mutation and asks for an explicit resilience
  profile instead of assuming the provider's default backup behavior

### Requirement: Durable data classes have independent protection plans

The recovery plan MUST classify database data, media and object data,
configuration, secrets, infrastructure state, queue messages, search indexes,
cache contents, and audit evidence as durable, reconstructible, or ephemeral.
Each durable class MUST have an owner, backup mechanism, retention, encryption
boundary, restore destination, and verification method.

#### Scenario: Search and cache are recreated after database restore

- **WHEN** a recovery restores the database and media but rebuilds search and
  cache from durable sources
- **THEN** the evidence records search and cache as reconstructible and proves
  that the restored application does not depend on an unbacked-up cache or
  stale index

### Requirement: Backups are immutable enough for the declared threat model

Database backups, point-in-time logs, object versions or replicas, state
backups, configuration snapshots, and recovery credentials MUST use retention,
encryption, access control, and immutability or deletion protection suitable
for the selected resilience profile. Backup success MUST be independent of
the application deployment success signal.

#### Scenario: A deployment fails after a successful backup

- **WHEN** a release fails after the pre-deployment backup completes
- **THEN** the operator can identify the backup, its retention deadline,
  checksum or provider identity, and its restore procedure without relying on
  the failed application stack

### Requirement: Restore is tested with known data

Every certifying resilience profile MUST restore a scrubbed fixture containing
known database, media, configuration, and representative queue records into an
isolated destination. The test MUST verify checksums, row or object counts,
application reads, permissions, and post-restore service health.

#### Scenario: A restore silently loses media

- **WHEN** the database restore passes but a media object or checksum is
  missing
- **THEN** the recovery exercise fails, records the loss class and measured
  RPO, and does not mark the profile resilient

### Requirement: HA failure behavior is exercised

Each HA profile MUST exercise at least one failure appropriate to its topology,
including a zone or node loss, stateful replica loss, ingress failure, queue
consumer failure, or managed-service failover. The application MUST maintain
the declared availability target or record the measured outage and classify
the profile accordingly.

#### Scenario: One availability zone becomes unavailable

- **WHEN** an HA profile loses one eligible zone or stateful replica
- **THEN** traffic, queue processing, and durable writes follow the declared
  failover policy, and evidence records recovery time, degraded signals, and
  any data loss

### Requirement: Disaster recovery is an executable runbook

Regional and alternate-provider recovery plans MUST define provisioning order,
secret and identity recovery, network and DNS changes, data restore, search and
  cache rebuild, queue handling, edge cutover, rollback or fencing, and final
  cleanup. A plan MUST state which steps are automatic, operator-approved, or
  unsupported.

#### Scenario: The primary region is unavailable

- **WHEN** the operator invokes an alternate-region recovery profile
- **THEN** the runbook provisions only the declared recovery scope, restores
  data and credentials, proves application and edge health, measures RPO/RTO,
  and leaves the primary region fenced or explicitly unavailable

### Requirement: Recovery prevents split-brain and unsafe failback

Failover and failback MUST use an explicit ownership or fencing mechanism so
that two writable application stacks cannot process the same durable workload
without a declared conflict strategy. DNS, edge, queue, and scheduled-job
cutovers MUST be coordinated with the durable data owner.

#### Scenario: The old region returns during failover

- **WHEN** the primary region becomes reachable after the recovery region is
  serving traffic
- **THEN** MageLift keeps one writer authoritative, blocks unsafe failback,
  and requires an explicit reconciliation or operator approval before write
  traffic moves again

### Requirement: Resilience evidence is release-gated

A profile MUST NOT be called HA, backup-protected, disaster-recoverable, or
production-ready until its applicable backup, restore, failure, RPO/RTO,
integrity, failover, edge, observability, and cleanup stages pass. Unsupported
or unexercised recovery paths MUST remain visible in the release gate.

#### Scenario: Backups pass but DR is untested

- **WHEN** a profile has successful backups but no measured regional recovery
- **THEN** the profile may be labeled backup-capable but remains open for DR and
  cannot be advertised as disaster-recoverable

### Requirement: Failure drills are resumable and cleanup-independent

Every automated or operator-assisted failure drill MUST persist the exact
architecture fingerprint, scenario identity, ownership marker, fault
operation/resource identities, observation, and cleanup state before it can be
resumed or included in evidence. An interruption after fault injection MUST
resume observation or cleanup for the same fault scope and MUST NOT inject a
second fault. Traffic, integrity, fencing, and recovery measurements MUST be
validated independently from cleanup; a passing failure observation MUST NOT
imply that injected resources were deleted or that unowned resources were
preserved.

#### Scenario: A zone-loss drill is interrupted after injection

- **WHEN** the provider reports that the zone fault was injected but the
  process exits before observation or cleanup completes
- **THEN** the next run loads the same scenario fingerprint and ownership
  scope, skips a second injection, completes observation and cleanup, and
  records a non-certifying result if any declared gate or cleanup proof is
  missing

#### Scenario: Checkpoint persistence fails after fault injection

- **WHEN** the provider confirms a fault but the runner cannot persist the
  post-injection checkpoint
- **THEN** the runner attempts bounded cleanup without honoring caller
  cancellation, records a blocked terminal result if cleanup completes, or
  persists an explicit pre-observation cleanup-pending state; a retry MUST
  never inject another fault while the original cleanup state is unresolved

#### Scenario: A provider cannot inject a failure safely

- **WHEN** the selected architecture has no safe fault injector for a declared
  scenario
- **THEN** the runner records an explicit `SKIP`, `BLOCKED`, or `NOT_RUN`
  reason without invoking a provider mutation and keeps the scenario visible
  in the release matrix

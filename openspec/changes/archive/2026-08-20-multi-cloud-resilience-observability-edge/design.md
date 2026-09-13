## Context

The existing provider adapters expose different lifecycle shapes and the
current acceptance work is organized around selected runtime cells. Existing
OpenSpec changes cover compatibility evidence, Fastly migration, and community
extensions, but none owns the complete production contract for resilience,
observability, native edge, and recovery across all four providers. See
`proposal.md` for the motivation and scope.

The design must preserve the current provider-neutral platform boundary,
Pulumi ownership inside provider adapters, secret-reference rules, immutable
artifact evidence, and checkpointed acceptance sessions. The former GCP cache
path hardcoded `VALKEY_8_0` even though current Memorystore documentation
supports Valkey 9.0 GA; that implementation has now been corrected, while old
Valkey 8 live evidence remains historical and non-certifying for the current
profile.

## Goals / Non-Goals

**Goals:**

- Make architecture coverage enumerable, source-dated, and pre-mutation
  validateable across AWS, GCP, Scaleway, and OVHcloud.
- Make resilience a first-class contract over durable data, recoverable
  services, edge routing, telemetry, and operator runbooks.
- Keep native provider services and external Fastly/New Relic capabilities
  composable without leaking vendor schemas into the portable core.
- Reduce paid certification time through boundary-aware scheduling and shared
  baselines while retaining independent evidence for every cold boundary.
- Produce evidence that a staff engineer can use to decide whether an
  architecture is supported, experimental, blocked, unavailable, or merely
  exercised.

**Non-Goals:**

- Promising every theoretical combination of every provider service.
- Automatically failing over between providers without an explicit data,
  identity, DNS, edge, and operator policy.
- Treating backups as valid until a restore and integrity check pass.
- Treating a native log exporter, New Relic dashboard, Fastly route, or CDN
  resource as production-certified without lifecycle and failure evidence.
- Choosing business RPO/RTO/SLO values on behalf of every MageLift user. The
  system will require explicit profiles and certify against their declared
  targets.

## Decisions

### 1. Use a capability registry plus architecture profiles

The source of truth will separate four decisions that are currently mixed:

1. Adobe release compatibility.
2. Provider product and regional availability.
3. MageLift adapter implementation status.
4. Live certification evidence.

An architecture profile references those records and selects compute,
stateful-service boundaries, resilience, observability, and edge. Provider
records include an official source URL, retrieval date, supported regions or
zones, service major, lifecycle stage, limits, and evidence reference. This
prevents a stale deployed instance from being mistaken for the provider's
current capability. For example, GCP 2.4.9 can select `VALKEY_9_0`, while old
Valkey 8 evidence remains historical until a new run passes.

Alternative rejected: infer support from Pulumi defaults or the last live
instance. That is exactly how the Valkey 8 mismatch escaped the previous gate.

### 2. Use progressive-disclosure YAML with semantic provider escape hatches

The configuration resolver has a deliberately layered shape:

1. project identity and required target inputs;
2. defaults, compatibility catalog, and named preset values;
3. environment overlays with value provenance; and
4. provider-owned semantic options and namespaced external integration options.

The first layer is intentionally small. The fourth layer is where an advanced
operator can select a provider-supported capacity mode, service major,
managed/self-hosted boundary, topology, replacement policy, retention rule, or
existing-resource identity. These are typed fields with pre-mutation validation,
not arbitrary Pulumi or provider SDK property bags. The resolver emits the
effective values and provenance; the provider planner maps them into the
provider-neutral architecture boundary and fingerprint.

This preserves a simple path for most users without making the advanced path a
second implementation of the lifecycle. Unknown YAML fields remain errors,
secret values remain references, and an adapter that cannot implement an
advanced choice returns a typed unsupported or blocked result before mutation.
Static validation is not sufficient for account- and region-dependent
availability. The core therefore exposes one optional, read-only plan-admission
port. It runs after static planning and before Pulumi backend or provider
resource creation; adapters may resolve provider-owned canonical zones or
catalog values, but the core checks that stable plan identity is unchanged.
Admission fails closed when the provider capability lookup is unavailable or
the requested combination is not currently admitted. Destroy and cleanup skip
admission so a temporary capability outage cannot strand resources. OVHcloud's
adapter uses the current region capability and managed-database availability
endpoints for its MKS, MySQL, Valkey, network, and node-count checks. The same
port is public in `sdk/v1` so community adapters can implement this behavior
without leaking provider SDK types into the core.
The same semantic boundary applies to provider-specific durability controls:
AWS RDS/Aurora and ElastiCache snapshot policy, GCP Cloud SQL and Memorystore
policy, Scaleway RDB policy, and OVH managed database policy are YAML choices
when the owning SDK supports them. Omitted values retain named safety defaults;
explicit zero and false values are preserved, and provider-plan-controlled
settings are documented rather than exposed as fake portable knobs.
The simple GCP standard and high-availability presets use eight retained Cloud
SQL backups plus seven days of transaction-log retention, while preview turns
those two recovery features off. Scaleway standard and high-availability use
daily, seven-day, same-region encrypted RDB backups, while preview turns
automated backups off. These defaults are materialized in effective YAML and
can be overridden without changing the core recovery contract.
AWS ECS and EKS presets follow the same rule: their task/node capacity,
managed database/cache/search shape, queue mode, workload replicas, and
retention are materialized in the provider catalog rather than being inferred
inside a Pulumi component. This keeps the minimal YAML path complete while
leaving every supported semantic capacity choice available to advanced YAML.
AWS preview explicitly materializes zero Valkey snapshot-retention days, while
standard and high-availability materialize seven days. The environment-class
database deletion-protection and automated-backup cleanup choices are also
materialized after overlays so a production safety boundary cannot be hidden in
an adapter fallback. GCP Cloud Armor follows the same rule: it is materialized
for durable presets and for production even when production selects preview
capacity, with an explicit YAML value taking precedence. When AWS fck-nat is
selected, the resolver also materializes the preview or durable-preset topology,
single-instance versus automatic-replacement policy, and the ARM64 `t4g.nano`
default; explicit topology, replacement, and instance-size values remain
overrideable in YAML.

### 3. Model failure domains and data ownership explicitly

Each profile names its failure domains: process, task or pod, node, zone,
region, provider account, edge provider, observability provider, and operator
credentials. Each data class has a source of truth and a recovery method:

| Data or state | Source of truth | Recovery rule |
| --- | --- | --- |
| Magento database | Managed or self-hosted database | PITR or snapshot restore, then checksum and application verification |
| Media and object data | Versioned object storage | Version restore or replicated-object recovery with manifest checksums |
| Configuration and secrets | Versioned state/config plus secret manager | Restore references and rotate credentials where required |
| Infrastructure state | Encrypted state backend and backup | Restore state only with ownership and provider identity checks |
| Queue messages | Broker or database messaging | Provider-specific durability and replay policy; no silent loss claim |
| Search indexes | Rebuildable projection | Rebuild from durable sources; measure rebuild RTO |
| Cache contents | Reconstructible acceleration layer | Never treat cache as the durable backup |

Alternative rejected: back up the whole stack as one opaque snapshot. It
cannot establish independent restore guarantees or prevent a corrupted backup
from being promoted.

### 4. Make resilience profiles policy inputs, not hardcoded promises

The profile schema will accept availability, RPO, RTO, retention, recovery
scope, and failover ownership. Initial implementation can provide named
profiles such as `development`, `production-ha`, and `regional-dr`, but the
profile must expose the actual targets and assumptions. A plan that cannot
meet a target fails before apply; a live result that misses it remains
non-certifying and records the measured value.

Disaster recovery is staged:

1. Backup and restore in the same region.
2. Zone or node failure for HA.
3. Alternate-region recovery.
4. Alternate-provider recovery only where the data and service contracts make
   it meaningful.

Each stage has a separate evidence gate. Passing stage 1 does not imply stage
3 or stage 4.

### 5. Keep native and external integrations as independent capabilities

The runtime profile composes an origin, native edge, external edge, native
observability, and external observability. AWS CloudWatch and CloudFront/WAF,
Google Cloud Observability and Google Cloud edge services, and the current
Scaleway or OVHcloud products are provider-owned capabilities. Fastly and New
Relic are extension-owned capabilities that can front or observe any origin
only when their credentials, routing, signal, and cleanup contracts pass.

New Relic will use a typed signal contract with provider-specific integration
selection first and OpenTelemetry/exporter fallback second. Fastly will use
typed origin, domain, TLS, purge, WAF, and routing intent. Neither vendor's
raw object schema becomes portable core configuration.

First-party native resource graphs carry the same portable observability intent
into provider adapters. AWS owns CloudWatch resources; GCP owns GKE collection
and Cloud Monitoring objects; Scaleway owns Cockpit data sources; and OVH owns
the documented MKS Kubernetes-audit subscription only when an existing Logs
Data Platform stream identity is supplied. A graph or preview is not delivery
proof: signal arrival, labels, retention, alert firing, redaction, and direct
cleanup remain live evidence classes. An opaque native destination reference
is identity-only and never a substitute for a provider field or credential.

Alternative rejected: make New Relic or Fastly a mandatory dependency of every
provider. Users must be able to operate with native services, while the
release matrix still verifies the external combinations that MageLift claims.

### 6. Use provider adapters for backup, recovery, telemetry, and edge ports

Provider adapters remain responsible for provider APIs and asynchronous
operations. Shared contracts define the observable behavior:

- discover and validate capability;
- plan without side effects;
- apply with ownership markers;
- observe health and operation progress;
- back up, restore, fail over, and fence where supported;
- configure or query native observability;
- configure and verify native edge;
- destroy and assert direct inventories.

The core never assumes that an AWS API shape applies to GCP, Scaleway, or OVH.
Provider-specific unsupported operations return typed status and diagnostics,
not silent no-ops.

The public `sdk/v1` contract owns the deterministic recovery graph compiler
(`CompileResiliencePlan`). First-party and community adapters provide a
descriptor of executable data-class actions and destinations, then execute the
returned stages through native APIs. This keeps backup/fence/restore/integrity/
failover/cleanup ordering, approval boundaries, idempotency, and required proof
names in one place without importing `internal/` packages into a community
provider.

The same boundary applies to live verification. Observability adapters return
semantic signal, operational-object, and cleanup observations through injected
probes; the core does not infer delivery from a configured exporter. Edge
adapters fail closed until origin and route health are observed, and cleanup
must poll the owning service inventory until deletion is complete or report a
bounded delayed tombstone. These probes keep provider SDK response shapes in
adapters while making evidence and release gates deterministic.

Planned module factories are the dependency-injection seam for first-party and
community implementations. `sdk/v1` exposes optional factories for resilience,
native edge, and native observability; the internal platform resolves them only
after a `PlannedStack` has passed target validation. The factory may use the
module-owned opaque plan to construct a regional SDK client, but it must not
create resources. Provider packages can therefore inject AWS, GCP, Scaleway,
or OVH operation clients without putting their SDK types in the core or
duplicating the lifecycle graph. A nil result is an explicit absence of that
target-owned port. Fastly and New Relic remain independent external adapters:
their destination identity is not forced to equal the origin provider.

### 7. Make certification a dependency graph with reusable fixtures

The scheduler will compile a coverage graph from architecture profiles. A
vertex is a cold boundary; an edge is an allowed warm transition. The graph
includes service majors, managed/self-hosted boundaries, resilience profile,
observability destination, edge path, artifact, schema, and migration. One
immutable artifact and scrubbed restore fixture are built per compatible
runtime contract.

Safe parallelism is limited to independent provider groups after quota,
credential, state, local resource, and ownership checks. Within one provider
project or account, shared networks, service-connection policies, databases,
restore fixtures, and cleanup scopes serialize mutations. Slow creation and
deletion are amortized by create-once sessions and bounded warm transitions;
they are not hidden by weakening cleanup.

The scheduler executes the compiled batches through one provider callback. It
writes `IN_PROGRESS`, `PASS`, and `FAIL` checkpoints for every cell covered by
an execution unit, and it cancels later dependency batches after a failure.
Provider operation IDs remain opaque and are carried only for resume; the
scheduler never interprets or logs provider credentials.

### 8. Separate evidence classes

Evidence distinguishes:

- offline contract and catalog validation;
- provider API and mock evidence;
- cold application certification;
- warm service transition evidence;
- backup/restore evidence;
- HA failure evidence;
- regional DR evidence;
- external edge evidence;
- native and New Relic observability evidence;
- cleanup and direct inventory evidence.

The release gate requires the evidence class named by the claim. A warm queue
pass cannot close DR. A backup creation pass cannot close restore. A Fastly
route pass cannot close origin failover. A native log stream cannot close
traces or New Relic integration.

### 9. Make recovery and cleanup resumable

Checkpoints will persist the architecture fingerprint, provider operation
IDs, backup and restore IDs, edge identities, telemetry identities, ownership
markers, completed stages, and cleanup state. Interruption resumes the same
scope; it never creates a second stack because a provider deletion is slow.
Direct owning-service inventories remain cleanup truth, while tag indexes and
provider tombstones are recorded separately.

### 10. Use one checkpointed failure-drill state machine

The certification core owns the failure-drill state transitions
`injected → observed → cleanup-pending → complete`. A provider implementation
only supplies three operations: inject the declared fault, observe the
provider-neutral traffic/integrity/fencing result, and clean up the exact
injection scope. The checkpoint binds all three operations to the architecture
fingerprint, scenario ID, and ownership marker. An interrupted process can
therefore retry observation or cleanup without creating a second fault or a
second writer.

An expected resilience miss is evidence with `FAIL`, `SKIP`, `BLOCKED`, or
`NOT_RUN` status and an explicit reason; it is not a Go executor error. API,
checkpoint, and persistence failures remain errors so the same checkpoint can
be retried. Cleanup is recorded separately from traffic and integrity proof,
which prevents a successful failover observation from being mistaken for
safe teardown.

## Risks / Trade-offs

- [The matrix grows too large] → Certify declared cold boundaries and selected
  warm transitions, generate explicit unsupported or unavailable rows, and
  require a coverage report before live apply.
- [Provider capabilities change] → Store source URL, retrieval date, service
  major, region, and API evidence; invalidate affected profiles when the
  source or provider inventory changes.
- [DR testing damages data or creates duplicate writers] → Use scrubbed
  fixtures, isolated recovery destinations, fencing, ownership markers, and
  explicit operator approval for production-like cutovers.
- [New Relic or Fastly adds cost or egress] → Make them independent profile
  capabilities, show planned cost and signal egress, and require budgets before
  apply.
- [Native provider observability differs too much] → Keep a common signal and
  SLO contract, but permit provider-specific implementations and explicit
  unavailable signals.
- [Cloud deletion remains asynchronous] → Use per-provider operation polling,
  bounded retries, delayed-tombstone status, and a new ownership scope only
  after direct inventory is empty.
- [Valkey 9.0 implementation differs from the previous Valkey 8 fixture] →
  keep the GCP adapter and catalog explicit, run compatibility and smoke tests
  first, then repeat only the GCP cold boundaries affected by the cache major
  before broader HA/DR certification.

## Migration Plan

1. Freeze and source-date the provider capability registry and Adobe matrix;
   keep the GCP adapter on Valkey 9.0 for 2.4.9 and track 9.1 as Preview.
2. Add architecture, resilience, observability, and edge profile validation
   with no provider mutation.
3. Add provider contract tests and local restore, failover, telemetry, and
   edge tests using scrubbed fixtures.
4. Implement or reconcile AWS ECS, AWS EKS, GCP GKE, Scaleway Kapsule, and OVH
   MKS adapter ports, recording unavailable boundaries rather than faking them.
   Add provider-side read-only admission for each first-party catalog so
   advanced YAML cannot create a paid resource before the provider has admitted
   its region, topology, network, capacity, and service-major combination.
5. Implement native provider observability and edge paths, then New Relic and
   Fastly extension paths with secret references and ownership-scoped cleanup.
6. Build the coverage graph, scheduler, checkpoint resume, and create-once
   session behavior. Run offline gates before consuming cloud credits.
7. Run one cold baseline per declared architecture boundary, then warm service
   transitions, restore drills, HA failures, regional DR drills, observability
   checks, edge failover, and final cleanup according to the profile.
8. Regenerate capability, release, and evidence documents from passing
   generated records. Do not close a profile because a neighboring profile
   passed.

Rollback is profile- and adapter-scoped. Existing single-provider acceptance
  paths remain available while the new contracts are introduced. A failed
  recovery or cleanup stage blocks the affected profile and retains only the
  exact operator-approved debug resources.

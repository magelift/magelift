## Purpose

Defines how one YAML configuration serves both the simple first-run path and
advanced provider architecture choices without leaking provider SDK objects
into the portable core.

## ADDED Requirements

### Requirement: Configuration has progressive disclosure levels

The first-release YAML contract MUST support a simple path based on project
identity, target provider/runtime, preset, and the small set of required
credentials or existing-resource references. Omitted optional choices MUST
resolve to documented, named defaults appropriate to the selected provider,
runtime, preset, and region rather than to an undocumented SDK default.

Environment overlays MUST be able to override or remove optional project values
without copying the complete project configuration. The effective result MUST
retain provenance for every resolved value.

#### Scenario: A normal user selects a supported preset

- **WHEN** a user supplies the required application, target, region, preset, and
  secret references but omits provider capacity and topology details
- **THEN** MageLift resolves the documented preset catalog, records the source
  of each resolved value, and exposes the effective configuration before any
  provider mutation

The first-party standard and high-availability GCP presets MUST resolve Cloud
SQL automated backups and MySQL binary logging with eight retained automated
backups and seven days of transaction-log retention. The preview GCP preset
MUST explicitly disable both features. The first-party standard and
high-availability Scaleway presets MUST resolve daily RDB backups, seven-day
retention, same-region backup placement, and encryption at rest; the preview
Scaleway preset MUST explicitly disable automated RDB backups. These are
named, provider-native defaults and remain individually overrideable in YAML.
The first-party AWS ECS presets MUST likewise materialize task capacity mode,
CPU, memory, managed database/cache/search capacity, queue shape, and retention;
the AWS EKS presets MUST materialize compute mode, Kubernetes version, workload
requests/replicas, and the shared managed database/cache shape. These defaults
MUST be selectable or overridden through the same typed YAML fields for ECS and
EKS; preview MUST explicitly disable Valkey automatic snapshots, while standard
and high-availability MUST materialize the named seven-day snapshot policy. The
effective AWS configuration MUST also materialize the environment-class database
deletion and automated-backup cleanup policy used by the adapter. A provider
adapter MUST NOT require an undocumented capacity or safety fallback.

The GCP Cloud Armor default MUST be materialized as part of the effective
configuration: durable presets enable it, and a production environment using a
preview capacity preset still enables it unless YAML explicitly overrides it.

### Requirement: Advanced YAML choices are semantic and provider-owned

Advanced fields MUST represent supported semantic architecture choices such as
capacity mode, managed versus self-hosted boundary, topology, service version,
replacement policy, existing-resource reference, retention, or provider-owned
integration identity. A provider adapter MUST translate those choices into its
SDK calls. Raw provider SDK argument maps, unvalidated arbitrary resource
properties, and provider SDK types MUST NOT become portable core fields.

Provider-specific advanced fields MUST remain under their target provider
namespace. Fastly, New Relic, and community integration options MUST remain in
their namespaced extension or typed adapter boundary. Unknown fields in
core-owned typed sections MUST fail configuration loading instead of being
silently passed through. An opaque `extensions.<namespace>` payload MAY be
accepted by the core loader so community adapters can own its schema, but the
registered adapter MUST strict-decode and validate it before planning or
mutation; an unregistered or unsupported extension is a stable blocked result.

#### Scenario: An operator selects advanced AWS fck-nat behavior

- **WHEN** YAML selects `natMode: fck-nat`, `natTopology: multi-az`,
  `natReplacementMode: auto-scaling`, and an optional ARM64 Graviton
  `natInstanceType`
- **THEN** the resolved AWS plan contains those semantic choices, the plan and
  architecture fingerprint change when any choice changes, and the AWS adapter
  alone translates them into fixed network interfaces, replacement groups,
  routes, and IAM permissions; omitted topology, replacement, and sizing
  resolve to the documented preview or durable-preset policy and cost-
  optimized ARM64 default in effective YAML, while an incompatible
  architecture is rejected

#### Scenario: An operator selects advanced managed Kubernetes capacity

- **WHEN** YAML selects GKE Standard instead of Autopilot, or selects an EKS
  compute mode such as Auto Mode, managed node groups, self-managed nodes, or
  Fargate, and supplies the relevant Kubernetes version, node shape, storage,
  replica, workload, or scheduling fields
- **THEN** the resolved provider-neutral plan retains those semantic choices,
  the selected GCP or AWS adapter translates only its own fields into the
  provider SDK, and a field that is invalid for the selected runtime is
  rejected before mutation rather than silently ignored

#### Scenario: An operator selects the workload target for projection recovery

- **WHEN** YAML supplies `resilience.projection.runtime: ecs` with an ECS
  cluster, service (or an explicitly pinned task), and container, or supplies
  `runtime: kubernetes` with a namespace, workload, and optional container
- **THEN** the resolved configuration records the semantic workload identity
  and provenance, ECS resolves a service target to a running task at operation
  time, Kubernetes resolves a ready pod from the declared workload, and no
  credential, raw SDK request, or verifier implementation enters YAML or
  portable recovery evidence; incompatible fields fail before mutation

#### Scenario: An operator customizes a non-AWS provider without raw SDK fields

- **WHEN** YAML supplies GCP Cloud SQL/Memorystore/GKE settings, Scaleway
  RDB/Redis/Kapsule settings, or OVH database/Valkey/MKS settings under the
  matching provider namespace
- **THEN** the effective configuration, provenance, fingerprint, provider
  plan, and Pulumi graph reflect the values, while omitted values come from the
  selected provider/preset catalog and unknown SDK-shaped fields are rejected

#### Scenario: An AWS operator customizes managed-service durability

- **WHEN** YAML supplies AWS RDS/Aurora backup and maintenance windows,
  database deletion-protection or automated-backup deletion policy, or
  ElastiCache Valkey snapshot retention and window under
  `target.aws.catalog`
- **THEN** the AWS adapter maps those semantic choices to the selected RDS
  engine shape or Valkey replication group, omitted values retain the existing
  environment-aware safety defaults, explicit boolean false and zero values
  remain distinguishable from omission, and unsupported or malformed values
  fail before Pulumi registers a provider resource

#### Scenario: An operator selects managed-service failure-domain shape

- **WHEN** YAML selects GCP Memorystore shard count, replicas per shard,
  cluster mode, and single- or multi-zone placement; Scaleway RDB HA and Redis
  standalone or two-node HA; or OVH MySQL and Valkey node counts
- **THEN** the selected provider adapter receives the exact semantic topology,
  the effective configuration and architecture fingerprint include it, and
  invalid combinations such as GCP Cluster Mode Disabled with more than one
  shard are rejected before provider mutation

#### Scenario: An operator customizes an OVH MKS failure-domain topology

- **WHEN** YAML supplies `target.ovh.mksPlan`, `target.ovh.zones`, and
  `target.ovh.nodeCount` for a Free or Standard MKS cluster
- **THEN** the resolved plan treats `zones` as MKS availability zones inside
  the selected region, creates one worker pool per zone with deterministic
  node distribution, and records the topology in provenance and the reuse
  fingerprint; a Free multi-zone request, duplicate or empty zone, or node
  count below the number of zones is rejected before mutation
- **AND** `target.ovh.databaseNodeCount` and the selected managed-database
  region remain independent from MKS worker zones, so YAML cannot accidentally
  claim database 3-AZ durability by listing unrelated Kubernetes zones

#### Scenario: An operator selects only currently documented OVH managed-service shapes

- **WHEN** YAML selects `target.ovh.databaseVersion`, `databasePlan`,
  `databaseNodeCount`, `valkeyVersion`, `valkeyPlan`, and `valkeyNodeCount`
- **THEN** the schema and OVH adapter expose the current source-dated choices:
  MySQL 8.0/8.4 with one, two, or three nodes for
  Essential/Business-Production/Enterprise-Advanced plan values, and Valkey
  7.2/8.0/8.1 from the official capability page plus 9.0/9.1 currently
  returned by the authenticated availability catalog, with one or two nodes
  for Essential/Business-Production plan values; the effective configuration,
  provenance, and fingerprint retain the selected values
- **AND** an unsupported version, plan, or plan/node-count combination is
  rejected before provider mutation rather than being forwarded as an
  unvalidated SDK field

#### Scenario: Provider availability admission runs before paid mutation

- **WHEN** a resolved plan contains provider-owned regional, zone, managed
  service, network, capacity, or integration choices whose validity depends on
  the current provider account or catalog
- **THEN** the core invokes the provider's optional read-only plan-admission
  port after static plan validation and before creating a Pulumi backend,
  provider resource, or paid managed service; the adapter may resolve
  provider-owned canonical values but MUST preserve the core plan identity
- **AND** the adapter fails closed with a stable diagnostic when its capability
  endpoint is unavailable or the selected combination is not currently
  admitted; destroy and cleanup MUST remain possible when admission is
  unavailable
- **AND** for OVHcloud, admission checks the selected region and MKS plan and
  then matches MySQL and Valkey engine, version, plan, flavor, region, private
  network, and node-count values against the current
  `/cloud/project/{serviceName}/database/availability` response; the source-
  dated capability page and live availability catalog may expose different
  version sets, so the live catalog is authoritative for the selected account
  and region and a stale or unavailable response fails closed
- **AND** a community adapter may implement the same SDK admission interface
  without adding provider SDK types or raw argument maps to the core

#### Scenario: An operator customizes managed-service durability and deletion safety

- **WHEN** YAML supplies GCP Cloud SQL automated-backup enablement, MySQL binary
  logging, provider-native retained-backup count, transaction-log retention,
  backup start time or location, and Cloud SQL or Memorystore deletion
  protection; Scaleway RDB backup enablement, frequency, retention, same-region
  placement, or encryption at rest; or OVH MySQL or Valkey backup time,
  destination regions, or deletion protection
- **THEN** the effective configuration preserves omitted-versus-explicit values,
  validates provider-native units and ranges, the selected adapter maps each
  supported value to its SDK resource, and the plan, fingerprint, evidence, and
  cleanup policy reflect the resulting durability boundary; a setting that the
  provider plan controls or that the runtime cannot honor is not presented as a
  portable generic field

#### Scenario: A managed-service security mode exceeds the application contract

- **WHEN** an operator requests a provider cache authentication or transport
  encryption mode for which the current Magento endpoint and credential contract
  has no end-to-end connection, secret, certificate, health, and recovery
  implementation
- **THEN** planning returns a stable typed unsupported diagnostic before
  resource mutation; the YAML contract does not expose a misleading switch that
  would provision a cache the application cannot safely use

#### Scenario: An operator requests a provider topology beyond the core connector contract

- **WHEN** YAML selects a Scaleway Redis cluster-mode shape with three to six
  nodes while the current Magento endpoint contract only supports one endpoint
  or a two-node failover topology
- **THEN** planning returns a stable unsupported diagnostic before provisioning
  and does not silently create a partitioned cache that the application cannot
  address; the field remains available for a future cluster-aware connector
  implementation

#### Scenario: A raw SDK field is mistaken for an advanced setting

- **WHEN** YAML contains an unknown provider SDK property under a typed target
  namespace that MageLift has not declared
- **THEN** configuration validation rejects it before planning and does not
  register a provider resource

#### Scenario: A community adapter owns an advanced extension payload

- **WHEN** YAML contains `extensions.community.network` and the explicitly
  registered community adapter declares that namespace
- **THEN** the core preserves the payload as opaque configuration, the adapter
  strict-decodes its own schema before planning, and the shared lifecycle still
  owns validation ordering, fingerprints, evidence, and cleanup

### Requirement: Resolved configuration is reproducible and secret-safe

The effective configuration, provider-neutral plan, architecture fingerprint,
checkpoint, and certification evidence MUST include resolved semantic choices,
their versioned catalog identity, and ownership boundaries. The core MUST carry
a fingerprint of the fully resolved configuration after sensitive values have
been redacted; this is a fingerprint input, not a provider SDK property bag.
Secret values MUST remain secret references and MUST NOT be copied into
effective output, fingerprints, logs, or evidence.

Changing an explicit advanced value, a preset default, a provider service
major, a region or zone set, an existing-resource boundary, or an adapter
implementation version MUST invalidate reuse when that value can affect
resource shape, recovery, observability, edge behavior, or cleanup.

#### Scenario: An environment overrides one advanced value

- **WHEN** a staging overlay changes only the AWS NAT topology from the project
  default
- **THEN** the effective configuration shows the overlay as the provenance,
  the resolved plan uses the new topology, and the certification scheduler
  treats it as a separate cold architecture boundary

#### Scenario: An external edge needs an origin safety proof

- **WHEN** YAML selects an external Fastly edge and supplies an `edge.health`
  policy with the expected route target, optional origin URL, route path, status,
  and convergence budget
- **THEN** the core carries that typed health intent to the adapter, a normal
  deployment may resolve the origin URL from the provisioned application output,
  standalone edge apply requires an explicit origin URL, and the adapter refuses
  mutation when it cannot construct a real health probe

### Requirement: Provider adapters preserve the core lifecycle contract

Provider adapters MUST implement the same provider-neutral validation,
ownership, idempotency, recovery ordering, evidence gates, and cleanup
semantics. Adding a provider or community adapter MUST NOT require a second
copy of core lifecycle logic or a new legacy provider selector.

An advanced choice that the adapter cannot validate, provision, observe, recover,
or clean up MUST be classified as unsupported or blocked before mutation with a
stable diagnostic; it MUST NOT degrade silently to a nearby default.

#### Scenario: An adapter cannot implement an advanced semantic choice

- **WHEN** YAML selects a provider-owned topology or durability option that
  the registered adapter cannot validate, provision, observe, recover, or
  clean up
- **THEN** planning returns a stable unsupported or blocked diagnostic before
  provider mutation, includes the affected semantic field and provider, and
  does not substitute a nearby default or invoke a second core lifecycle
  implementation

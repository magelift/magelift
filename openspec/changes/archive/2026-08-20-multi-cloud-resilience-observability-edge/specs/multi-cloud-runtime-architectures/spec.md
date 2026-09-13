## Purpose

Defines the supported production architecture families and provider service
boundaries for AWS, GCP, Scaleway, and OVHcloud without treating a nearby
topology or wire-compatible service as proof of support.

## ADDED Requirements

### Requirement: Architecture profiles are complete

Every supported or evaluated architecture MUST be represented by a profile
that identifies provider, region or region set, compute runtime, Kubernetes
topology where applicable, managed or self-hosted stateful services, service
majors, network and ingress mode, edge mode, observability mode, backup mode,
recovery mode, edition, release, artifact, and declared RPO/RTO targets.
An omitted field MUST resolve to an explicit named profile value, never to an
undocumented provider default.

#### Scenario: An operator selects an HA EKS profile

- **WHEN** the operator selects EKS with managed control plane, multi-zone
  nodes, managed database and cache, self-hosted queue, Fastly edge, and New
  Relic telemetry
- **THEN** the plan records each boundary, service major, region set,
  resilience profile, edge capability, and observability capability before
  any provider mutation

### Requirement: Generic behavior lives in the core SDK boundary

The portable core MUST own architecture and service intent validation,
capability classification, ownership markers, recovery graph construction,
idempotency, scheduling, evidence gates, and cleanup semantics exactly once.
The SDK MUST expose the recovery graph compiler so adapters do not reimplement
backup, fencing, restore, integrity, failover, or cleanup ordering.
Provider implementations MUST expose native API calls only through the
versioned SDK contracts; provider SDK types, product-specific lifecycle rules,
and provider configuration MUST remain inside the provider adapter. A
community-maintained provider MUST be able to implement the same contracts
without importing MageLift `internal/` packages or changing core behavior.

Provider-owned resource identities MAY be carried as opaque
`ServiceBoundaryIntent.ResourceReference` values. The value MUST remain
single-line, non-secret identity data; provider adapters MUST resolve its
meaning without adding vendor fields to the portable contract.

Provider modules MAY expose planned adapter factories through the versioned
SDK. A factory MUST receive the validated provider-neutral plan, MUST construct
provider SDK clients without mutating provider state, and MUST return the
provider-neutral resilience, edge, or observability adapter for the selected
target. The core MUST validate the returned descriptor and retain ownership,
idempotency, scheduling, evidence, and cleanup semantics. A nil factory result
means that the module does not own that lifecycle for the selected plan.

External edge and observability adapters remain independently owned. Their
descriptor provider MAY be Fastly, New Relic, or a community destination rather
than the origin provider; the target provider and external destination are
carried separately in the typed intent and plan request.

#### Scenario: A new provider implements a managed cache

- **WHEN** a community adapter adds a cache implementation with a new native
  SDK
- **THEN** it declares provider-neutral target, resilience, observability, and
  edge capabilities through the public SDK, while the core applies the same
  validation, ownership, scheduling, evidence, and cleanup rules as first-party
  adapters

### Requirement: First-release configuration has no legacy singular provider fields

The portable configuration MUST reject `edge.provider` and
`observability.provider`. Edge and observability ownership MUST be represented
by independent `nativeProvider` and `externalProvider` fields so native cloud
services, Fastly, New Relic, and future community adapters can be composed
without a provider switch in the core. Provider fields on target and existing
resource identity records remain valid identity data and are not legacy edge or
observability selectors.

#### Scenario: A user selects Fastly and native CloudWatch together

- **WHEN** the configuration sets `edge.externalProvider: fastly` and
  `observability.nativeProvider: cloudwatch`
- **THEN** validation preserves both independent intents and no singular
  provider field is read, migrated, or silently preferred

### Requirement: Provider capability is current and dated

The capability catalog MUST distinguish Adobe compatibility, provider
availability, MageLift implementation status, and live certification status.
Provider service versions and regional availability MUST carry a source URL,
retrieval date, and provider account or project evidence when live. A stale
provider default MUST NOT downgrade a currently available compatible service or
silently upgrade an unverified service.

#### Scenario: A provider adds a supported service major

- **WHEN** current provider documentation exposes a service major that meets
  the selected Adobe release but the adapter still provisions an older major
- **THEN** the catalog reports provider capability as available, implementation
  support as incomplete, and certification as open until the adapter and live
  evidence are updated

### Requirement: AWS compute architectures are explicit

The AWS catalog MUST cover each declared ECS capacity mode, including Fargate
and any supported managed-instance mode, and full EKS operation including
control-plane, node, workload, storage, ingress, and managed-service choices.
ECS and EKS profiles MUST separately identify managed RDS or Aurora, managed
ElastiCache, managed search or self-hosted search, managed or self-hosted
queues, media storage, CloudWatch, CloudFront/WAF behavior, and the private
subnet egress mode. AWS egress profiles MUST distinguish managed NAT Gateway
from the explicit `fck-nat` NAT-instance path. An `fck-nat` profile MUST record
the ARM64 AMI ownership and version selection, instance type, public-subnet
placement, source/destination-check setting, route targets, failure domain,
replacement strategy, and whether it is a cost-optimized preview path or an
HA path. A single NAT instance MUST NOT satisfy an HA claim.

#### Scenario: ECS and EKS use different stateful boundaries

- **WHEN** one profile uses ECS Fargate with managed RDS and ElastiCache and a
  second uses EKS with a self-hosted queue and managed database
- **THEN** the profiles receive distinct compatibility, backup, HA, recovery,
  observability, and certification identities even when they deploy the same
  Magento artifact

#### Scenario: AWS fck-nat is an explicit cost and availability boundary

- **WHEN** an operator selects `natMode: fck-nat`
- **THEN** planning records the fck-nat AMI owner and architecture, creates
  only the selected NAT-instance resources, points private-subnet default
  routes at those instances or their stable network interfaces, and records
  the selected preview or HA replacement policy
- **AND** a preview profile may intentionally share one instance across its
  private subnets for cost, while a standard or HA profile MUST use an
  independent failure-domain strategy and MUST refuse to call a plain
  one-instance deployment highly available
- **AND** an existing-VPC profile MUST treat egress as operator-owned and MUST
  not create or delete fck-nat resources it does not own

### Requirement: GCP compute architectures use current managed services

The GCP catalog MUST cover GKE Autopilot and GKE Standard where the workload
requirements differ, and MUST represent Cloud SQL, Memorystore for Valkey,
search, queue, object storage, Google Cloud Observability, and native Google
edge capabilities as independent boundaries. A GCP 2.4.9 profile MUST be able
to select the current Adobe-compatible Valkey 9 service major when the provider
and adapter support it.

#### Scenario: GCP Valkey capability changes

- **WHEN** current Memorystore documentation supports Valkey 9.0 and the
  selected Adobe release requires Valkey 9
- **THEN** the provider capability catalog marks the service available, the
  plan selects `VALKEY_9_0` explicitly, and an old Valkey 8 evidence row cannot
  close the new profile without a fresh run

### Requirement: Community provider boundaries are honest

Scaleway Kapsule and OVHcloud MKS profiles MUST enumerate their managed
database, cache, search, queue, object, network, load-balancer, native edge,
and native observability capabilities from current provider evidence. A
missing provider equivalent MUST be reported as unavailable or experimental;
it MUST NOT be substituted with another provider's service without an explicit
compatibility classification.

#### Scenario: OVH lacks a required managed service in a region

- **WHEN** an OVH profile requests a service that the selected OVH region or
  adapter does not expose
- **THEN** validation stops before mutation and reports the exact unavailable
  boundary, alternative profile options, and its effect on the certification
  claim

### Requirement: Managed and self-hosted transitions are cold boundaries

A transition between managed and self-hosted database, cache, search, queue,
storage, ingress, observability, or edge services MUST invalidate a warm
session unless the catalog explicitly proves schema, data, credential,
network, and recovery compatibility.

#### Scenario: Managed database becomes self-hosted

- **WHEN** a user changes an HA profile from managed database to a Kubernetes
  database workload
- **THEN** the runner requires a new cold baseline, a new backup and restore
  proof, and new service-health evidence

### Requirement: Unsupported combinations fail before mutation

The plan MUST reject an architecture when Adobe requirements, provider
availability, MageLift implementation, security policy, or declared
resilience targets cannot be satisfied. The rejection MUST name the failing
dimension and MUST preserve an explicit escape hatch only when policy allows
an experimental non-certifying run.

#### Scenario: A profile requests an unsupported cache or topology

- **WHEN** a user requests a service major or topology outside the dated
  compatibility intersection
- **THEN** the plan returns a pre-mutation diagnostic and the release matrix
  records the profile as unsupported, unavailable, or blocked rather than
  silently selecting a nearby service

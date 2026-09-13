## Purpose

This capability provides real provider and SaaS lifecycle execution behind the
provider-neutral MageLift contract, so architecture support means executable,
observable, recoverable, and cleanable behavior rather than only a deployment
graph or a capability descriptor.

## ADDED Requirements

### Requirement: Provider-neutral lifecycle behavior is implemented once

The system SHALL keep planning, lifecycle ordering, ownership validation,
idempotency, cancellation, checkpointing, evidence validation, cleanup gates,
and typed unavailable/unsupported outcomes in the core SDK. Provider packages
MUST translate those semantic operations to their own SDK/API clients without
adding provider-shaped fields or provider SDK types to the public contract.

A first-party or community module MAY expose planned adapter factories. A
factory MUST receive a validated plan, MUST construct only provider-owned
clients and adapters, MUST perform no provider mutation while constructing, and
MUST return a descriptor whose provider identity matches the owned lifecycle.

#### Scenario: One core runner executes multiple provider clients

- **WHEN** AWS, GCP, Scaleway, and OVHcloud clients receive the same semantic lifecycle stage
- **THEN** the core applies the same validation, idempotency, polling, checkpoint, evidence, and cleanup rules while each client performs only its native API translation

#### Scenario: Community provider adds a lifecycle without core changes

- **WHEN** a community module implements the public SDK factory and adapter contracts
- **THEN** it can participate in planning and certification through the same core runner without importing `internal/cloud` or changing the portable evidence schema

#### Scenario: Factory is given an unrelated plan

- **WHEN** a factory receives a plan whose target ID, provider, or runtime does not match the registered module
- **THEN** the core rejects the lifecycle before the factory can create or mutate a provider client

### Requirement: Every declared architecture has an honest executable boundary

The system SHALL model and test these architecture families independently:
AWS ECS Fargate, Fargate Spot, EC2 Auto Scaling capacity, and ECS Managed
Instances; AWS EKS managed control plane with managed node groups, Auto Mode,
Fargate, and self-managed capacity; GKE Autopilot and Standard; Scaleway
Kapsule; and OVHcloud MKS. A PASS result for one family MUST NOT satisfy a
different family unless the fingerprint explicitly proves that all relevant
boundaries are identical.

Each family SHALL declare ownership, failure domains, stateful-service mode,
network and ingress mode, native and external observability, native and
external edge, resilience policy, artifact, migration, and fixture identity.
Provider gaps MUST be represented as `unavailable`, `unsupported`,
`blocked`, `experimental`, or `not-run` with a reason.

#### Scenario: AWS compute modes are not collapsed

- **WHEN** a certification schedule contains ECS Fargate, ECS EC2 capacity, EKS Auto Mode, and EKS managed-node-group cells
- **THEN** the scheduler creates distinct cold boundaries and does not reuse a stack, failure result, or certificate across those compute modes

#### Scenario: Fargate Spot tasks receive the full interruption grace period

- **WHEN** an ECS task is planned for the `FARGATE_SPOT` capacity provider
- **THEN** every Linux container definition sets `stopTimeout` to 120 seconds, the AWS-documented maximum and the duration of the Spot interruption warning ([Fargate capacity providers](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/fargate-capacity-providers.html), [ContainerDefinition API](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html)); regular Fargate and EC2 task definitions omit this Spot-specific override
- **AND** live certification still verifies SIGTERM handling, replacement capacity, task-state interruption events, data integrity, and cleanup independently of the task-definition setting

#### Scenario: GKE Autopilot and Standard differ

- **WHEN** an operation requires node-level configuration that Autopilot does not expose
- **THEN** the Autopilot profile returns a typed unsupported or unavailable result before mutation and the Standard profile remains independently eligible

#### Scenario: Scaleway Kapsule zones drive the worker topology

- **WHEN** a Scaleway Kapsule YAML profile declares multiple availability zones and a node count that can place at least one node in each zone
- **THEN** the provider adapter creates one worker pool per declared zone, distributes the requested nodes deterministically, and applies strict zone spreading to eligible replicated workloads; the zone list is part of the resolved plan and reuse fingerprint
- **AND** when the selected preset requires more zones than the YAML declares, or the node count cannot cover the declared zones, planning fails before Pulumi or a provider API mutates anything
- **AND** live multi-zone failure, managed-service recovery, and Magento runtime evidence remain independent certification gates

#### Scenario: OVHcloud MKS zones drive the worker topology

- **WHEN** an OVHcloud MKS YAML profile selects a Standard cluster with multiple
  documented availability zones and a node count that can place at least one
  worker in every declared zone
- **THEN** the provider adapter creates one node pool per declared zone, sends
  exactly one zone to each pool, distributes the requested nodes
  deterministically, and makes every workload depend on every pool; the zone
  list and node distribution are part of the resolved plan and reuse fingerprint
- **AND** a Free plan with more than one zone, a multi-zone plan with fewer
  workers than zones, duplicate, or empty zones fails before Pulumi or an OVH
  API mutates anything; a provider-incompatible zone is eligible only after a
  provider capability/admission check proves it, otherwise the profile is
  blocked before mutation
- **AND** managed database nodes remain in the selected OVH managed-database
  region; database 1-AZ versus 3-AZ durability is selected by the documented
  region and service plan rather than by pretending that MKS worker-zone names
  are database regions ([MKS node pools](https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/managing-nodes),
  [database deployment modes](https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison))
- **AND** live multi-zone failure, managed-service recovery, and Magento runtime
  evidence remain independent certification gates

#### Scenario: OVH managed-service versions and plan caps remain source-dated

- **WHEN** an OVH YAML profile selects MySQL or Valkey engine versions, plans,
  and node counts
- **THEN** the typed configuration and provider adapter accept only the current
  documented MySQL versions 8.0/8.4 and Valkey versions 7.2/8.0/8.1; MySQL
  Essential, Business/Production, and Enterprise/Advanced plans are capped at
  one, two, and three nodes respectively, while Valkey Essential and
  Business/Production plans are capped at one and two nodes
- **AND** unsupported versions, plan names, or plan/node-count combinations are
  rejected before Pulumi or an OVH API mutates anything, and the source-dated
  capability record is refreshed when OVH changes the documented set
  ([MySQL capabilities](https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities),
  [Valkey capabilities](https://docs.ovhcloud.com/en/guides/public-cloud/databases/redis-capabilities))

#### Scenario: Provider lacks a managed service

- **WHEN** a Scaleway or OVHcloud profile requests a managed queue or search service not present in the source-dated catalog
- **THEN** planning records the missing capability and refuses an executable durable-backup claim instead of silently substituting another provider's service

### Requirement: Durable data recovery is provider-backed and data-class complete

For every profile that claims recovery, the provider adapter SHALL implement
backup, restore, integrity verification, and cleanup for each declared durable
data class: database, media/object data, configuration and secret references,
infrastructure state, queue or broker data when declared, and append-only
certification evidence. Search indexes and caches MUST be represented as
rebuild/reconstruct operations unless the selected provider explicitly proves
durable backup semantics.

Every successful durable operation MUST provide an owning-service operation or
resource identity, ownership scope, idempotency proof, polling result,
retention proof, encryption proof, deletion-protection or immutability proof
where the profile requires it, and a normalized evidence record. Restore MUST
support both same-region/in-place and isolated-destination modes when the
profile declares them; destructive in-place restore and any second-writer path
MUST require an operator approval reference.

#### Scenario: Provider backup completes asynchronously

- **WHEN** a provider starts a backup and returns a pending operation
- **THEN** the adapter polls the owning provider API with bounded timeout and cancellation, records the operation ID and backup identity, and refuses PASS evidence until retention, encryption, protection, and ownership checks succeed

#### Scenario: Restore uses the known-content fixture

- **WHEN** an isolated restore completes for a profile that declares a recovery fixture
- **THEN** the adapter verifies manifest and content checksums, record/object counts, application reads, permissions, secret references, service health, and measured restore duration for every declared data class

#### Scenario: OVHcloud Secret Manager restores a protected version

- **WHEN** an OVHcloud profile selects the documented Secret Manager/OKMS path for configuration-secret recovery
- **THEN** the provider adapter reads only an ownership-bound active version, seals its value through the shared archive envelope into protected Object Storage, restores to an ownership-scoped versioned secret idempotently, verifies reachability without recording the value, and returns a typed pre-mutation refusal when the OKMS identity, archive protection, or ownership metadata is not proven

#### Scenario: Cache and search are reconstructible

- **WHEN** a profile loses its cache or search index
- **THEN** the recovery graph records reconstruction/rebuild and integrity checks separately from database or media backup proof, and a cache warmup cannot satisfy a durable-backup requirement

#### Scenario: Restore or recovery is unavailable

- **WHEN** the selected provider/client cannot implement a requested data class or destination
- **THEN** the core returns a typed capability error before provider mutation and the certification record contains the reason and unsupported destination

### Requirement: GCP Pub/Sub recovery uses an explicit snapshot and seek boundary

The GCP adapter SHALL translate queue backup and recovery through the official
Cloud Pub/Sub snapshot and seek API, while keeping Pub/Sub protobufs and client
types inside the provider package. A queue resource reference SHALL identify a
subscription, not a topic. Backup SHALL create or reuse a deterministic,
ownership-labeled snapshot, verify its source topic, fixture ownership,
service-managed encryption boundary, and expiry, and return the snapshot
identity as normalized evidence. Reuse SHALL be idempotent and SHALL refuse a
same-name snapshot whose ownership, class, or fixture labels do not match;
provider labels outside MageLift's ownership keys MUST be preserved because
the adapter MUST NOT overwrite a foreign or drifted resource.

The adapter SHALL support same-region restore by seeking the owned source
subscription to the verified snapshot and SHALL require the application queue
verifier before reporting message-count, consumer-read, permission, and health
evidence. It SHALL reject retention targets above the documented seven-day
snapshot boundary, isolated or alternate-region destinations, and a CMEK
requirement that the narrow adapter cannot verify. Cleanup SHALL list and
delete only ownership-labeled MageLift snapshots and SHALL treat already
deleted snapshots as clean.

#### Scenario: GCP Pub/Sub snapshot recovery is bounded and idempotent

- **WHEN** a GCP queue profile requests backup, restore, or integrity verification for an owned subscription
- **THEN** the adapter uses the official snapshot/seek API, reuses the deterministic snapshot on retry, verifies the source topic and expiry, seeks only the existing same-region subscription for restore, and returns normalized ownership, retention, encryption, fixture, permission, and health evidence

#### Scenario: GCP Pub/Sub recovery refuses unsupported or unsafe boundaries

- **WHEN** the profile requests a topic reference, retention above seven days, isolated or alternate-region queue restore, unverifiable CMEK, a foreign snapshot, or a same-name snapshot with a different fixture
- **THEN** the adapter returns a typed or validation error before creating, relabeling, seeking, or deleting a Pub/Sub resource

#### Scenario: GCP Pub/Sub cleanup is ownership-scoped

- **WHEN** a certification session cleans up its GCP queue resources
- **THEN** the adapter deletes only snapshots carrying the session ownership marker and queue class, preserves unmarked or differently owned snapshots, and reports provider inventory as the cleanup truth

### Requirement: AWS SQS recovery uses an explicit quiesced export boundary

The AWS adapter SHALL keep SQS request and response types inside the provider
package and SHALL expose queue recovery through a provider-local port around
the official AWS SDK. Because SQS does not provide a native snapshot,
writer-fencing, or cross-region replication primitive, the adapter SHALL
require an operator approval reference for every source export, SHALL inspect
the source ownership tags, encryption, retention, approximate in-flight and
delayed message counts, and SHALL refuse FIFO queues and an unquiesced source
before creating a recovery queue. A queue retention target above the
documented fourteen-day SQS boundary MUST be rejected before mutation.

The supported boundary SHALL be a bounded standard-queue export into a
deterministic ownership-tagged SQS queue. The adapter MUST preserve source
messages by using visibility changes rather than deleting them, require a
stable SQS `MessageId` for every copied message, use batches of at most ten,
inspect every `SendMessageBatch` partial failure, and seal the export with an
ownership-scoped completion marker only after the copy and source visibility
reset succeed. A foreign or drifted deterministic queue MUST be refused
without tag, message, or deletion mutation. The export is a bounded,
fixture-verifiable recovery boundary; it MUST NOT be represented as an atomic
whole-queue snapshot or as proof of zero-loss behavior while writers or
consumers are not fenced.

Restore SHALL replay only into a new isolated queue in the same region. It
MUST reject in-place and alternate-region destinations, refuse an unsealed or
foreign export, refuse an existing partial restore without a completion
marker, and require the application verifier before reporting message counts,
known-content reads, permissions, and service health. Inventory and cleanup
MUST list and delete only deterministic MageLift export/restore queues whose
ownership and queue class tags match the requested marker; source queues and
unmarked or differently owned queues MUST be preserved.

#### Scenario: AWS SQS export preserves the source and is idempotent

- **WHEN** an approved, owned, encrypted, quiescent standard queue is backed up within the configured message limit
- **THEN** the adapter reuses the deterministic sealed export on retry, copies each observed `MessageId` at most once per operation, checks batch success, resets source visibility, returns the export identity, and leaves source messages available

#### Scenario: AWS SQS recovery refuses unsafe boundaries

- **WHEN** the request lacks quiescence approval, observes in-flight or delayed messages, exceeds fourteen-day retention or the configured message limit, finds a foreign/different fixture queue, requests FIFO/in-place/alternate-region recovery, or encounters a partial batch failure
- **THEN** the adapter returns a typed or validation error before claiming recovery proof and never mutates a foreign queue or deletes source messages

#### Scenario: AWS SQS isolated replay and cleanup are bounded

- **WHEN** a sealed export is restored or integrity-checked
- **THEN** the adapter replays only into the deterministic same-region isolated queue, requires application-owned known-content evidence, and cleanup removes only owned export/restore queues while the owning-service inventory remains the cleanup truth

### Requirement: HA, regional DR, fencing, and failure safety are executable

The system SHALL provide provider-backed exercises for process/task/pod loss,
node loss, zone loss, traffic and topology failure, queue/database/cache/search
behavior, regional loss, partial restore, corruption, failed deployment,
credential loss, provider outage, and interrupted teardown wherever a profile
claims those guarantees. A recovery path that can create a second writer MUST
implement fencing, single-writer protection, split-brain detection, approval,
stale-origin prevention, failback, and route convergence evidence.

Alternate-provider recovery SHALL be offered only when the profile proves
compatible identity, data, network, edge, service, and ownership contracts;
otherwise the result MUST be a typed unsupported reason rather than an implied
portability promise.

#### Scenario: Zone failure is injected

- **WHEN** a certification harness terminates or isolates a process, task, pod, node, or zone within the declared ownership scope
- **THEN** the adapter verifies traffic health, topology constraints, data-integrity behavior, queue/database semantics, and measured recovery time against the selected profile

#### Scenario: Regional failover starts

- **WHEN** an operator approves an alternate-region recovery
- **THEN** the old writer is fenced before the new writer accepts writes, edge traffic moves only after origin health is verified, RPO/RTO are measured, and failback records the final single-writer state

#### Scenario: Fencing cannot be proven

- **WHEN** a provider outage or stale origin prevents proof that the old writer is fenced
- **THEN** the system blocks traffic promotion and records a blocked recovery without mutating the second runtime into an accepting writer

### Requirement: Native observability is independently lifecycle-managed

The system SHALL implement native observability lifecycle behavior for the
declared provider paths: AWS CloudWatch and container/Kubernetes integrations,
Google Cloud Logging/Monitoring and GKE integrations, Scaleway Cockpit, and
the documented OVHcloud MKS audit-log forwarding to Logs Data Platform. Each
profile SHALL explicitly state which logs, metrics, traces, audit events,
application health, provider operations, recovery signals, edge signals, and
cleanup signals are supported or unavailable.

Native observability MUST verify delivery, identity labels, retention,
redaction, actionable alert behavior, dashboards, SLOs, and post-destroy
inventory. An application exporter or Pulumi resource graph alone MUST NOT be
reported as live observability certification.

#### Scenario: Native CloudWatch profile is verified

- **WHEN** an AWS ECS or EKS profile applies its CloudWatch observability plan
- **THEN** the adapter verifies the declared logs, metrics, alarms, dashboards, audit signals, labels, retention, redaction, and cleanup through CloudWatch-owned identities

#### Scenario: GKE Autopilot native observability is selected

- **WHEN** a GKE Autopilot profile requests native observability
- **THEN** the adapter accounts for the provider-managed Cloud Logging and Cloud Monitoring boundary, records unsupported disablement or node-level signals explicitly, and verifies the signals that the profile declares

#### Scenario: GCP SLO uses an existing Monitoring Service

- **WHEN** a GKE Autopilot or Standard profile declares an `application-health` SLO and supplies an opaque native reference to an existing Google Cloud Monitoring Service
- **THEN** the GCP adapter uses the official Service Monitoring SLO API or provider resource, validates a 1–30 day rolling window and the provider goal limit before mutation, owns the deterministic SLO identity, refuses foreign or drifted resources, and uses plan-scoped inventory for verification and cleanup

#### Scenario: GCP SLO has no existing service reference

- **WHEN** a profile declares a native GCP SLO without an opaque Monitoring Service reference
- **THEN** planning marks the SLO operation unavailable and apply cannot mutate Google Cloud resources

#### Scenario: OVH audit-only path is selected

- **WHEN** an OVHcloud profile selects the documented MKS audit forwarding path
- **THEN** the adapter requires the opaque existing stream reference and does not claim generic workload logs, metrics, or traces unless a separate supported destination is configured and verified

### Requirement: New Relic is an independent composable integration

New Relic SHALL be implemented as an external observability adapter whose
provider identity is `newrelic`, independent of the origin target provider.
It SHALL support the documented collector or OpenTelemetry/exporter path
selected by the architecture and SHALL keep New Relic object schemas out of
the portable core. The ECS path SHALL model the OpenTelemetry Collector
Contrib sidecar boundary for ECS on EC2 and Fargate. The Kubernetes path SHALL
model the documented NRDOT collector chart boundary where the target is
source-supported, currently including EKS and GKE in the source-dated
catalog. Kapsule and MKS SHALL use OTLP or a separately verified custom
collector until their target-specific support is evidenced. Generic workloads
SHALL use OTLP or produce a typed unsupported result; the adapter MUST NOT
invent a provider-specific New Relic account integration. ECS, Kubernetes, and generic OTLP paths MUST remain
separate evidence boundaries where their instrumentation and ownership differ.

The OTLP adapter SHALL use the current New Relic OTLP/HTTP contract: an HTTPS
endpoint, the signal-specific `/v1/{traces,metrics,logs}` path, and an opaque
license-key reference resolved only inside the provider adapter. A successful
HTTP response SHALL be classified as synchronous ingestion acceptance because
New Relic validates payload contents asynchronously. When queryable delivery
proof is requested, the provider-owned NerdGraph/NRQL verifier SHALL accept a
separate opaque user-key reference for the account-scoped NerdGraph API;
license and user-key material MUST never cross the portable core or appear in
plans, logs, checkpoints, or evidence. The verifier MAY poll the documented
`Log`, `Metric`, or `Span` event type for the exact ownership marker and
promote delivery and label evidence only after a positive result; the portable
core shall receive only the semantic observation. Collector deployment,
retention, alert, dashboard, SLO, rollback, and direct cleanup evidence SHALL
remain separate lifecycle requirements.

#### Scenario: New Relic is combined with native telemetry

- **WHEN** an AWS, GCP, Scaleway, or OVHcloud profile selects both native and New Relic observability
- **THEN** the scheduler applies and verifies each destination independently, records separate identities and retention/redaction evidence, and cleanup cannot pass while either owned destination remains live

#### Scenario: New Relic credential rotates

- **WHEN** the integration requests credential validation, rotation, or revocation
- **THEN** only a secret reference crosses the core boundary, the adapter proves ownership scope and redaction, and the evidence records the credential version or operation identity without the secret value

#### Scenario: New Relic collector deployment is planned without mutation

- **WHEN** an ECS or Kubernetes profile selects the New Relic collector path
- **THEN** planning returns only the target reference, collector distribution,
  endpoint, signal intent, and credential reference; the collector is not
  created until an injected deployment adapter and its ownership policy are
  available

#### Scenario: Contrib collector deployment is reversible and ownership-scoped

- **WHEN** an injected AWS ECS or Kubernetes Contrib backend applies a collector
  plan with an immutable image and a secret reference
- **THEN** the backend writes only provider-native secret references, refuses
  same-name unowned resources before mutation, verifies the owning workload is
  ready and healthy and that signals are delivered, rolls back an unverified
  deployment, and destroys only resources proven by direct inventory to belong
  to the requested ownership marker

#### Scenario: ECS verification rejects stale tasks

- **WHEN** an ECS service still reports a running task from the pre-collector
  task-definition revision
- **THEN** collector verification remains incomplete and the shared lifecycle
  cannot certify signal delivery from that stale task

#### Scenario: Kubernetes Helm collector deployment keeps chart logic behind the provider boundary

- **WHEN** an injected Helm backend applies a Kubernetes collector plan
- **THEN** the shared lifecycle passes only an opaque chart reference, exact
  chart version, endpoint, signal intent, and credential reference; the backend
  refuses an unowned release or same-marker configuration drift before mutation,
  requires ready/healthy/signal-delivery proof, and removes only the exact owned
  release on rollback or cleanup

#### Scenario: New Relic OTLP accepts a payload asynchronously

- **WHEN** the OTLP endpoint returns a successful response
- **THEN** the adapter records synchronous acceptance only and does not mark
  queryable delivery, retention, alerting, dashboard, SLO, or cleanup as
  verified without their own probes

#### Scenario: New Relic NerdGraph verifies an OTLP ownership marker

- **WHEN** a provider-owned query verifier is configured for an OTLP lifecycle
  and NerdGraph returns a positive NRQL count for the exact
  `magelift.ownership_marker` on the signal's `Log`, `Metric`, or `Span` event
- **THEN** the adapter records delayed delivery and label evidence, keeps the
  API key inside the credential callback, and does not infer retention,
  redaction, alerting, dashboard, SLO, collector, or cleanup evidence

#### Scenario: New Relic query proof uses a separate NerdGraph credential

- **WHEN** queryable OTLP proof is requested with an opaque license-key
  reference for ingestion and an opaque user-key reference for NerdGraph
- **THEN** the exporter resolves only the license key for the signal endpoint,
  the verifier resolves only the user key for NerdGraph, and neither secret is
  exposed to the portable core, plan, checkpoint, log, or evidence boundary

### Requirement: Native and Fastly edge paths are health-gated and independent

The system SHALL implement native edge lifecycle behavior for CloudFront/WAF,
Google Cloud load balancing/CDN/Cloud Armor, and officially supported
Scaleway and OVHcloud edge paths. It SHALL implement Fastly as an external
edge adapter with service/domain/TLS, origin health, routing, cache policy,
purge, WAF or security policy references, failover, rollback, ownership,
asynchronous deletion, and direct inventory verification.

An edge apply MUST NOT direct traffic to an unhealthy or unverified origin.
Edge evidence MUST independently prove TLS/certificate ownership, DNS
ownership, routing and failover convergence, origin authentication, cache-key
and bypass behavior, purge completion, security policy behavior, rollback, and
post-destroy cleanup. The public configuration MUST use composable native and
external provider fields; it MUST NOT reintroduce singular legacy provider
selectors.

#### Scenario: Fastly is composed with an AWS origin

- **WHEN** a profile selects Fastly as external edge and AWS as the target provider
- **THEN** the Fastly adapter retains provider identity `fastly`, the edge plan targets AWS as the origin, and the core validates both identities without treating Fastly as an AWS module port

#### Scenario: Origin health fails

- **WHEN** an edge apply or failover probe reports an unhealthy origin
- **THEN** traffic promotion is blocked, no route is directed to that origin, and the evidence records a blocked result with the health reason

#### Scenario: Fastly existing-service mutation uses an editable version

- **WHEN** a Fastly apply or cleanup changes domains or versioned configuration
  on an existing service
- **THEN** the adapter clones, validates, and activates an editable service
  version through a separate provider SDK port, restores the previous active
  version after post-activation verification failure, and rejects the
  versioned mutation before state changes when that port is unavailable

#### Scenario: Purge and destroy complete

- **WHEN** a native or Fastly edge configuration is destroyed
- **THEN** the adapter proves purge/detachment where required, polls asynchronous deletion through the owning API, preserves unmarked user resources, and reports zero owned live resources only after direct inventory verification

### Requirement: Credentials and provider output never escape the trust boundary

Provider and SaaS clients SHALL resolve credentials from typed secret or
identity references inside provider packages. Secret values, access tokens,
private keys, certificates, database passwords, DNS credentials, and recovery
credentials MUST NOT enter public plans, checkpoints, operation IDs, logs,
artifacts, evidence, generated documentation, or diagnostics. Credential
rotation and revocation MUST be testable without changing the core contract.

#### Scenario: Provider SDK returns sensitive text

- **WHEN** an SDK error or response contains credential-shaped material
- **THEN** the adapter redacts it before crossing the core boundary and the lifecycle fails closed if a safe normalized identity cannot be produced

#### Scenario: Acceptance is missing a credential

- **WHEN** local admission cannot resolve a required credential reference
- **THEN** the scheduler returns a blocked admission report before any paid provider mutation and does not retry the cell as PASS

### Requirement: Certification reuses resources without weakening evidence

The certification system SHALL build one immutable artifact per compatible
runtime contract, reuse compatible scrubbed fixtures and warm sessions, and
reuse backup, observability, and edge setup only when the complete fingerprint
proves those boundaries unchanged. It SHALL perform quota, account/project,
network, state-backend, fixture, ownership, and local-resource admission before
paid mutations; independent provider groups MAY run concurrently, while shared
mutation keys MUST serialize.

Every cold boundary and warm transition SHALL emit independent evidence with
architecture fingerprint, artifact digest, operation identities, ownership
marker, reuse mode, duration/cost, and cleanup truth. Interrupted runs MUST
resume the same scope or fail closed on stale/mismatched checkpoints.

#### Scenario: Compatible warm transition is selected

- **WHEN** two cells share architecture, fixture, migration, resilience,
  observability, edge, artifact, state, and ownership boundaries and differ only
  in an explicitly allowed transition
- **THEN** the scheduler reuses the existing session and records the transition
  and reused identity without creating a duplicate writer or claiming a new cold
  baseline

#### Scenario: Fingerprint changes

- **WHEN** the provider, runtime, service major, resilience destination,
  telemetry destination, edge path, artifact, fixture, migration, state backend,
  or ownership boundary changes
- **THEN** warm reuse is rejected and the scheduler requires a new cold boundary

#### Scenario: Cleanup is delayed

- **WHEN** a provider returns delayed tombstones or protected resources after a
  test
- **THEN** cleanup remains incomplete, direct owning-service polling continues
  within its budget, and the run cannot be certified until the inventory is
  empty or the result is explicitly operator-approved and non-certified

### Requirement: Capability and certification documentation are evidence-derived

Generated capability, architecture, resilience, observability, edge,
certification, and recovery-runbook documents SHALL include source URLs and
retrieval dates for fast-moving provider facts and SHALL consume only validated
catalog records and sealed evidence records. Missing or absent evidence MUST be
visible as evidence-gated, not inferred as support or certification.

#### Scenario: GCP Valkey documentation is refreshed

- **WHEN** the source-dated catalog is refreshed from the official Memorystore
  supported-versions documentation
- **THEN** Valkey 9.0 is represented as the default GA target and Valkey 9.1
  as Preview, with the source URL and retrieval date attached to both claims

#### Scenario: No live evidence bundle exists

- **WHEN** documentation generation runs without a sealed JSONL evidence bundle
- **THEN** it still renders the policy and source catalog but explicitly states
  that live certification is evidence-gated and does not emit a PASS claim

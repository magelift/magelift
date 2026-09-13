## Purpose

Defines provider-native and New Relic observability as first-class,
credential-safe capabilities with signal coverage, alerting, SLO evidence, and
failure behavior across all supported cloud architectures.

## Requirements

### Requirement: Observability signals are explicit

Each profile MUST declare logs, metrics, traces, audit events, application
health, provider health, backup health, recovery health, and edge health
signals. The plan MUST identify the authoritative destination, retention,
sampling, labels, alert owners, and expected data residency for each enabled
signal.

#### Scenario: A profile enables native logs and New Relic metrics

- **WHEN** an operator selects provider-native logs plus New Relic metrics and
  traces
- **THEN** the plan shows both destinations, their signal ownership, retention,
  credential references, export path, and any unsupported signals before apply

### Requirement: Native provider observability is capability-driven

The catalog MUST represent the current native observability capabilities of
AWS, GCP, Scaleway, and OVHcloud from dated provider sources. AWS CloudWatch
and Google Cloud Observability MUST be modeled where applicable; Scaleway and
OVHcloud native equivalents MUST be modeled when their current products expose
the required signal. Missing logs, metrics, traces, audit, or alerting support
MUST be reported explicitly rather than inferred.

#### Scenario: A provider lacks a native trace service

- **WHEN** the selected provider exposes logs and metrics but not a supported
  native trace path
- **THEN** the plan marks native traces unavailable and offers a declared
  New Relic or OpenTelemetry path without claiming native trace coverage

### Requirement: Native destination identity is opaque and provider-owned

An existing native telemetry destination MAY be selected through one opaque,
single-line identity reference. The portable contract MUST NOT encode a
provider SDK object, secret value, or legacy provider selector in that field.
An adapter MUST reject a missing or invalid reference when its native operation
requires an existing destination, and MUST report signals that its resource
graph cannot deliver as unavailable.

#### Scenario: OVH audit delivery uses an existing Logs Data Platform stream

- **WHEN** an OVH MKS profile enables native audit events
- **THEN** the plan carries only the opaque stream identity, provisions the
  documented Kubernetes audit subscription, and keeps generic workload logs,
  metrics, and traces explicitly unavailable until a separate adapter path is
  implemented

#### Scenario: A provider graph exists without delivery proof

- **WHEN** a native dashboard, data source, or alert policy is present in a
  Pulumi preview
- **THEN** the profile remains open for delivery, label, retention, alert,
  redaction, and cleanup evidence; resource registration alone cannot close
  observability certification

#### Scenario: GCP owns an SLO under an existing Monitoring Service

- **WHEN** a GKE profile declares an `application-health` SLO and carries an opaque native reference in the form of an existing Google Cloud Monitoring Service
- **THEN** the provider adapter creates or reuses only the owned SLO through the official Service Monitoring API or native resource, verifies its goal, rolling period, availability SLI, and ownership labels, and inventories it with the plan-scoped service identity during cleanup

### Requirement: New Relic integration has a vendor-neutral contract

New Relic MUST be selectable through typed observability intent that supports
logs, metrics, traces, Kubernetes or container infrastructure, application
instrumentation, dashboards, alerts, and SLOs. The implementation MUST use a
current official New Relic collector integration where available and an
OpenTelemetry or supported exporter fallback where it is not. The documented
ECS path is the OpenTelemetry Collector Contrib sidecar boundary for ECS on
EC2 and Fargate; the documented Kubernetes path is the NRDOT collector chart
for source-supported environments such as EKS and GKE; Kapsule and MKS MUST
use OTLP or a separately verified custom collector rather than inherit an
unsupported NRDOT claim. Generic workloads use OTLP or receive a typed
unsupported result. The adapter MUST NOT invent a provider-specific New
Relic account integration.
Every New Relic OTLP path MUST require an opaque license-key credential
reference because the current OTLP endpoint requires an `api-key` header; an
endpoint alone is not sufficient authentication. A successful OTLP HTTP
response MUST be recorded as ingestion acceptance only. When queryable proof
is requested, the provider-owned NerdGraph/NRQL verifier MUST accept a separate
opaque user-key reference because the NerdGraph API is a different credential
boundary. The verifier MAY poll the exact ownership marker on the documented
`Log`, `Metric`, or `Span` event and record queryable delivery and label
evidence only after a positive result; the portable contract receives only that
semantic observation. Neither credential may appear in the portable core,
plans, logs, checkpoints, or evidence.

Collector deployment, queryable delivery, retention, alert, dashboard, SLO,
rollback, and direct cleanup evidence MUST remain separate lifecycle classes;
a typed plan or accepted OTLP probe MUST NOT close those classes.

#### Scenario: New Relic is selected for EKS and ECS

- **WHEN** an operator enables New Relic for both EKS and ECS profiles
- **THEN** the plan selects the appropriate container or Kubernetes telemetry
  path, records signal coverage and required privileges, and does not require
  the user to place New Relic-specific objects in the portable core profile

#### Scenario: New Relic OTLP is missing its license-key reference

- **WHEN** an operator supplies an OTLP endpoint without a credential reference
- **THEN** planning or provider preflight rejects the configuration before any
  export call, because New Relic requires the `api-key` header

#### Scenario: New Relic accepts an OTLP probe

- **WHEN** New Relic returns a successful response after authenticating and
  validating the probe request
- **THEN** evidence records bounded ingestion acceptance and ownership
  attributes, while delivery, retention, redaction, and alert evidence remain
  incomplete until a query or application smoke probe proves them

#### Scenario: Provider-owned NRQL verifies an OTLP marker

- **WHEN** the New Relic adapter is configured with an account-scoped
  NerdGraph verifier and NRQL returns a positive count for the exact
  ownership marker on the signal's `Log`, `Metric`, or `Span` event
- **THEN** evidence records delayed queryable delivery and label proof without
  exposing the API key or moving NerdGraph/NRQL types into the portable core;
  retention, redaction, alert, dashboard, SLO, collector, and cleanup proof
  remain independent

#### Scenario: Provider visibility delay stays within a bounded query budget

- **WHEN** New Relic accepts an OTLP payload but the first exact-marker NRQL
  query returns an empty result
- **THEN** the provider-owned verifier polls the same signal and marker with a
  finite retry and request-timeout budget, records queryable delivery only
  after a positive result, and reports the signal as unproven when the budget
  expires

#### Scenario: OTLP ingestion and NerdGraph verification use separate keys

- **WHEN** a live New Relic OTLP plan supplies an opaque license-key reference
  and an opaque account-scoped NerdGraph user-key reference
- **THEN** OTLP resolves only the license key, NRQL resolves only the user key,
  and the evidence records the two credential boundaries without recording
  either secret value

#### Scenario: The authenticated New Relic CLI verifies a marker without a resource

- **WHEN** a bounded acceptance run uses an authenticated New Relic CLI profile
  to post one custom event with a unique MageLift marker and polls the same
  marker through NRQL
- **THEN** the evidence records queryable data-plane delivery and the observed
  ingestion delay, creates no dashboard, alert, entity, or collector, and
  records provider-managed retention instead of claiming that telemetry was
  deleted during infrastructure cleanup

#### Scenario: New Relic collector deployment is planned without mutation

- **WHEN** an operator selects the ECS or Kubernetes collector path
- **THEN** planning returns only the opaque target reference, collector
  distribution, endpoint, signals, and credential reference; resource creation
  waits for an injected deployment adapter with ownership and cleanup proof

#### Scenario: Collector deployment is readiness-gated and reversible

- **WHEN** an injected ECS or Kubernetes collector backend applies a planned
  deployment
- **THEN** the shared lifecycle requires exact ownership, ready and healthy
  workloads, and signal-delivery proof before success; a failed verification
  invokes provider rollback, and destroy proves direct owning-service inventory
  cleanup without touching unowned collector resources

#### Scenario: Contrib collector backends keep secrets and ownership separate

- **WHEN** the AWS ECS or Kubernetes Contrib backend applies an injected
  collector deployment
- **THEN** only a provider-native secret reference crosses into the workload,
  immutable images are required, same-name unowned resources are rejected
  before mutation, readiness/health and signal delivery are verified, failed
  verification rolls back, and direct owning-service inventory proves cleanup

#### Scenario: Kubernetes secret key selection remains non-secret

- **WHEN** a Kubernetes collector plan uses a
  `kubernetes-secret://namespace/name#key` credential reference
- **THEN** the core accepts the scheme-scoped key selector, the backend emits a
  SecretKeyRef rather than a value, and generic credential schemes continue to
  reject fragments

#### Scenario: Helm collector chart logic remains provider-owned

- **WHEN** an injected Kubernetes Helm backend applies a collector plan
- **THEN** the shared lifecycle sends only an opaque chart reference, exact chart
  version, endpoint, signal intent, and credential reference; ownership and
  configuration drift are checked before mutation, readiness/health/signal
  delivery gates success, and rollback or cleanup removes only the exact owned
  release

### Requirement: Credentials and telemetry data are protected

New Relic licenses, ingest keys, provider credentials, certificates, and
instrumentation secrets MUST be supplied only through secret references or
provider-managed identity. Secret values MUST NOT appear in configuration,
state, evidence, logs, diagnostics, or telemetry labels. The plan MUST expose
PII, sensitive-field, sampling, and egress controls.

#### Scenario: A user supplies a plaintext New Relic key

- **WHEN** validation receives a literal ingest key in portable configuration
- **THEN** validation rejects or redacts it and requires a supported secret
  reference before mutation

### Requirement: Observability covers resilience and edge operations

Every certifying profile MUST emit or query signals for backup completion,
restore progress, replication lag, queue depth, database health, cache health,
search health, edge origin health, TLS expiry, failover state, and cleanup
status in addition to application health.

#### Scenario: Backup lag exceeds the RPO budget

- **WHEN** telemetry shows that the latest durable backup is older than the
  profile's maximum RPO
- **THEN** the SLO or release gate reports a resilience violation and prevents
  the profile from being marked healthy or certifying

### Requirement: Alerts and SLOs are actionable

The plan MUST define alert thresholds, severity, notification ownership,
deduplication, maintenance suppression, and runbook links for availability,
latency, errors, backup age, replication lag, and recovery objectives. A
dashboard without alert and runbook behavior MUST NOT count as operational
observability certification.

#### Scenario: A failover alert fires during a drill

- **WHEN** an HA or DR exercise crosses an availability, latency, or RPO/RTO
  threshold
- **THEN** the alert identifies the affected profile, opens the declared
  runbook path, and is recorded with detection and recovery timestamps

### Requirement: Observability lifecycle is clean and resumable

Provider-native and New Relic resources created by a run MUST have exact
ownership markers, bounded teardown, retryable cleanup, and a direct inventory
assertion. Pre-existing accounts, dashboards, alert policies, and credentials
MUST be preserved unless ownership is proven.

#### Scenario: Telemetry cleanup fails after runtime destroy

- **WHEN** the cloud runtime is gone but an observability resource remains
- **THEN** cleanup reports the exact owned resource, preserves retry state, and
  fails the certification result until the resource is removed or explicitly
  retained with operator approval

### Requirement: New Relic live cells follow thin-credit attach

New Relic Magento telemetry evidence MUST attach to a packed GCP Magento origin when possible. Thin paid credits apply. Collector lifecycle remains experimental until evidenced.

#### Scenario: New Relic does not spawn Magento

- **WHEN** New Relic queryable delivery is the open claim
- **THEN** the runner uses the existing Magento session or records the cell unproven

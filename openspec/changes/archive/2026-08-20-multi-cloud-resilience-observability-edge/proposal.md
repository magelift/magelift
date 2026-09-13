## Why

MageLift's current RC1 work proves selected runtime cells, but it does not yet
define a complete production-support contract for AWS, GCP, Scaleway, and
OVHcloud across compute architectures, managed and self-hosted stateful
services, high availability, backup and restore, disaster recovery,
observability, or edge delivery. The open changes also treat Fastly,
provider-native edge, and vendor observability as separate experiments, which
does not give operators one honest scope for protecting data and maintaining
service availability.

The current provider documentation also invalidated one implementation
assumption: Google Cloud Memorystore for Valkey supports Valkey 9.0 GA and
9.1 Preview. The former GCP adapter path and its existing live evidence used
Valkey 8.0; the implementation now selects 9.0 for the current Adobe profile
and keeps 9.1 explicitly Preview. The change must make provider capability,
Adobe compatibility, implementation support, and live certification evidence
agree before a production claim is closed.

## What Changes

- Define a complete four-provider architecture catalog covering AWS ECS
  Fargate and other supported ECS capacity modes, full AWS EKS operation,
  GCP GKE Autopilot and Standard, Scaleway Kapsule, and OVHcloud MKS.
- Model managed and self-hosted boundaries explicitly for databases, cache,
  search, queues, media/object storage, networking, ingress, and edge. Every
  combination is classified as supported, experimental, unavailable,
  unsupported, or blocked rather than inferred from a nearby topology.
- Define production resilience profiles for single-zone, multi-zone HA,
  cross-zone failure, regional disaster recovery, restore-in-place, and
  alternate-region recovery. Each profile requires explicit RPO, RTO,
  availability, data-retention, and failover assumptions.
- Add backup and restore contracts for database point-in-time recovery and
  snapshots, media/object versioning and replication, infrastructure state,
  configuration and secrets, search rebuild, cache reconstruction, and queue
  recovery. A cache or search rebuild MUST NOT be mistaken for a durable data
  backup.
- Define disaster exercises for provider outage, zone loss, region loss,
  corrupted data, failed deployment, lost credentials, and partial teardown,
  including data-integrity checks and measured RPO/RTO results.
- Define native observability capabilities for each provider, including logs,
  metrics, traces, dashboards, alerts, audit events, health checks, and SLO
  evidence. Add New Relic as a vendor-neutral integration with provider-native
  integrations where available and OpenTelemetry fallback where they are not.
- Define native and external edge capabilities: CloudFront and WAF for AWS,
  Google Cloud Load Balancing, Cloud CDN, and Cloud Armor for GCP, the current
  Scaleway and OVHcloud edge products where their official capabilities support
  the requested profile, and Fastly as a provider-independent edge layer.
- Require edge origin health, TLS and certificate rotation, DNS ownership,
  cache invalidation, purge, WAF policy, failover, rollback, and cleanup
  evidence for every claimed edge architecture.
- Replace Cartesian-product live certification with a coverage-driven
  scheduler: build and publish each immutable artifact once, reuse compatible
  warm sessions, create cold baselines at deterministic boundaries, run
  independent provider groups concurrently only when quotas and cleanup scopes
  are isolated, and retain one shared fixture per certified boundary.
- Add bounded checkpoint resume, interruption cleanup, asynchronous deletion
  polling, provider quota and cost budgets, restore-fixture reuse, and direct
  post-run inventories so slow cloud resources are amortized without hiding
  failed cleanup or data-loss risk.
- Regenerate readiness and capability claims only from evidence that includes
  architecture identity, service versions, backup and DR results, observability
  signal coverage, edge behavior, measured RPO/RTO, cost, and cleanup truth.

## Capabilities

### New Capabilities

- `multi-cloud-runtime-architectures`: Supported architecture families and
  managed/self-hosted service boundaries for AWS, GCP, Scaleway, and OVHcloud.
- `resilience-and-disaster-recovery`: HA, backup, restore, RPO/RTO, failover,
  regional recovery, data-integrity, and disaster-exercise contracts.
- `provider-observability-and-new-relic`: Native provider telemetry plus New
  Relic and OpenTelemetry integration, alerting, SLOs, and credential safety.
- `native-and-fastly-edge`: Provider-native edge and Fastly origin delivery,
  TLS, WAF, purge, failover, routing, rollback, and cleanup.
- `efficient-cloud-certification`: Coverage-driven cold and warm execution,
  immutable artifact reuse, scheduling, checkpoints, cost controls, and
  cleanup-safe certification evidence.

### Modified Capabilities

None. The existing RC1, Fastly, and community-provider changes are narrower
planning artifacts. This change coordinates their remaining work and provides
the production-resilience source of truth without silently rewriting their
completed history.

## Impact

The change affects the compatibility and architecture catalog, configuration
schema, provider adapters under `internal/cloud`, shared platform contracts,
backup and restore workflows, edge and observability extensions, acceptance
session scheduling, evidence generation, release gates, and operational
documentation. It requires current provider capability inventories, disposable
accounts or projects, test data with known checksums, New Relic and edge
credentials supplied only by secret references, and explicit operator-selected
RPO/RTO profiles.

The implementation must not claim that every theoretical provider combination
is supported. Completeness means every declared architecture family and every
documented service boundary has an explicit status and an executable reason
when it is unsupported, unavailable, blocked, or not yet certified.

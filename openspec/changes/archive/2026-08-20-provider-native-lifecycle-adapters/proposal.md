## Why

The existing provider-neutral contracts now define the correct lifecycle,
ownership, evidence, and community-extension boundaries, but the first-party
cloud packages still stop at descriptors and injected operation-client seams.
That is not full provider support: without concrete SDK/API clients, credential
resolution, polling, and live cleanup, a module must remain experimental even
when its Pulumi graph can be previewed. This change makes the production
provider and SaaS implementations explicit and evidence-gated so the core
remains generic while provider packages own only their native APIs.

Provider documentation is part of the input, not an implementation assumption.
For example, the current Google Cloud documentation lists Valkey 9.0 as the
default supported version and Valkey 9.1 as Preview, so the adapter and
certification matrix must preserve that distinction rather than collapsing both
to a generic “Valkey 9” value.

## What Changes

- Add concrete provider-side lifecycle clients behind the existing provider-neutral SDK ports for AWS, GCP, Scaleway, and OVHcloud.
- Cover every declared architecture boundary independently: AWS ECS capacity modes and EKS compute modes; GKE Autopilot and Standard; Scaleway Kapsule; and OVHcloud MKS.
- Implement provider-specific backup, restore, integrity, HA, DR, fencing, failback, cleanup, and asynchronous operation polling for each durable data class that the capability catalog marks executable. Unsupported search, queue, or cache-backup claims remain typed unavailable or reconstruction-only results.
- Implement native observability lifecycle clients for CloudWatch, Google Cloud Observability, Scaleway Cockpit, and the documented OVHcloud Logs Data Platform path, including signal coverage, alert/dashboard/SLO verification, retention, redaction, credential rotation, revocation, and cleanup.
- Implement native edge lifecycle clients for CloudFront/WAF, Google Cloud load balancing/CDN/Cloud Armor, and the officially supported Scaleway and OVHcloud edge paths, with health-gated routing, TLS/DNS, purge, authentication, failover, rollback, and inventory cleanup.
- Complete Fastly and New Relic as independent external providers composed with any supported origin target; they must not be forced into the origin module's provider identity.
- Add efficient live certification orchestration that reuses immutable artifacts, scrubbed fixtures, compatible warm sessions, backup sets, telemetry setup, and edge setup while preserving independent evidence at every cold boundary and preventing duplicate writers.
- Generate release documentation from validated catalog data and sealed JSONL evidence, keeping absent, stale, unavailable, unsupported, blocked, experimental, and failed rows explicit.
- Keep the first-release configuration clean: do not add or preserve singular legacy `edge.provider` or `observability.provider` fields. Target and existing-resource provider identities remain identity data.

## Capabilities

### New Capabilities

- `provider-native-lifecycle-adapters`: Concrete cloud and SaaS lifecycle implementations behind the provider-neutral SDK, with architecture-specific capability declarations and evidence gates.

### Modified Capabilities

<!-- The existing provider-neutral and certification changes define the public
     contract. This change adds implementations behind those contracts and does
     not rewrite their semantic requirements. -->

## Impact

- Affected Go packages: `sdk/v1`, `internal/platform`, `internal/cloud/<provider>`, `internal/external`, `internal/certification`, provider registries, and acceptance harnesses.
- Provider SDK/API dependencies will be isolated under provider packages; the core and public SDK must not import cloud or SaaS SDK types.
- Live tests will require disposable accounts/projects, least-privilege credential references, quota admission, network/state backends, and ownership-scoped cleanup. No secret value may enter plans, checkpoints, logs, artifacts, or committed evidence.
- Existing first-party modules remain deployment-safe and experimental until their concrete lifecycle clients and live evidence satisfy the new requirements.

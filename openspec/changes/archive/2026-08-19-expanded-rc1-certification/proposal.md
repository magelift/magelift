## Why

Related scope is defined by [`multi-cloud-resilience-observability-edge`](../multi-cloud-resilience-observability-edge/proposal.md). This change owns the release-specific live harness and cleanup evidence; the related change owns the provider-neutral capability, architecture, resilience, observability, edge-composition, scheduler, and release-gate contracts.

The current RC1 evidence covers AWS ECS Fargate and GCP GKE Autopilot, while OVHcloud and Scaleway remain experimental. The paid accounts make a broader release gate practical, but only if the test matrix follows Adobe's supported combinations and every live run proves teardown.

## What Changes

- Add a versioned compatibility catalog for Adobe Commerce 2.4.6-p15, 2.4.7-p10, 2.4.8-p5, and 2.4.9.
- Represent database, search, cache, queue, web cache, edge, runtime, and deployment shape as explicit certification dimensions.
- Classify each cell as Adobe-supported, MageLift-compatible, unsupported, or not available on the target.
- Extend the acceptance evidence contract to AWS, GCP, OVHcloud, and Scaleway.
- Add provider-specific orphan discovery and cleanup assertions with exact prefixes and tags.
- Make RC1 certification claims depend on generated evidence rather than hand-edited matrix rows.
- Keep Docker and local Compose as compatibility test targets, not cloud certification targets.
- **BREAKING** Stop treating a provider or architecture as certified when it has only Pulumi mocks or a successful infrastructure preview.

## Capabilities

### New Capabilities

- `compatibility-catalog`: Adobe release and dependency compatibility rules.
- `certification-evidence`: Reproducible acceptance cells and release evidence.
- `teardown-safety`: Provider cleanup, orphan detection, and zero-leftover proof.

### Modified Capabilities

None. The repository has no existing OpenSpec capability specs.

## Impact

The change affects `internal/config`, `internal/platform`, provider adapters under `internal/cloud`, acceptance scripts under `scripts/`, evidence generation, the capability matrix, and release-readiness documentation. It also requires live credentials and disposable account prefixes for the four first-party providers.

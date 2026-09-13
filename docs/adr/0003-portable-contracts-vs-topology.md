# ADR 0003: Portable Magento contracts vs per-cloud topology

- Status: Accepted
- Date: 2026-08-22

## Context

Magento build and deploy phases are mostly portable. An Aurora cluster, an ECS service, and a GKE workload are not. A shared YAML schema of "the cloud" would hide failure modes and leak lowest-common-denominator fields into `magelift.yaml`.

## Decision

Portable contracts describe what Magento needs: application model, artifact manifest, build lifecycle, release identity, capability requirements. They do not name the cloud resources that satisfy them.

Provider topology lives in `internal/cloud/<provider>/` (network, database, cache, search, queue, runtime, edge, observability, stack). `internal/platform` and `sdk/v1` stay provider-neutral. The PHP Composer package under `build/` is not the Go tree.

Shared Pulumi components that switch on `if provider ==` are forbidden. Each cloud owns its graph (ADR 0004).

Typed provider options may appear under `target.<provider>`. Raw provider schemas do not enter portable YAML.

## Consequences

Moving a shop from Fargate to Autopilot keeps Magento contracts and replaces topology. Cost, recovery, and IAM stay provider-specific. Adding a cloud is a new adapter package, not a third copy of Magento orchestration.

## Alternatives considered

- Universal infrastructure schema: rejected (weakest-shared-feature YAML).
- Raw Pulumi in project YAML: rejected (unstable public contract).

## Provenance

Original project decision. Public Pulumi and Magento docs only.

# ADR 0004. Ports and adapters

Accepted on 2026-08-22.

## Context

Magento deploys through the same CLI on each provider. Each cloud has its own
resources and API clients.

## Decision

`internal/platform` and `sdk` own the shared contracts for stack modules,
outputs and Magento workload settings. The CLI selects the module by
`target.provider` and `target.runtime`. It asks that module to plan, deploy and
run the operations it supports.

The provider modules under `providers/` implement those contracts. Each owns
its infrastructure and interprets its `target.<provider>` settings. Stack names
include the provider and runtime to avoid collisions in a shared backend.

## Consequences

AWS, Google Cloud and Azure build separate plugins. The CLI can reuse Magento
orchestration and keep output names stable across providers.
[Provider plugin contract](0013-provider-plugin-contract.md)

## Alternatives considered

We considered a shared Pulumi graph with provider switches. Each cloud needs
its own topology and lifecycle rules. [Portable contracts](0003-portable-contracts-vs-topology.md)

## Provenance

Original design. See `internal/platform`, `sdk` and `providers/`.

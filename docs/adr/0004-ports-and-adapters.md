# ADR 0004: Ports and adapters

- Status: Accepted
- Date: 2026-08-22

## Context

Magento orchestration must not be copied into every `internal/cloud/<p>` package. The CLI must not type factories on a single provider's Spec.

## Decision

MageLift uses ports and adapters:

1. **Ports** live in `internal/platform` (and `sdk`). They own stack module registration, stable output keys, Magento workloads, and env binding names. They do not import cloud SDKs beyond the Pulumi `RunFunc` handed to Automation API.
2. **Adapters** live in `internal/cloud/<provider>/`. Each owns topology, catalogs, and `target.<provider>` YAML.
3. The CLI selects a `StackModule` by `target.provider` + `target.runtime` through `cmd/magelift` / `platform.ModuleRegistry`. `infra.RegisterTarget` alone does not ship `magelift deploy`.
4. Day-2 hangs off the same module via small `Has*` interfaces: `HasOps`, `HasBootstrap`, `HasState`, `HasSecrets`, `HasRuntimeObserve`. An omitted port or `ErrNotSupported` keeps that surface experimental.
5. Stack DIY names include provider and runtime so two clouds do not collide on one backend.

## Consequences

A third cloud is a new adapter plus module wiring. AWS packages stay under `internal/cloud/aws/`. Output keys used by portable CLI commands stay Magento-shaped where possible.

## Alternatives considered

- Shared Pulumi graph with provider switches: rejected (ADR 0003).
- CLI typed on `awsstack.Spec` forever: rejected (blocks a second provider).

## Provenance

Original. Matches the current `internal/platform` and `internal/cloud/*` layout.

# ADR 0009: Day-2 Magento ports on StackModule

- Status: Accepted
- Date: 2026-07-19

## Context

ADR 0008 introduced `platform.StackModule` for plan/program/outputs and optional
`HasOps` for Magento candidate deploy. Day-2 CLI commands (bootstrap, state,
secrets, logs, exec, health) remained typed on AWS packages, so a third provider
still required editing each command.

## Decision

Optional day-2 ports hang off the same `StackModule` via small `Has*` interfaces:

| Port | Interface | Magento use |
| --- | --- | --- |
| `HasOps` | `Ops` | Deploy lock + candidate steps |
| `HasBootstrap` | `Bootstrap` | DIY backend + CI identity |
| `HasState` | `State` | DIY lock UX / backup |
| `HasSecrets` | `Secrets` | Application secrets CLI |
| `HasRuntimeObserve` | `RuntimeObserve` | Logs, exec, runtime health |

Adapters that omit a port (or return `ErrNotSupported`) keep the target
experimental for that surface. The CLI resolves ports through
`platform.Module*` helpers after selecting the module by provider/runtime.

Stack DIY names include provider and runtime (`project-env-provider-runtime`,
dots in runtime become dashes) so AWS and GCP stacks do not collide on one
backend. Portable capability IDs use Magento-shaped names (`object-storage.blob`,
`edge.cdn`, …); AWS product aliases remain as deprecated constants.

## Consequences

- Adding Azure day-2 is implementing ports on `internal/cloud/azure`, not rewriting
  CLI factories.
- AWS implementations live under `internal/cloud/aws/ops` (and related packages).
- `internal/infra.Registry` remains the SDK extension index only.

## Alternatives considered

- One fat `CloudProvider` interface: rejected; violates Go interface segregation.
- Shared Pulumi day-2 resources: rejected (ADR 0002/0008).

## Provenance

Extends ADR 0008 after OSS multi-provider readiness review (2026-07-19).

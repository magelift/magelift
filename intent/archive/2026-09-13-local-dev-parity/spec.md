---
status: specified
slug: local-dev-parity
intent: intent.md
---

# Spec: local stack as close to production as Compose allows

Auto-approved per the standing `/goal` instruction. Defaults taken:
parity stays in `local init` plan-time warnings plus docs; no new
`local doctor` command for v1 (warnings fire exactly when the user
generates the stack, and a second surface would need its own
contract). Local Redis without the hatch already fails; this spec
locks that behavior and closes the substitute and drift gaps around
it.

## Requirements

### Requirement: cache twin covers ElastiCache

`PlanFor` SHALL steer the local cache family to Valkey and record a
substitute when the cloud cache product is ElastiCache Valkey, the
same as it does for Memorystore.

#### Scenario: AWS cache substitute recorded

- **WHEN** planning local for an AWS target
- **THEN** the runtime plan records `ElastiCache Valkey` as the cloud
  twin of the local Valkey container

### Requirement: rabbit family covers every broker mode

`ecs-artemis` and `rabbitmq` queue modes SHALL steer the local
queue family to RabbitMQ and record the managed twin, like
`amazon-mq` and `ecs-rabbitmq` do today.

#### Scenario: artemis steers and records

- **WHEN** planning local for a cloud queue mode of `ecs-artemis`
- **THEN** the local queue family is RabbitMQ and the plan records
  the `ECS Artemis` substitute

### Requirement: provisioned search records its twin

An AWS `provisioned` search mode SHALL record the managed-domain
substitute instead of passing silently.

#### Scenario: provisioned substitute recorded

- **WHEN** planning local for an AWS `provisioned` search mode
- **THEN** the plan records the OpenSearch provisioned domain twin

### Requirement: drift warnings when cloud has less than local

`PlanFor` SHALL warn when the cloud environment disables search or
uses database-backed queues while the local plan still runs the
container, because local success then proves nothing about cloud.

#### Scenario: disabled search warns

- **WHEN** planning local for a cloud search mode of `disabled`
- **THEN** the plan warns that local OpenSearch does not prove cloud
  behavior

#### Scenario: database queues warn

- **WHEN** planning local for a cloud queue mode of `db` or
  `database`
- **THEN** the plan warns that the local broker does not prove cloud
  behavior

### Requirement: impossible parity fails named

Planning local with cache family `redis` and no
`compatibility.allowUnsupported` SHALL keep failing with the named
Adobe-row reason (regression lock, no behavior change).

#### Scenario: redis without hatch fails

- **WHEN** planning local with `redis` cache and no hatch
- **THEN** `PlanFor` returns the Adobe compatibility row error

### Requirement: docs state what local proves

`docs/local-vs-cloud.md` SHALL state in one place what a green local
run proves and what still needs a cloud preview.

#### Scenario: proof scope readable

- **WHEN** a reader opens the local-vs-cloud page
- **THEN** one section lists what local proves and what needs cloud,
  matching the substitute and warning behavior

## Design

All behavior lives in `internal/localdev` (`hints.go` steering plus
substitutes, `catalog.go` drift warnings in `PlanFor`); `local init`
already surfaces `Warnings` and `Substitutes` in its JSON output, so
no CLI changes. Docs edit is confined to `local-vs-cloud.md`.

## Gotchas / policy flags

- Never soften the Redis hatch: warn-only would let Adobe-incompatible
  local runs look green.
- Warnings name the direction (cloud has less) so nobody reads them
  as local misconfiguration.
- GCP `opensearch` workload mode is the same-shape twin, not a
  substitute; it stays unrecorded.

## Open questions carried forward

- The three top first-deploy mismatches are unknown until trials;
  this change instruments the known twin gaps (cache, broker modes,
  provisioned search, disabled-service drift) and trial feedback
  extends the table.
- `local doctor` stays out of v1; revisit if trials show warnings
  get missed.
